// Package prometheus 提供 Prometheus 指标上报能力。
// 该包负责将应用指标转换为 Prometheus 格式，
// 并通过 HTTP 暴露或 PushGateway 推送。
package prometheus

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
	"github.com/linchenxuan/strix/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/push"
)

const (
	_metricsChanSize     = 1000000
	_serviceName         = "exporter"
	_healthCheckInterval = 30 * time.Second // TTL 健康检查间隔
)

// metricType 表示 Prometheus 指标类型。
type metricType int

const (
	_metricTypeCounter metricType = iota
	_metricTypeGauge
	_metricTypeHistogram
	_metricTypeSummary
)

// metricOpt 封装创建 Prometheus 指标所需的选项。
type metricOpt struct {
	subsystem   string
	name        string
	constLabels map[string]string
}

func normalizePromMetricIdent(value string) string {
	return strings.ReplaceAll(value, ".", "_")
}

func normalizePromLabelValue(value string) string {
	return strings.ReplaceAll(value, ".", "_")
}

func buildSortedLabelString(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(":")
		sb.WriteString(labels[k])
		sb.WriteString(",")
	}
	return sb.String()
}

func buildMergedLabels(
	dimensions map[string]string,
	extLabels map[string]string,
	metricName string,
	groupName string,
) map[string]string {
	labels := make(map[string]string, len(dimensions)+len(extLabels))
	for k, v := range extLabels {
		labels[k] = normalizePromLabelValue(v)
	}
	for k, v := range dimensions {
		newValue := normalizePromLabelValue(v)
		if oldValue, exists := labels[k]; exists {
			log.Warn().
				Str("metric", metricName).
				Str("group", groupName).
				Str("label", k).
				Str("extValue", oldValue).
				Str("dimValue", newValue).
				Msg("prometheus label overridden by dimensions")
		}
		labels[k] = newValue
	}
	return labels
}

// newMetricOpt 基于指标记录和外部标签构造指标选项。
func newMetricOpt(rc *metrics.Record, extLabels map[string]string) *metricOpt {
	constLabels := buildMergedLabels(rc.Dimensions(), extLabels, rc.Metrics().Name(), rc.Metrics().Group())
	opts := &metricOpt{
		subsystem:   normalizePromMetricIdent(rc.Metrics().Group()),
		name:        normalizePromMetricIdent(rc.Metrics().Name()),
		constLabels: constLabels,
	}
	return opts
}

// promGauge 对 Prometheus Gauge 进行封装，并额外记录平均值所需的累计数据。
type promGauge struct {
	prometheus.Gauge
	value       float64 // 用于平均值计算的累计值
	cnt         int     // 样本计数
	current     float64 // 当前 gauge 值（用于 max/min 聚合）
	initialized bool    // 是否已初始化 current
}

// newPromGauga 根据指标记录创建新的 Gauge 包装器。
func newPromGauga(rc *metrics.Record, extLabels map[string]string) *metricWrapper {
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

// merge 按指标策略更新 gauge 值。
func (p *promGauge) merge(rc *metrics.Record) error {
	v := float64(rc.Value())
	switch rc.Metrics().Policy() {
	case metrics.Policy_Set:
		p.current = v
		p.initialized = true
		p.Set(v)
	case metrics.Policy_Sum:
		if !p.initialized {
			p.current = 0
			p.initialized = true
		}
		p.current += v
		p.Add(v)
	case metrics.Policy_Max:
		if !p.initialized || v > p.current {
			p.current = v
			p.initialized = true
			p.Set(v)
		}
	case metrics.Policy_Min:
		if !p.initialized || v < p.current {
			p.current = v
			p.initialized = true
			p.Set(v)
		}
	case metrics.Policy_Avg, metrics.Policy_Stopwatch:
		v, c := rc.RawData()
		p.value += float64(v)
		p.cnt += c
		if p.cnt <= 0 {
			return fmt.Errorf("metrics(%s) count invalid", rc.Metrics().Name())
		}
		avg := p.value / float64(p.cnt)
		p.current = avg
		p.initialized = true
		p.Set(avg)
	default:
		return fmt.Errorf("metrics(%s) policy invalid", rc.Metrics().Name())
	}
	return nil
}

// newPromCounter 根据指标记录创建新的 Prometheus Counter。
func newPromCounter(rc *metrics.Record, extLabels map[string]string) *metricWrapper {
	o := newMetricOpt(rc, extLabels)
	opts := prometheus.CounterOpts{
		Subsystem:   o.subsystem,
		Name:        o.name,
		ConstLabels: o.constLabels,
	}

	c := promauto.NewCounter(opts)
	addCounterValue(c, rc)
	return &metricWrapper{
		m:  c,
		mt: _metricTypeCounter,
	}
}

func addCounterValue(c prometheus.Counter, rc *metrics.Record) {
	v := float64(rc.Value())
	if v < 0 {
		log.Error().
			Str("metric", rc.Metrics().Name()).
			Str("group", rc.Metrics().Group()).
			Float64("value", v).
			Msg("prometheus counter ignores negative value")
		return
	}
	c.Add(v)
}

// metricWrapper 封装 Prometheus 指标对象。
// Counter 与 Gauge 处理流程类似，因此统一用该结构承载实例与类型信息。
type metricWrapper struct {
	m  prometheus.Metric
	mt metricType
}

// merge 使用新记录更新已封装的 Prometheus 指标。
func (m *metricWrapper) merge(rc *metrics.Record) {
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
			addCounterValue(c, rc)
		}
	}

	if !convertSuc {
		log.Error().Str("promtype", fmt.Sprintf("%T", m.m)).
			Int("metrictype", int(m.mt)).Msg("prometheus merge failed")
	}
}

// PrometheusReporterConfig 定义 Prometheus reporter 配置项。
type PrometheusReporterConfig struct {
	Tag               string            `mapstructure:"tag"`               // 服务标签
	PushAddr          string            `mapstructure:"pushAddr"`          // PushGateway 地址
	PushIntervalSec   int               `mapstructure:"pushIntervalSec"`   // 推送间隔（秒）
	PushJobName       string            `mapstructure:"pushJobName"`       // Push job 名称
	UsePush           bool              `mapstructure:"usePush"`           // 是否启用 push 模式
	HTTPListenIP      string            `mapstructure:"httpListenIP"`      // HTTP 服务监听 IP
	MetricPath        string            `mapstructure:"metricPath"`        // 指标 HTTP 路径
	ExtLabels         map[string]string `mapstructure:"extLabels"`         // 外部标签
	EnableHealthCheck bool              `mapstructure:"enableHealthCheck"` // 是否启用健康检查
	HealthCheckPath   string            `mapstructure:"healthCheckPath"`   // 健康检查路径
}

// GetExtLabelsStr 返回外部标签缓存串。
// 该值用于指标唯一键构造与分组识别。
func (x *PrometheusReporterConfig) GetExtLabelsStr() string {
	labels := make(map[string]string, len(x.ExtLabels))
	for k, v := range x.ExtLabels {
		labels[k] = normalizePromLabelValue(v)
	}
	return buildSortedLabelString(labels)
}

// PrometheusReporter 实现 Prometheus 指标上报器。
// 它将应用指标转换为 Prometheus 格式，并通过 HTTP 或 PushGateway 输出。
type PrometheusReporter struct {
	cfg               *PrometheusReporterConfig
	promSvr           *http.Server
	pusher            *push.Pusher
	metricsChan       chan metrics.Record
	metrics           map[string]*metricWrapper
	ctx               context.Context
	cancel            context.CancelFunc
	healthCheckTicker *time.Ticker // 健康检查定时器
	startTime         time.Time    // reporter 启动时间
	lastMetricAt      int64        // 最近一次处理指标的时间（UnixNano）
	healthStatus      int32        // 健康状态（0=健康，1=不健康）
}

// NewPrometheusReporter 创建 Prometheus reporter 实例。
func NewPrometheusReporter() *PrometheusReporter {
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now()
	p := &PrometheusReporter{
		cfg: &PrometheusReporterConfig{
			HealthCheckPath: "/health", // 默认健康检查路径
		},
		metricsChan:  make(chan metrics.Record, _metricsChanSize),
		metrics:      map[string]*metricWrapper{},
		ctx:          ctx,
		cancel:       cancel,
		startTime:    now,
		lastMetricAt: now.UnixNano(),
		healthStatus: 0, // 初始为健康状态
	}

	return p
}

func (x *PrometheusReporter) FactoryName() string {
	return "prometheus"
}

// Report 上报一条指标记录。
func (x *PrometheusReporter) Report(r metrics.Record) {
	select {
	case x.metricsChan <- r:
	default:
		log.Error().Msg("metrics chan full")
	}
}

func (x *PrometheusReporter) start() error {
	now := time.Now()
	x.startTime = now
	atomic.StoreInt64(&x.lastMetricAt, now.UnixNano())

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

func (x *PrometheusReporter) stop() {
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

// startHTTPSvr 启动 Prometheus HTTP 服务。
// 该方法会监听随机可用端口，注册指标和可选健康检查路由，并开始提供服务。
// 返回监听地址；若启动失败则返回错误。
func (x *PrometheusReporter) startHTTPSvr() (net.Addr, error) {
	metricPath := strings.TrimSpace(x.cfg.MetricPath)
	if metricPath == "" {
		return nil, fmt.Errorf("metricPath is required")
	}
	if !strings.HasPrefix(metricPath, "/") {
		metricPath = "/" + metricPath
	}

	var listenIP net.IP
	listenIPStr := strings.TrimSpace(x.cfg.HTTPListenIP)
	if listenIPStr != "" {
		listenIP = net.ParseIP(listenIPStr)
		if listenIP == nil {
			return nil, fmt.Errorf("invalid httpListenIP: %s", listenIPStr)
		}
	}

	l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: listenIP, Port: 0})
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle(metricPath, promhttp.Handler())

	// 添加健康检查处理器
	if x.cfg.EnableHealthCheck {
		healthPath := strings.TrimSpace(x.cfg.HealthCheckPath)
		if healthPath == "" {
			_ = l.Close()
			return nil, fmt.Errorf("healthCheckPath is required when health check is enabled")
		}
		if !strings.HasPrefix(healthPath, "/") {
			healthPath = "/" + healthPath
		}
		mux.HandleFunc(healthPath, x.healthCheckHandler)
		log.Info().Str("path", healthPath).Msg("health check endpoint enabled")
	}

	x.promSvr = &http.Server{Handler: mux} //nolint:gosec
	go x.promSvr.Serve(l)
	log.Info().Str("url", path.Join(l.Addr().String(), metricPath)).Msg("prometheus http start listen on")

	return l.Addr(), nil
}

// startAggregate 启动指标聚合协程。
// 该协程持续消费 metricsChan 中的数据并合并到内部存储，
// 直到上下文被取消。
func (x *PrometheusReporter) startAggregate() {
	go func() {
		log.Info().Msg("prometheus collector begin")
		for {
			select {
			case rc := <-x.metricsChan:
				atomic.StoreInt64(&x.lastMetricAt, time.Now().UnixNano())
				x.merge(&rc)
			case <-x.ctx.Done():
				log.Info().Msg("prometheus collector shutdown")
				return
			}
		}
	}()
}

// healthCheckHandler 处理 HTTP 健康检查请求。
// 健康时返回 200，不健康时返回 503，并附带状态与时间信息。
func (x *PrometheusReporter) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 检查健康状态
	if atomic.LoadInt32(&x.healthStatus) == 0 {
		now := time.Now()
		lastMetricAt := time.Unix(0, atomic.LoadInt64(&x.lastMetricAt))
		uptime := now.Sub(x.startTime)
		if uptime < 0 {
			uptime = 0
		}

		response := map[string]interface{}{
			"status":     "healthy",
			"timestamp":  now.Format(time.RFC3339),
			"service":    _serviceName,
			"uptime":     uptime.String(),
			"lastMetric": lastMetricAt.Format(time.RFC3339),
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

// startHealthCheck 启动 TTL 健康检查流程。
// 方法会创建定时器并启动协程，周期性检测 reporter 运行状态。
func (x *PrometheusReporter) startHealthCheck() {
	if !x.cfg.EnableHealthCheck {
		return
	}

	x.healthCheckTicker = time.NewTicker(_healthCheckInterval)

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

// performHealthCheck 执行健康检查逻辑。
// 通过通道占用率和最近一次指标处理时间判断当前健康状态。
func (x *PrometheusReporter) performHealthCheck() {
	// 检查指标通道状态
	chanUsage := float64(len(x.metricsChan)) / float64(cap(x.metricsChan))

	// 检查最近一次处理指标的时间
	lastMetricAt := time.Unix(0, atomic.LoadInt64(&x.lastMetricAt))
	timeSinceLastMetric := time.Since(lastMetricAt)

	// 判断健康状态
	if chanUsage > 0.9 { // Channel使用率超过90%
		atomic.StoreInt32(&x.healthStatus, 1)
		log.Warn().
			Float64("chan_usage", chanUsage).
			Float64("since_last_metric_seconds", timeSinceLastMetric.Seconds()).
			Msg("Health check failed - high channel usage")
	} else if timeSinceLastMetric > _healthCheckInterval*2 { // 超过2个检查周期没有处理指标
		atomic.StoreInt32(&x.healthStatus, 1)
		log.Warn().
			Float64("since_last_metric_seconds", timeSinceLastMetric.Seconds()).
			Msg("Health check failed - no recent metric updates")
	} else {
		atomic.StoreInt32(&x.healthStatus, 0)
		log.Debug().
			Float64("chan_usage", chanUsage).
			Float64("since_last_metric_seconds", timeSinceLastMetric.Seconds()).
			Msg("Health check passed")
	}
}

// merge 将指标记录合并到内部存储。
// 若已存在相同 key 则更新，否则按指标类型创建新指标对象。
//
//nolint:gocritic,staticcheck
func (x *PrometheusReporter) merge(rc *metrics.Record) {
	key := x.getFullName(rc)
	if m, exist := x.metrics[key]; exist {
		m.merge(rc)
		return
	}
	switch m := rc.Metrics().(type) {
	case metrics.Counter:
		c := newPromCounter(rc, x.cfg.ExtLabels)
		x.metrics[key] = c
	case metrics.StopWatch, metrics.Gauge:
		g := newPromGauga(rc, x.cfg.ExtLabels)
		x.metrics[key] = g
	default:
		log.Error().Str("merictype", fmt.Sprintf("%T", m)).Msg("prometheus merge unkonwn")
	}
}

// getFullName 为一条指标记录生成唯一键。
// 键由分组、名称、外部标签和维度拼接而成，用于内部存取定位。
func (x *PrometheusReporter) getFullName(rc *metrics.Record) string {
	var sb strings.Builder
	sb.Grow(256)
	sb.WriteString(normalizePromMetricIdent(rc.Metrics().Group()))
	sb.WriteString("*")
	sb.WriteString(normalizePromMetricIdent(rc.Metrics().Name()))
	sb.WriteString("*")
	labels := buildMergedLabels(rc.Dimensions(), x.cfg.ExtLabels, rc.Metrics().Name(), rc.Metrics().Group())
	sb.WriteString(buildSortedLabelString(labels))
	return sb.String()
}
