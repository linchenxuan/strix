package metrics

// Counter interface for counter metrics that accumulate values over time.
// Counters are typically used to track cumulative metrics like request counts,
// error counts, or total bytes processed.
type Counter interface {
	Metrics
	// IncrWithDim increments the counter by delta with specified dimensions.
	IncrWithDim(delta Value, dimensions Dimension)
	// Incr increments the counter by delta without dimensions.
	Incr(delta Value)
}

// counter implements the Counter interface with a sum aggregation policy.
type counter struct {
	name  string // Metric name
	group string // Metric group for categorization
}

// Name returns the metric name.
func (c *counter) Name() string {
	return c.name
}

// Group returns the metric group.
func (c *counter) Group() string {
	return c.group
}

// Policy returns the aggregation policy for this counter (Policy_Sum).
func (c *counter) Policy() Policy {
	return Policy_Sum
}

// Incr increments the counter value by v without dimensions.
func (c *counter) Incr(v Value) {
	c.IncrWithDim(v, nil)
}

// IncrWithDim increments the counter value by v with specified dimensions.
// The metric is reported to all registered reporters.
func (c *counter) IncrWithDim(v Value, dimensions Dimension) {
	r := Record{
		metrics:    c,
		value:      v,
		dimensions: dimensions,
	}
	for _, reporter := range _Reporters {
		reporter.Report(r)
	}
}
