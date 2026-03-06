package metrics

import (
	"time"
)

// StopWatch 定义耗时统计指标接口。
// 该指标用于记录操作耗时，例如请求处理时间、数据库查询耗时等。
type StopWatch interface {
	Metrics
	// RecordWithDim 记录从 startTime 到当前时刻的耗时，并携带指定维度。
	RecordWithDim(dimensions Dimension, startTime time.Time) time.Duration
}

// stopwatch 是 StopWatch 接口的实现。
type stopwatch struct {
	name  string // 指标名称
	group string // 指标分组
}

// Name 返回秒表指标名称。
func (s *stopwatch) Name() string {
	return s.name
}

// Group 返回秒表指标分组。
func (s *stopwatch) Group() string {
	return s.group
}

// Policy 返回该秒表的聚合策略（Policy_Stopwatch）。
func (s *stopwatch) Policy() Policy {
	return Policy_Stopwatch
}

// RecordWithDim 记录从 startTime 到当前的耗时，并携带指定维度。
// 上报值单位为毫秒，最终会投递给所有已注册的 reporter。
// 返回值为本次实际测得的耗时。
func (s *stopwatch) RecordWithDim(dimensions Dimension, startTime time.Time) time.Duration {
	duration := time.Since(startTime)
	r := Record{
		metrics:    s,
		value:      Value(float64(duration.Microseconds()) / 1000),
		cnt:        1,
		dimensions: dimensions.Clone(),
	}

	for _, reporter := range _Reporters {
		reporter.Report(r)
	}
	return duration
}
