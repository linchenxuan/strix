package metrics

// Gauge interface for metrics that represent a point-in-time value.
// Gauges are typically used for measurements that can go up or down,
// such as temperature, memory usage, or number of active connections.
type Gauge interface {
	Metrics
	// Update sets the gauge's absolute value.
	Update(value Value)
	// UpdateWithDim sets the gauge's absolute value with specified dimensions.
	UpdateWithDim(value Value, dimensions Dimension)
}

// gauge implements the Gauge interface with a set aggregation policy.
// It represents a point-in-time measurement that can be updated.
type gauge struct {
	name  string // Metric name
	group string // Metric group for categorization
}

// Name returns the metric name.
func (g *gauge) Name() string {
	return g.name
}

// Group returns the metric group.
func (g *gauge) Group() string {
	return g.group
}

// Policy returns the aggregation policy for this gauge (Policy_Set).
// Gauges use Policy_Set which means the last value overwrites previous values.
func (g *gauge) Policy() Policy {
	return Policy_Set
}

// Update updates the gauge value without dimensions.
func (g *gauge) Update(v Value) {
	g.UpdateWithDim(v, nil)
}

// UpdateWithDim updates the gauge value with specified dimensions.
// The metric is reported to all registered reporters.
func (g *gauge) UpdateWithDim(v Value, dimensions Dimension) {
	r := Record{
		metrics:    g,
		value:      v,
		dimensions: dimensions,
	}
	for _, reporter := range _Reporters {
		reporter.Report(r)
	}
}
