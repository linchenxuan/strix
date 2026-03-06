package metrics

import (
	"testing"
	"time"
)

type delayedReporter struct {
	gate    chan struct{}
	records chan Record
}

func (r *delayedReporter) Report(record Record) {
	go func(rc Record) {
		<-r.gate
		r.records <- rc
	}(record)
}

// TestMetricsCacheKeyIncludesGroup 验证同名不同分组的指标不会复用同一实例。
func TestMetricsCacheKeyIncludesGroup(t *testing.T) {
	_counters = make(map[string]Counter)
	_gauges = make(map[string]Gauge)
	_avggauges = make(map[string]Gauge)
	_maxGauges = make(map[string]Gauge)
	_minGauges = make(map[string]Gauge)
	_stopwatchs = make(map[string]StopWatch)
	defer func() {
		_counters = make(map[string]Counter)
		_gauges = make(map[string]Gauge)
		_avggauges = make(map[string]Gauge)
		_maxGauges = make(map[string]Gauge)
		_minGauges = make(map[string]Gauge)
		_stopwatchs = make(map[string]StopWatch)
	}()

	if getCounter("same_name", "group_a") == getCounter("same_name", "group_b") {
		t.Fatal("counter with same name but different group should not reuse instance")
	}
	if getGauge("same_name", "group_a") == getGauge("same_name", "group_b") {
		t.Fatal("gauge with same name but different group should not reuse instance")
	}
	if getAvgGauge("same_name", "group_a") == getAvgGauge("same_name", "group_b") {
		t.Fatal("avg gauge with same name but different group should not reuse instance")
	}
	if getMaxGauge("same_name", "group_a") == getMaxGauge("same_name", "group_b") {
		t.Fatal("max gauge with same name but different group should not reuse instance")
	}
	if getMinGauge("same_name", "group_a") == getMinGauge("same_name", "group_b") {
		t.Fatal("min gauge with same name but different group should not reuse instance")
	}
	if getStopWatch("same_name", "group_a") == getStopWatch("same_name", "group_b") {
		t.Fatal("stopwatch with same name but different group should not reuse instance")
	}
}

// TestDimensionCloneForAsyncReporter 验证维度会在上报前被深拷贝，避免异步 reporter 读到外部修改。
func TestDimensionCloneForAsyncReporter(t *testing.T) {
	reporter := &delayedReporter{
		gate:    make(chan struct{}),
		records: make(chan Record, 3),
	}
	_Reporters = []Reporter{reporter}
	defer func() {
		_Reporters = nil
	}()

	maxDim := Dimension{"k": "max_before"}
	minDim := Dimension{"k": "min_before"}
	swDim := Dimension{"k": "sw_before"}

	getMaxGauge("clone_max", "clone_group").UpdateWithDim(10, maxDim)
	getMinGauge("clone_min", "clone_group").UpdateWithDim(10, minDim)
	getStopWatch("clone_sw", "clone_group").RecordWithDim(swDim, time.Now().Add(-time.Millisecond))

	maxDim["k"] = "max_after"
	minDim["k"] = "min_after"
	swDim["k"] = "sw_after"

	close(reporter.gate)

	got := map[string]string{}
	for i := 0; i < 3; i++ {
		rc := <-reporter.records
		got[rc.Metrics().Name()] = rc.Dimensions()["k"]
	}

	if got["clone_max"] != "max_before" {
		t.Fatalf("max gauge dimensions should be cloned, got %q", got["clone_max"])
	}
	if got["clone_min"] != "min_before" {
		t.Fatalf("min gauge dimensions should be cloned, got %q", got["clone_min"])
	}
	if got["clone_sw"] != "sw_before" {
		t.Fatalf("stopwatch dimensions should be cloned, got %q", got["clone_sw"])
	}
}
