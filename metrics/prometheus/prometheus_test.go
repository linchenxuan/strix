package prometheus

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/linchenxuan/strix/metrics"
	"github.com/linchenxuan/strix/plugin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type testMetric struct {
	name   string
	group  string
	policy metrics.Policy
}

func (m testMetric) Name() string           { return m.name }
func (m testMetric) Group() string          { return m.group }
func (m testMetric) Policy() metrics.Policy { return m.policy }

type testCounter struct {
	testMetric
}

func (c testCounter) IncrWithDim(delta metrics.Value, dimensions metrics.Dimension) {}
func (c testCounter) Incr(delta metrics.Value)                                      {}

type testGauge struct {
	testMetric
}

func (g testGauge) Update(value metrics.Value)                                      {}
func (g testGauge) UpdateWithDim(value metrics.Value, dimensions metrics.Dimension) {}

type invalidPlugin struct{}

func (invalidPlugin) FactoryName() string { return "invalid" }

func makeRecord(m metrics.Metrics, v metrics.Value, dim metrics.Dimension) metrics.Record {
	var r metrics.Record
	r.SetMetrics(m)
	r.SetValue(v)
	r.SetDimension(dim)
	return r
}

func uniqueMetricName(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func TestFactoryBasics(t *testing.T) {
	f := &factory{}
	if f.Type() != plugin.Type("metrics") {
		t.Fatalf("unexpected type: %s", f.Type())
	}
	if f.Name() != "prometheus" {
		t.Fatalf("unexpected name: %s", f.Name())
	}
	if _, ok := f.ConfigType().(*PrometheusReporterConfig); !ok {
		t.Fatal("ConfigType should return *PrometheusReporterConfig")
	}
}

func TestFactorySetupErrorHandling(t *testing.T) {
	f := &factory{}

	if _, err := f.Setup(struct{}{}); err == nil {
		t.Fatal("expected setup error for invalid config type")
	}

	var nilCfg *PrometheusReporterConfig
	if _, err := f.Setup(nilCfg); err == nil {
		t.Fatal("expected setup error for nil config")
	}

	if _, err := f.Setup(&PrometheusReporterConfig{}); err == nil {
		t.Fatal("expected setup error for missing metricPath")
	}

	if _, err := f.Setup(&PrometheusReporterConfig{
		MetricPath:        "/metrics",
		EnableHealthCheck: true,
	}); err == nil {
		t.Fatal("expected setup error for missing healthCheckPath when health check is enabled")
	}

	if _, err := f.Setup(&PrometheusReporterConfig{
		MetricPath:      "/metrics",
		UsePush:         true,
		PushAddr:        "http://127.0.0.1:9091",
		PushIntervalSec: 10,
	}); err == nil {
		t.Fatal("expected setup error for missing pushJobName")
	}

	if _, err := f.Setup(&PrometheusReporterConfig{
		MetricPath:      "/metrics",
		UsePush:         true,
		PushAddr:        "http://127.0.0.1:9091",
		PushIntervalSec: 0,
	}); err == nil {
		t.Fatal("expected setup error for invalid push interval")
	}
}

func TestFactoryDestroyInvalidType(t *testing.T) {
	f := &factory{}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("destroy should not panic, got: %v", r)
		}
	}()
	f.Destroy(invalidPlugin{})
}

func TestNewMetricOpt(t *testing.T) {
	r := makeRecord(
		testMetric{name: "rpc.cost", group: "net.dispatcher", policy: metrics.Policy_Set},
		1,
		metrics.Dimension{
			"zone":   "cn.hz",
			"shared": "from.dim",
		},
	)

	opt := newMetricOpt(&r, map[string]string{
		"env":    "prod.a",
		"shared": "from.ext",
	})

	if opt.subsystem != "net_dispatcher" {
		t.Fatalf("unexpected subsystem: %s", opt.subsystem)
	}
	if opt.name != "rpc_cost" {
		t.Fatalf("unexpected name: %s", opt.name)
	}
	if opt.constLabels["env"] != "prod_a" {
		t.Fatalf("unexpected env label: %s", opt.constLabels["env"])
	}
	if opt.constLabels["zone"] != "cn_hz" {
		t.Fatalf("unexpected zone label: %s", opt.constLabels["zone"])
	}
	if opt.constLabels["shared"] != "from_dim" {
		t.Fatalf("dimension label should override ext label, got: %s", opt.constLabels["shared"])
	}
}

func TestGetFullNameSortAndFilter(t *testing.T) {
	p := NewPrometheusReporter()
	p.cfg.ExtLabels = map[string]string{
		"region": "cn",
		"drop":   "1",
	}

	r := makeRecord(
		testMetric{name: "recv_total", group: "net.dispatcher", policy: metrics.Policy_Set},
		1,
		metrics.Dimension{
			"b":    "2",
			"a":    "1",
			"drop": "x",
		},
	)

	key := p.getFullName(&r)
	want := "net_dispatcher*recv_total*a:1,b:2,drop:x,region:cn,"
	if key != want {
		t.Fatalf("unexpected key\nwant: %s\ngot:  %s", want, key)
	}
}

func TestHealthCheckHandler(t *testing.T) {
	p := NewPrometheusReporter()

	// 非 GET 请求应返回 405。
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	p.healthCheckHandler(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status for non-GET request: %d", rr.Code)
	}

	// 健康状态应返回 200。
	atomic.StoreInt32(&p.healthStatus, 0)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/health", nil)
	p.healthCheckHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status for healthy request: %d", rr.Code)
	}
	var healthy map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &healthy); err != nil {
		t.Fatalf("unmarshal healthy response failed: %v", err)
	}
	if healthy["status"] != "healthy" {
		t.Fatalf("unexpected healthy status: %v", healthy["status"])
	}
	uptimeRaw, ok := healthy["uptime"].(string)
	if !ok || uptimeRaw == "" {
		t.Fatalf("expected uptime in healthy response, got: %v", healthy["uptime"])
	}
	lastMetricRaw, ok := healthy["lastMetric"].(string)
	if !ok || lastMetricRaw == "" {
		t.Fatalf("expected lastMetric in healthy response, got: %v", healthy["lastMetric"])
	}

	// 不健康状态应返回 503。
	atomic.StoreInt32(&p.healthStatus, 1)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/health", nil)
	p.healthCheckHandler(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status for unhealthy request: %d", rr.Code)
	}
	var unhealthy map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &unhealthy); err != nil {
		t.Fatalf("unmarshal unhealthy response failed: %v", err)
	}
	if unhealthy["status"] != "unhealthy" {
		t.Fatalf("unexpected unhealthy status: %v", unhealthy["status"])
	}
}

func TestPerformHealthCheck(t *testing.T) {
	// 通道占用超过阈值，应判定不健康。
	p1 := NewPrometheusReporter()
	p1.metricsChan = make(chan metrics.Record, 10)
	for i := 0; i < cap(p1.metricsChan); i++ {
		p1.metricsChan <- metrics.Record{}
	}
	atomic.StoreInt64(&p1.lastMetricAt, time.Now().UnixNano())
	p1.performHealthCheck()
	if atomic.LoadInt32(&p1.healthStatus) != 1 {
		t.Fatal("expected unhealthy status when channel usage is high")
	}

	// 长时间无更新，应判定不健康。
	p2 := NewPrometheusReporter()
	p2.metricsChan = make(chan metrics.Record, 10)
	atomic.StoreInt64(&p2.lastMetricAt, time.Now().Add(-3*_healthCheckInterval).UnixNano())
	p2.performHealthCheck()
	if atomic.LoadInt32(&p2.healthStatus) != 1 {
		t.Fatal("expected unhealthy status when last check is too old")
	}

	// 正常情况应判定健康。
	p3 := NewPrometheusReporter()
	p3.metricsChan = make(chan metrics.Record, 10)
	atomic.StoreInt64(&p3.lastMetricAt, time.Now().UnixNano())
	p3.performHealthCheck()
	if atomic.LoadInt32(&p3.healthStatus) != 0 {
		t.Fatal("expected healthy status in normal case")
	}
}

func TestStartHTTPSvrValidation(t *testing.T) {
	p1 := NewPrometheusReporter()
	p1.cfg.MetricPath = ""
	if _, err := p1.startHTTPSvr(); err == nil {
		t.Fatal("expected error when metricPath is empty")
	}

	p2 := NewPrometheusReporter()
	p2.cfg.MetricPath = "/metrics"
	p2.cfg.HTTPListenIP = "invalid-ip"
	if _, err := p2.startHTTPSvr(); err == nil {
		t.Fatal("expected error when httpListenIP is invalid")
	}

	p3 := NewPrometheusReporter()
	p3.cfg.MetricPath = "/metrics"
	p3.cfg.HTTPListenIP = "127.0.0.1"
	addr, err := p3.startHTTPSvr()
	if err != nil {
		t.Fatalf("expected startHTTPSvr success, got error: %v", err)
	}
	defer p3.stop()
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		t.Fatalf("expected tcp address, got: %T", addr)
	}
	if !tcpAddr.IP.Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("expected listen ip 127.0.0.1, got: %s", tcpAddr.IP.String())
	}
}

func TestMergeCounterAndGauge(t *testing.T) {
	p := NewPrometheusReporter()
	p.cfg.ExtLabels = map[string]string{}

	counterMetric := testCounter{
		testMetric: testMetric{
			name:   uniqueMetricName("counter"),
			group:  "strix",
			policy: metrics.Policy_Sum,
		},
	}
	c1 := makeRecord(counterMetric, 1, metrics.Dimension{"k": "v"})
	c2 := makeRecord(counterMetric, 2, metrics.Dimension{"k": "v"})

	p.merge(&c1)
	p.merge(&c2)

	counterKey := p.getFullName(&c1)
	counterWrapper, ok := p.metrics[counterKey]
	if !ok {
		t.Fatal("counter metric wrapper not found")
	}
	if counterWrapper.mt != _metricTypeCounter {
		t.Fatalf("unexpected counter metric type: %v", counterWrapper.mt)
	}
	counterCollector, ok := counterWrapper.m.(prometheus.Counter)
	if !ok {
		t.Fatalf("unexpected counter collector type: %T", counterWrapper.m)
	}
	if got := testutil.ToFloat64(counterCollector); got != 3 {
		t.Fatalf("unexpected counter value: %v", got)
	}

	gaugeMetric := testGauge{
		testMetric: testMetric{
			name:   uniqueMetricName("gauge"),
			group:  "strix",
			policy: metrics.Policy_Set,
		},
	}
	g1 := makeRecord(gaugeMetric, 10, metrics.Dimension{"k": "v"})
	g2 := makeRecord(gaugeMetric, 25, metrics.Dimension{"k": "v"})

	p.merge(&g1)
	p.merge(&g2)

	gaugeKey := p.getFullName(&g1)
	gaugeWrapper, ok := p.metrics[gaugeKey]
	if !ok {
		t.Fatal("gauge metric wrapper not found")
	}
	if gaugeWrapper.mt != _metricTypeGauge {
		t.Fatalf("unexpected gauge metric type: %v", gaugeWrapper.mt)
	}
	pg, ok := gaugeWrapper.m.(*promGauge)
	if !ok {
		t.Fatalf("unexpected gauge collector type: %T", gaugeWrapper.m)
	}
	if got := testutil.ToFloat64(pg.Gauge); got != 25 {
		t.Fatalf("unexpected gauge value: %v", got)
	}
}

func TestMergeCounterNegativeValue(t *testing.T) {
	p := NewPrometheusReporter()
	p.cfg.ExtLabels = map[string]string{}

	counterMetric := testCounter{
		testMetric: testMetric{
			name:   uniqueMetricName("counter_negative"),
			group:  "strix",
			policy: metrics.Policy_Sum,
		},
	}

	r1 := makeRecord(counterMetric, 5, metrics.Dimension{"k": "v"})
	r2 := makeRecord(counterMetric, -3, metrics.Dimension{"k": "v"})

	p.merge(&r1)
	p.merge(&r2)

	key := p.getFullName(&r1)
	counterWrapper, ok := p.metrics[key]
	if !ok {
		t.Fatal("counter metric wrapper not found")
	}
	counterCollector, ok := counterWrapper.m.(prometheus.Counter)
	if !ok {
		t.Fatalf("unexpected counter collector type: %T", counterWrapper.m)
	}
	if got := testutil.ToFloat64(counterCollector); got != 5 {
		t.Fatalf("unexpected counter value: %v", got)
	}
}

func TestMergeGaugeMaxMinPolicy(t *testing.T) {
	p := NewPrometheusReporter()
	p.cfg.ExtLabels = map[string]string{}

	maxMetric := testGauge{
		testMetric: testMetric{
			name:   uniqueMetricName("max"),
			group:  "strix",
			policy: metrics.Policy_Max,
		},
	}
	maxRecords := []metrics.Record{
		makeRecord(maxMetric, 10, metrics.Dimension{"k": "v"}),
		makeRecord(maxMetric, 6, metrics.Dimension{"k": "v"}),
		makeRecord(maxMetric, 18, metrics.Dimension{"k": "v"}),
		makeRecord(maxMetric, 9, metrics.Dimension{"k": "v"}),
	}
	for i := range maxRecords {
		p.merge(&maxRecords[i])
	}
	maxKey := p.getFullName(&maxRecords[0])
	maxWrapper, ok := p.metrics[maxKey]
	if !ok {
		t.Fatal("max gauge metric wrapper not found")
	}
	maxPromGauge, ok := maxWrapper.m.(*promGauge)
	if !ok {
		t.Fatalf("unexpected max gauge collector type: %T", maxWrapper.m)
	}
	if got := testutil.ToFloat64(maxPromGauge.Gauge); got != 18 {
		t.Fatalf("unexpected max gauge value: %v", got)
	}

	minMetric := testGauge{
		testMetric: testMetric{
			name:   uniqueMetricName("min"),
			group:  "strix",
			policy: metrics.Policy_Min,
		},
	}
	minRecords := []metrics.Record{
		makeRecord(minMetric, 10, metrics.Dimension{"k": "v"}),
		makeRecord(minMetric, 6, metrics.Dimension{"k": "v"}),
		makeRecord(minMetric, 18, metrics.Dimension{"k": "v"}),
		makeRecord(minMetric, 9, metrics.Dimension{"k": "v"}),
	}
	for i := range minRecords {
		p.merge(&minRecords[i])
	}
	minKey := p.getFullName(&minRecords[0])
	minWrapper, ok := p.metrics[minKey]
	if !ok {
		t.Fatal("min gauge metric wrapper not found")
	}
	minPromGauge, ok := minWrapper.m.(*promGauge)
	if !ok {
		t.Fatalf("unexpected min gauge collector type: %T", minWrapper.m)
	}
	if got := testutil.ToFloat64(minPromGauge.Gauge); got != 6 {
		t.Fatalf("unexpected min gauge value: %v", got)
	}
}
