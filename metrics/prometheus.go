// Package reporter provides Prometheus metrics reporting functionality.
// This package implements a reporter that converts application metrics to Prometheus format
// and exposes them via HTTP endpoint or push gateway.
package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/linchenxuan/strix/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/push"
)

const (
	_metricsChanSize     = 1000000
	_serviceName         = "exporter"
	_healthCheckInterval = 30 * time.Second // Health check interval for TTL monitoring
)

// metricType defines the type of Prometheus metric.
type metricType int

const (
	_metricTypeCounter metricType = iota
	_metricTypeGauge
	_metricTypeHistogram
	_metricTypeSummary
)

// metricOpt contains options for creating Prometheus
type metricOpt struct {
	subsystem   string
	name        string
	constLabels map[string]string
}

// newMetricOpt creates metric options from a metric record and external labels.
func newMetricOpt(rc *Record, extLabels map[string]string) *metricOpt {
	opts := &metricOpt{
		subsystem:   strings.ReplaceAll(rc.Metrics().Group(), ".", "_"),
		name:        strings.ReplaceAll(rc.Metrics().Name(), ".", "_"),
		constLabels: make(map[string]string, len(rc.Dimensions())+len(extLabels)),
	}

	for k, v := range extLabels {
		opts.constLabels[k] = strings.ReplaceAll(v, ".", "_")
	}

	for k, v := range rc.Dimensions() {
		opts.constLabels[k] = strings.ReplaceAll(v, ".", "_")
	}
	return opts
}

// promGauge wraps a Prometheus gauge with additional value tracking for averaging.
type promGauge struct {
	prometheus.Gauge
	value float64 // Accumulated value for averaging
	cnt   int     // Count of observations
}

// newPromGauga creates a new Prometheus gauge wrapper from a metric record.
func newPromGauga(rc *Record, extLabels map[string]string) *metricWrapper {
	o := newMetricOpt(rc, extLabels)
	opts := prometheus.GaugeOpts{
		Subsystem:   o.subsystem,
		Name:        o.name,
		ConstLabels: o.constLabels,
	}

	g := &promGauge{
		Gauge: promauto.NewGauge(opts),
		value: 0,
		cnt:   0,
	}
	_ = g.merge(rc)

	return &metricWrapper{
		m:  g,
		mt: _metricTypeGauge,
	}
}

// merge updates the gauge value based on the metric policy.
func (p *promGauge) merge(rc *Record) error {
	switch rc.Metrics().Policy() {
	case Policy_Set:
		p.Set(float64(rc.Value()))
	case Policy_Sum:
		p.Add(float64(rc.Value()))
	case Policy_Max:
		p.Set(float64(rc.Value()))
	case Policy_Min:
		p.Set(float64(rc.Value()))
	case Policy_Avg, Policy_Stopwatch:
		v, c := rc.RawData()
		p.value += float64(v)
		p.cnt += c
		if p.cnt <= 0 {
			return fmt.Errorf("metrics(%s) count invalid", rc.Metrics().Name())
		}
		p.Set(p.value / float64(p.cnt))
	default:
		return fmt.Errorf("metrics(%s) policy invalid", rc.Metrics().Name())
	}
	return nil
}

// newPromCounter creates a new Prometheus counter from a metric record.
func newPromCounter(rc *Record, extLabels map[string]string) *metricWrapper {
	o := newMetricOpt(rc, extLabels)
	opts := prometheus.CounterOpts{
		Subsystem:   o.subsystem,
		Name:        o.name,
		ConstLabels: o.constLabels,
	}

	c := promauto.NewCounter(opts)
	c.Add(float64(rc.Value()))
	return &metricWrapper{
		m:  c,
		mt: _metricTypeCounter,
	}
}

// metricWrapper wraps Prometheus metrics since Counter and Gauge interfaces are similar.
// We only need one wrapper structure to store metrics and their types.
type metricWrapper struct {
	m  prometheus.Metric
	mt metricType
}

// merge updates the wrapped metric with new record data.
func (m *metricWrapper) merge(rc *Record) {
	convertSuc := false
	switch m.mt {
	case _metricTypeGauge:
		if c, ok := m.m.(*promGauge); ok && c != nil {
			convertSuc = true
			if err := c.merge(rc); err != nil {
				log.Error().Err(err).Msg("prometheus merge")
			}
		}
	case _metricTypeCounter:
		if c, ok := m.m.(prometheus.Counter); ok && c != nil {
			convertSuc = true
			c.Add(float64(rc.Value()))
		}
	}

	if !convertSuc {
		log.Error().Str("promtype", fmt.Sprintf("%T", m.m)).
			Int("metrictype", int(m.mt)).Msg("prometheus merge failed")
	}
}

// PrometheusReporterConfig contains configuration for the Prometheus reporter.
type PrometheusReporterConfig struct {
	Tag               string            `mapmapstructure:"tag"`               // Service tag
	PushAddr          string            `mapmapstructure:"pushAddr"`          // Push gateway address
	PushIntervalSec   int               `mapmapstructure:"pushIntervalSec"`   // Push interval in seconds
	PushJobName       string            `mapmapstructure:"pushJobName"`       // Push job name
	UsePush           bool              `mapmapstructure:"usePush"`           // Enable push mode
	HTTPListenIP      string            `mapmapstructure:"httpListenIP"`      // HTTP server listen IP
	MetricPath        string            `mapmapstructure:"metricPath"`        // Metrics HTTP path
	ExtLabels         map[string]string `mapmapstructure:"extLabels"`         // External labels
	extLabelsStr      string            `mapmapstructure:"tag"`               // External labels string
	EnableHealthCheck bool              `mapmapstructure:"enableHealthCheck"` // Enable health check
	HealthCheckPath   string            `mapmapstructure:"healthCheckPath"`   // Health check path
}

// GetExtLabelsStr returns the external labels string.
// This method provides thread-safe access to the cached external labels string
// that is used for metric identification and grouping.
func (x *PrometheusReporterConfig) GetExtLabelsStr() string {
	return x.extLabelsStr
}

// PrometheusReporter implements a Prometheus metrics reporter that converts
// application metrics to Prometheus format and exposes them via HTTP or push gateway.
type PrometheusReporter struct {
	cfg               *PrometheusReporterConfig
	promSvr           *http.Server
	pusher            *push.Pusher
	metricsChan       chan Record
	metrics           map[string]*metricWrapper
	ctx               context.Context
	cancel            context.CancelFunc
	healthCheckTicker *time.Ticker // Health check timer
	lastHealthCheck   time.Time    // Last health check time
	healthStatus      int32        // Health status (0=healthy, 1=unhealthy)
}

// NewPrometheusReporter creates a new Prometheus reporter instance.
func NewPrometheusReporter() *PrometheusReporter {
	ctx, cancel := context.WithCancel(context.Background())
	p := &PrometheusReporter{
		cfg: &PrometheusReporterConfig{
			HealthCheckPath: "/health", // 默认健康检查路径
		},
		metricsChan:  make(chan Record, _metricsChanSize),
		metrics:      map[string]*metricWrapper{},
		ctx:          ctx,
		cancel:       cancel,
		healthStatus: 0, // 初始状态为健康
	}

	p.start()
	return p
}

// Report 插件上报.
func (x *PrometheusReporter) Report(r Record) {
	select {
	case x.metricsChan <- r:
	default:
		log.Error().Msg("metrics chan full")
	}
}

func (x *PrometheusReporter) start() error {
	x.startAggregate()
	if x.cfg.UsePush {
		x.startPusher()
	}

	_, err1 := x.startHTTPSvr()
	if err1 != nil {
		return err1
	}

	x.startHealthCheck()

	return nil
}

func (x *PrometheusReporter) Stop() {
	if x.cancel != nil {
		x.cancel()
		x.cancel = nil
	}

	// 停止健康检查定时器
	if x.healthCheckTicker != nil {
		x.healthCheckTicker.Stop()
		x.healthCheckTicker = nil
	}

	if x.promSvr != nil {
		if err := x.promSvr.Close(); err != nil {
			log.Error().Err(err).Msg("stop PromHttpSvr stop")
		}
		x.promSvr = nil
	}
}

func (x *PrometheusReporter) startPusher() {
	x.pusher = push.New(x.cfg.PushAddr, x.cfg.PushJobName)
	x.pusher.Gatherer(prometheus.DefaultGatherer)
	go func() {
		log.Info().Msg("prometheus pusher stared")
		t := time.NewTicker(time.Second * time.Duration(x.cfg.PushIntervalSec))
		defer t.Stop()
		for {
			select {
			case <-x.ctx.Done():
				log.Info().Msg("prometheus pusher end")
				return
			case <-t.C:
				newCtx, cancel := context.WithTimeout(x.ctx, time.Second*5)
				if err := x.pusher.PushContext(newCtx); err != nil {
					log.Error().Err(err).End()
				}
				cancel()
			}
		}
	}()
}

// startHTTPSvr starts the Prometheus HTTP server for exposing
// It creates a TCP listener on a random available port, sets up HTTP handlers
// for metrics endpoint and optional health check, and starts serving requests.
// Returns the network address the server is listening on, or an error if setup fails.
func (x *PrometheusReporter) startHTTPSvr() (net.Addr, error) {
	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: nil, Port: 0}) //nolint:gosec
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle(x.cfg.MetricPath, promhttp.Handler())

	// 添加健康检查处理器
	if x.cfg.EnableHealthCheck {
		mux.HandleFunc(x.cfg.HealthCheckPath, x.healthCheckHandler)
		log.Info().Str("path", x.cfg.HealthCheckPath).Msg("health check endpoint enabled")
	}

	x.promSvr = &http.Server{Handler: mux} //nolint:gosec
	go x.promSvr.Serve(l)
	log.Info().Str("url", path.Join(l.Addr().String(), x.cfg.MetricPath)).Msg("prometheus http start listen on")

	return l.Addr(), nil
}

// startAggregate starts the metrics aggregation goroutine.
// It continuously processes incoming metrics records from the channel,
// merging them into the internal storage until the context is cancelled.
func (x *PrometheusReporter) startAggregate() {
	go func() {
		log.Info().Msg("prometheus collector begin")
		for {
			select {
			case rc := <-x.metricsChan:
				x.merge(&rc)
			case <-x.ctx.Done():
				log.Info().Msg("prometheus collector shutdown")
				return
			}
		}
	}()
}

// healthCheckHandler handles HTTP health check requests.
// It responds with service health status, timestamp, and uptime information.
// Returns HTTP 200 for healthy status, HTTP 503 for unhealthy status.
func (x *PrometheusReporter) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 检查健康状态
	if atomic.LoadInt32(&x.healthStatus) == 0 {
		// 更新最后检查时间
		x.lastHealthCheck = time.Now()

		response := map[string]interface{}{
			"status":    "healthy",
			"timestamp": x.lastHealthCheck.Format(time.RFC3339),
			"service":   _serviceName,
			"uptime":    time.Since(x.lastHealthCheck).String(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	} else {
		response := map[string]interface{}{
			"status":    "unhealthy",
			"timestamp": time.Now().Format(time.RFC3339),
			"service":   _serviceName,
			"message":   "Metrics reporter is experiencing issues",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(response)
	}
}

// startHealthCheck starts the TTL health check system.
// It initializes a ticker for periodic health checks and launches a goroutine
// that monitors the health status of the metrics reporter.
func (x *PrometheusReporter) startHealthCheck() {
	if !x.cfg.EnableHealthCheck {
		return
	}

	x.healthCheckTicker = time.NewTicker(_healthCheckInterval)
	x.lastHealthCheck = time.Now()

	go func() {
		log.Info().Float64("interval_seconds", _healthCheckInterval.Seconds()).Msg("TTL health check started")
		for {
			select {
			case <-x.ctx.Done():
				if x.healthCheckTicker != nil {
					x.healthCheckTicker.Stop()
				}
				log.Info().Msg("TTL health check stopped")
				return
			case <-x.healthCheckTicker.C:
				x.performHealthCheck()
			}
		}
	}()
}

// performHealthCheck executes the health check logic.
// It checks metrics channel usage and time since last health update to determine
// the current health status of the metrics reporter.
func (x *PrometheusReporter) performHealthCheck() {
	// 检查metrics channel状态
	chanUsage := float64(len(x.metricsChan)) / float64(cap(x.metricsChan))

	// 检查最后处理时间
	timeSinceLastCheck := time.Since(x.lastHealthCheck)

	// 判断健康状态
	if chanUsage > 0.9 { // Channel使用率超过90%
		atomic.StoreInt32(&x.healthStatus, 1)
		log.Warn().
			Float64("chan_usage", chanUsage).
			Float64("since_last_check_seconds", timeSinceLastCheck.Seconds()).
			Msg("Health check failed - high channel usage")
	} else if timeSinceLastCheck > _healthCheckInterval*2 { // 超过2个检查周期没有更新
		atomic.StoreInt32(&x.healthStatus, 1)
		log.Warn().
			Float64("since_last_check_seconds", timeSinceLastCheck.Seconds()).
			Msg("Health check failed - no recent health updates")
	} else {
		atomic.StoreInt32(&x.healthStatus, 0)
		log.Debug().
			Float64("chan_usage", chanUsage).
			Float64("since_last_check_seconds", timeSinceLastCheck.Seconds()).
			Msg("Health check passed")
	}

	// 更新最后检查时间
	x.lastHealthCheck = time.Now()
}

// merge combines a metrics record into the internal storage.
// It either updates an existing metric with the same key or creates a new one
// based on the metric type (Counter, StopWatch, or Gauge).
//
//nolint:gocritic,staticcheck
func (x *PrometheusReporter) merge(rc *Record) {
	key := x.getFullName(rc)
	if m, exist := x.metrics[key]; exist {
		m.merge(rc)
		return
	}
	switch m := rc.Metrics().(type) {
	case Counter:
		c := newPromCounter(rc, x.cfg.ExtLabels)
		x.metrics[key] = c
	case StopWatch, Gauge:
		g := newPromGauga(rc, x.cfg.ExtLabels)
		x.metrics[key] = g
	default:
		log.Error().Str("merictype", fmt.Sprintf("%T", m)).Msg("prometheus merge unkonwn")
	}
}

// getFullName generates a unique key for a metrics record.
// It combines the metric group, name, external labels, and dimensions into a single string
// to uniquely identify the metric for storage and retrieval.
func (x *PrometheusReporter) getFullName(rc *Record) string {
	var sb strings.Builder
	sb.Grow(256)
	sb.WriteString(rc.Metrics().Group())
	sb.WriteString("*")
	sb.WriteString(rc.Metrics().Name())
	sb.WriteString("*")
	sb.WriteString(x.cfg.GetExtLabelsStr())
	type kv struct {
		key   string
		value string
	}
	keys := make([]*kv, 0, len(rc.Dimensions()))
	for k, v := range rc.Dimensions() {
		if _, ok := x.cfg.ExtLabels[k]; ok {
			continue
		}
		keys = append(keys, &kv{
			key:   k,
			value: v,
		})
	}
	sort.Slice(keys, func(a, b int) bool {
		return keys[a].key < keys[b].key
	})
	for _, v := range keys {
		sb.WriteString(v.key)
		sb.WriteString(":")
		sb.WriteString(v.value)
		sb.WriteString(",")
	}
	return sb.String()
}
