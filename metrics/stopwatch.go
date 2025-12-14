package metrics

import (
	"time"
)

// StopWatch interface for timing metrics that measure duration.
// Stopwatches are used to track how long operations take, such as
// request processing time, database query duration, etc.
type StopWatch interface {
	Metrics
	// RecordWithDim records the duration since startTime with specified dimensions.
	RecordWithDim(dimensions Dimension, startTime time.Time) time.Duration
}

// stopwatch implements the StopWatch interface for measuring durations.
type stopwatch struct {
	name  string // Metric name
	group string // Metric group for categorization
}

// Name returns the stopwatch name.
func (s *stopwatch) Name() string {
	return s.name
}

// Group returns the stopwatch group.
func (s *stopwatch) Group() string {
	return s.group
}

// Policy returns the aggregation policy for this stopwatch (Policy_Stopwatch).
func (s *stopwatch) Policy() Policy {
	return Policy_Stopwatch
}

// RecordWithDim records the duration since startTime with specified dimensions.
// The duration is measured in milliseconds and reported to all registered reporters.
// Returns the actual duration that was recorded.
func (s *stopwatch) RecordWithDim(dimensions Dimension, startTime time.Time) time.Duration {
	duration := time.Since(startTime)
	r := Record{
		metrics:    s,
		value:      Value(float64(duration.Microseconds()) / 1000),
		cnt:        1,
		dimensions: dimensions,
	}

	for _, reporter := range _Reporters {
		reporter.Report(r)
	}
	return duration
}
