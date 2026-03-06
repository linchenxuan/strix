package metrics

import (
	"sync"
	"time"
)

// Metrics 是所有指标类型的基础接口。
// 所有指标都必须实现 Name()、Group() 和 Policy()。
type Metrics interface {
	// Name 返回指标名称。
	Name() string
	// Group 返回指标分组。
	Group() string
	// Policy 返回该指标的聚合策略。
	Policy() Policy
}

var (
	// _counters 存储所有计数器实例，并通过读写锁保证并发安全。
	_counters     = map[string]Counter{}
	_lockCounters = sync.RWMutex{}
	// _gauges 存储所有 gauge 实例，并通过读写锁保证并发安全。
	_gauges     = map[string]Gauge{}
	_lockGauges = sync.RWMutex{}
	// _stopwatchs 存储所有秒表实例，并通过读写锁保证并发安全。
	_stopwatchs    = map[string]StopWatch{}
	_lockstopwatch = sync.RWMutex{}
	// _avggauges 存储所有平均值 gauge 实例，并通过读写锁保证并发安全。
	_avggauges     = map[string]Gauge{}
	_lockavggauges = sync.RWMutex{}
	// _maxGauges 存储所有最大值 gauge 实例，并通过读写锁保证并发安全。
	_maxGauges     = map[string]Gauge{}
	_lockMaxGauges = sync.RWMutex{}
	// _minGauges 存储所有最小值 gauge 实例，并通过读写锁保证并发安全。
	_minGauges     = map[string]Gauge{}
	_lockMinGauges = sync.RWMutex{}
)

func buildMetricKey(name string, group string) string {
	return group + "\x00" + name
}

// IncrCounterWithGroup 按指定分组与值递增计数器指标。
// 计数器用于追踪随时间累积增长的值。
func IncrCounterWithGroup(key string, group string, value Value) {
	if c := getCounter(key, group); c != nil {
		c.Incr(value)
	}
}

// IncrCounterWithDimGroup 在指定分组和维度下递增计数器指标。
// 计数器用于追踪随时间累积增长的值。
func IncrCounterWithDimGroup(key string, group string, value Value, dimensions Dimension) {
	if c := getCounter(key, group); c != nil {
		c.IncrWithDim(value, dimensions)
	}
}

// UpdateGaugeWithGroup 在指定分组下更新 gauge 指标值。
// gauge 用于追踪可升可降的瞬时值。
func UpdateGaugeWithGroup(key string, group string, value Value) {
	if g := getGauge(key, group); g != nil {
		g.Update(value)
	}
}

// UpdateGaugeWithDimGroup 在指定分组和维度下更新 gauge 指标值。
// gauge 用于追踪可升可降的瞬时值。
func UpdateGaugeWithDimGroup(key string, group string, value Value, dimensions Dimension) {
	if g := getGauge(key, group); g != nil {
		g.UpdateWithDim(value, dimensions)
	}
}

// UpdateAvgGaugeWithGroup 在指定分组下更新平均值 gauge。
// 平均值 gauge 用于追踪一段时间内观测值的均值。
func UpdateAvgGaugeWithGroup(key string, group string, value Value) {
	if g := getAvgGauge(key, group); g != nil {
		g.Update(value)
	}
}

// UpdateAvgGaugeWithDimGroup 在指定分组和维度下更新平均值 gauge。
// 平均值 gauge 用于追踪一段时间内观测值的均值。
func UpdateAvgGaugeWithDimGroup(key string, group string, value Value, dimensions Dimension) {
	if g := getAvgGauge(key, group); g != nil {
		g.UpdateWithDim(value, dimensions)
	}
}

// UpdateMaxGaugeWithGroup 在指定分组下更新最大值 gauge。
// 最大值 gauge 用于追踪观测到的最高值。
func UpdateMaxGaugeWithGroup(key string, group string, value Value) {
	if g := getMaxGauge(key, group); g != nil {
		g.Update(value)
	}
}

// UpdateMaxGaugeWithDimGroup 在指定分组和维度下更新最大值 gauge。
// 最大值 gauge 用于追踪观测到的最高值。
func UpdateMaxGaugeWithDimGroup(key string, group string, value Value, dimensions Dimension) {
	if g := getMaxGauge(key, group); g != nil {
		g.UpdateWithDim(value, dimensions)
	}
}

// UpdateMinGaugeWithGroup 在指定分组下更新最小值 gauge。
// 最小值 gauge 用于追踪观测到的最低值。
func UpdateMinGaugeWithGroup(key string, group string, value Value) {
	if g := getMinGauge(key, group); g != nil {
		g.Update(value)
	}
}

// UpdateMinGaugeWithDimGroup 在指定分组和维度下更新最小值 gauge。
// 最小值 gauge 用于追踪观测到的最低值。
func UpdateMinGaugeWithDimGroup(key string, group string, value Value, dimensions Dimension) {
	if g := getMinGauge(key, group); g != nil {
		g.UpdateWithDim(value, dimensions)
	}
}

// RecordStopwatch 记录不带维度的秒表耗时。
// 秒表用于以毫秒为单位统计操作耗时。
func RecordStopwatch(key string, startTime time.Time) time.Duration {
	if s := getStopWatch(key, ""); s != nil {
		return s.RecordWithDim(nil, startTime)
	}
	return 0
}

// RecordStopwatchWithGroup 记录指定分组下的秒表耗时。
// 秒表用于以毫秒为单位统计操作耗时。
func RecordStopwatchWithGroup(key string, group string, startTime time.Time) time.Duration {
	if s := getStopWatch(key, group); s != nil {
		return s.RecordWithDim(nil, startTime)
	}
	return 0
}

// RecordStopwatchWithDimGroup 记录指定分组与维度下的秒表耗时。
// 秒表用于以毫秒为单位统计操作耗时。
func RecordStopwatchWithDimGroup(key string, group string, startTime time.Time, dimensions Dimension) time.Duration {
	if s := getStopWatch(key, group); s != nil {
		return s.RecordWithDim(dimensions, startTime)
	}
	return 0
}

// getGauge 获取或创建 gauge 指标，包含并发安全保护。
func getGauge(name string, group string) Gauge {
	key := buildMetricKey(name, group)
	_lockGauges.RLock()
	g, ok := _gauges[key]
	_lockGauges.RUnlock()
	if ok && g != nil {
		return g
	}

	_lockGauges.Lock()
	defer _lockGauges.Unlock()
	g, ok = _gauges[key]
	if ok && g != nil {
		return g
	}
	g = &gauge{
		name:  name,
		group: group,
	}
	_gauges[key] = g
	return g
}

// getAvgGauge 获取或创建平均值 gauge，包含并发安全保护。
func getAvgGauge(name string, group string) Gauge {
	key := buildMetricKey(name, group)
	_lockavggauges.RLock()
	g, ok := _avggauges[key]
	_lockavggauges.RUnlock()
	if ok && g != nil {
		return g
	}

	_lockavggauges.Lock()
	defer _lockavggauges.Unlock()
	g, ok = _avggauges[key]
	if ok && g != nil {
		return g
	}
	g = &avggauge{
		name:  name,
		group: group,
	}
	_avggauges[key] = g
	return g
}

// getMaxGauge 获取或创建最大值 gauge，包含并发安全保护。
func getMaxGauge(name string, group string) Gauge {
	key := buildMetricKey(name, group)
	_lockMaxGauges.RLock()
	g, ok := _maxGauges[key]
	_lockMaxGauges.RUnlock()
	if ok && g != nil {
		return g
	}

	_lockMaxGauges.Lock()
	defer _lockMaxGauges.Unlock()
	g, ok = _maxGauges[key]
	if ok && g != nil {
		return g
	}
	g = &maxGauge{
		name:  name,
		group: group,
	}
	_maxGauges[key] = g
	return g
}

// getMinGauge 获取或创建最小值 gauge，包含并发安全保护。
func getMinGauge(name string, group string) Gauge {
	key := buildMetricKey(name, group)
	_lockMinGauges.RLock()
	g, ok := _minGauges[key]
	_lockMinGauges.RUnlock()
	if ok && g != nil {
		return g
	}

	_lockMinGauges.Lock()
	defer _lockMinGauges.Unlock()
	g, ok = _minGauges[key]
	if ok && g != nil {
		return g
	}
	g = &minGauge{
		name:  name,
		group: group,
	}
	_minGauges[key] = g
	return g
}

// getCounter 获取或创建计数器指标，包含并发安全保护。
func getCounter(name string, group string) Counter {
	key := buildMetricKey(name, group)
	_lockCounters.RLock()
	c, ok := _counters[key]
	_lockCounters.RUnlock()
	if ok && c != nil {
		return c
	}

	_lockCounters.Lock()
	defer _lockCounters.Unlock()
	c, ok = _counters[key]
	if ok && c != nil {
		return c
	}
	c = &counter{
		name:  name,
		group: group,
	}
	_counters[key] = c
	return c
}

// getStopWatch 获取或创建秒表指标，包含并发安全保护。
func getStopWatch(name string, group string) StopWatch {
	key := buildMetricKey(name, group)
	_lockstopwatch.RLock()
	s, ok := _stopwatchs[key]
	_lockstopwatch.RUnlock()
	if ok && s != nil {
		return s
	}

	_lockstopwatch.Lock()
	defer _lockstopwatch.Unlock()
	s, ok = _stopwatchs[key]
	if ok && s != nil {
		return s
	}
	s = &stopwatch{
		name:  name,
		group: group,
	}
	_stopwatchs[key] = s
	return s
}
