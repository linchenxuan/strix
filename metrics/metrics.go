package metrics

import (
	"sync"
	"time"
)

// Metrics is the base interface for all metric types.
// All metrics must implement Name(), Group(), and Policy() methods.
type Metrics interface {
	// Name returns the metric name
	Name() string
	// Group returns the metric group for categorization
	Group() string
	// Policy returns the aggregation policy for this metric
	Policy() Policy
}

var (
	// _counters stores all counter instances with thread-safe access
	_counters     = map[string]Counter{}
	_lockCounters = sync.RWMutex{}
	// _gauges stores all gauge instances with thread-safe access
	_gauges     = map[string]Gauge{}
	_lockGauges = sync.RWMutex{}
	// _stopwatchs stores all stopwatch instances with thread-safe access
	_stopwatchs    = map[string]StopWatch{}
	_lockstopwatch = sync.RWMutex{}
	// _avggauges stores all average gauge instances with thread-safe access
	_avggauges     = map[string]Gauge{}
	_lockavggauges = sync.RWMutex{}
	// _maxGauges stores all max gauge instances with thread-safe access
	_maxGauges     = map[string]Gauge{}
	_lockMaxGauges = sync.RWMutex{}
	// _minGauges stores all min gauge instances with thread-safe access
	_minGauges     = map[string]Gauge{}
	_lockMinGauges = sync.RWMutex{}
)

// SetMetricsReporters sets the global list of metric reporters.
// All metrics will be reported to these reporters when updated.
func SetMetricsReporters(reports []Reporter) {
	_Reporters = reports
}

// IncrCounterWithGroup increases a counter metric with specified group and value.
// Counters track cumulative values that only increase over time.
func IncrCounterWithGroup(key string, group string, value Value) {
	if c := getCounter(key, group); c != nil {
		c.Incr(value)
	}
}

// IncrCounterWithDimGroup increases a counter metric with specified group, value, and dimensions.
// Counters track cumulative values that only increase over time.
func IncrCounterWithDimGroup(key string, group string, value Value, dimensions Dimension) {
	if c := getCounter(key, group); c != nil {
		c.IncrWithDim(value, dimensions)
	}
}

// UpdateGaugeWithGroup updates a gauge metric with specified group and value.
// Gauges track point-in-time values that can go up or down.
func UpdateGaugeWithGroup(key string, group string, value Value) {
	if g := getGauge(key, group); g != nil {
		g.Update(value)
	}
}

// UpdateGaugeWithDimGroup updates a gauge metric with specified group, value, and dimensions.
// Gauges track point-in-time values that can go up or down.
func UpdateGaugeWithDimGroup(key string, group string, value Value, dimensions Dimension) {
	if g := getGauge(key, group); g != nil {
		g.UpdateWithDim(value, dimensions)
	}
}

// UpdateAvgGaugeWithGroup updates an average gauge with specified group and value.
// Average gauges track the mean value of observations over time.
func UpdateAvgGaugeWithGroup(key string, group string, value Value) {
	if g := getAvgGauge(key, group); g != nil {
		g.Update(value)
	}
}

// UpdateAvgGaugeWithDimGroup updates an average gauge with specified group, value, and dimensions.
// Average gauges track the mean value of observations over time.
func UpdateAvgGaugeWithDimGroup(key string, group string, value Value, dimensions Dimension) {
	if g := getAvgGauge(key, group); g != nil {
		g.UpdateWithDim(value, dimensions)
	}
}

// UpdateMaxGaugeWithGroup updates a max gauge with specified group and value.
// Max gauges track the highest value observed.
func UpdateMaxGaugeWithGroup(key string, group string, value Value) {
	if g := getMaxGauge(key, group); g != nil {
		g.Update(value)
	}
}

// UpdateMaxGaugeWithDimGroup updates a max gauge with specified group, value, and dimensions.
// Max gauges track the highest value observed.
func UpdateMaxGaugeWithDimGroup(key string, group string, value Value, dimensions Dimension) {
	if g := getMaxGauge(key, group); g != nil {
		g.UpdateWithDim(value, dimensions)
	}
}

// UpdateMinGaugeWithGroup updates a min gauge with specified group and value.
// Min gauges track the lowest value observed.
func UpdateMinGaugeWithGroup(key string, group string, value Value) {
	if g := getMinGauge(key, group); g != nil {
		g.Update(value)
	}
}

// UpdateMinGaugeWithDimGroup updates a min gauge with specified group, value, and dimensions.
// Min gauges track the lowest value observed.
func UpdateMinGaugeWithDimGroup(key string, group string, value Value, dimensions Dimension) {
	if g := getMinGauge(key, group); g != nil {
		g.UpdateWithDim(value, dimensions)
	}
}

// RecordStopwatch records a stopwatch duration without dimensions.
// Stopwatches measure the time taken for operations in milliseconds.
func RecordStopwatch(key string, startTime time.Time) time.Duration {
	if s := getStopWatch(key, ""); s != nil {
		return s.RecordWithDim(nil, startTime)
	}
	return 0
}

// RecordStopwatchWithGroup records a stopwatch duration with specified group.
// Stopwatches measure the time taken for operations in milliseconds.
func RecordStopwatchWithGroup(key string, group string, startTime time.Time) time.Duration {
	if s := getStopWatch(key, group); s != nil {
		return s.RecordWithDim(nil, startTime)
	}
	return 0
}

// RecordStopwatchWithDimGroup records a stopwatch duration with specified group and dimensions.
// Stopwatches measure the time taken for operations in milliseconds.
func RecordStopwatchWithDimGroup(key string, group string, startTime time.Time, dimensions Dimension) time.Duration {
	if s := getStopWatch(key, group); s != nil {
		return s.RecordWithDim(dimensions, startTime)
	}
	return 0
}

// getGauge gets or creates a gauge metric with thread-safe access.
func getGauge(name string, group string) Gauge {
	_lockGauges.RLock()
	g, ok := _gauges[name]
	_lockGauges.RUnlock()
	if ok && g != nil {
		return g
	}

	_lockGauges.Lock()
	defer _lockGauges.Unlock()
	g, ok = _gauges[name]
	if ok && g != nil {
		return g
	}
	g = &gauge{
		name:  name,
		group: group,
	}
	_gauges[name] = g
	return g
}

// getAvgGauge gets or creates an average gauge with thread-safe access.
func getAvgGauge(name string, group string) Gauge {
	_lockavggauges.RLock()
	g, ok := _avggauges[name]
	_lockavggauges.RUnlock()
	if ok && g != nil {
		return g
	}

	_lockavggauges.Lock()
	defer _lockavggauges.Unlock()
	g, ok = _avggauges[name]
	if ok && g != nil {
		return g
	}
	g = &avggauge{
		name:  name,
		group: group,
	}
	_avggauges[name] = g
	return g
}

// getMaxGauge gets or creates a max gauge with thread-safe access.
func getMaxGauge(name string, group string) Gauge {
	_lockMaxGauges.RLock()
	g, ok := _maxGauges[name]
	_lockMaxGauges.RUnlock()
	if ok && g != nil {
		return g
	}

	_lockMaxGauges.Lock()
	defer _lockMaxGauges.Unlock()
	g, ok = _maxGauges[name]
	if ok && g != nil {
		return g
	}
	g = &maxGauge{
		name:  name,
		group: group,
	}
	_maxGauges[name] = g
	return g
}

// getMinGauge gets or creates a min gauge with thread-safe access.
func getMinGauge(name string, group string) Gauge {
	_lockMinGauges.RLock()
	g, ok := _minGauges[name]
	_lockMinGauges.RUnlock()
	if ok && g != nil {
		return g
	}

	_lockMinGauges.Lock()
	defer _lockMinGauges.Unlock()
	g, ok = _minGauges[name]
	if ok && g != nil {
		return g
	}
	g = &minGauge{
		name:  name,
		group: group,
	}
	_minGauges[name] = g
	return g
}

// getCounter gets or creates a counter metric with thread-safe access.
func getCounter(name string, group string) Counter {
	_lockCounters.RLock()
	c, ok := _counters[name]
	_lockCounters.RUnlock()
	if ok && c != nil {
		return c
	}

	_lockCounters.Lock()
	defer _lockCounters.Unlock()
	c, ok = _counters[name]
	if ok && c != nil {
		return c
	}
	c = &counter{
		name:  name,
		group: group,
	}
	_counters[name] = c
	return c
}

// getStopWatch gets or creates a stopwatch metric with thread-safe access.
func getStopWatch(name string, group string) StopWatch {
	_lockstopwatch.RLock()
	s, ok := _stopwatchs[name]
	_lockstopwatch.RUnlock()
	if ok && s != nil {
		return s
	}

	_lockstopwatch.Lock()
	defer _lockstopwatch.Unlock()
	s, ok = _stopwatchs[name]
	if ok && s != nil {
		return s
	}
	s = &stopwatch{
		name:  name,
		group: group,
	}
	_stopwatchs[name] = s
	return s
}
