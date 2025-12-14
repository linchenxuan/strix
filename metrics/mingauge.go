package metrics

// minGauge implements a gauge that tracks the minimum value observed.
// It always reports the lowest value that has been recorded.
type minGauge struct {
	name  string // Metric name
	group string // Metric group for categorization
}

// Name returns the metric name.
func (g *minGauge) Name() string {
	return g.name
}

// Group returns the metric group.
func (g *minGauge) Group() string {
	return g.group
}

// Policy returns the aggregation policy for this gauge (Policy_Min).
func (g *minGauge) Policy() Policy {
	return Policy_Min
}

// Update updates the minimum gauge value without dimensions.
func (g *minGauge) Update(v Value) {
	g.UpdateWithDim(v, nil)
}

// UpdateWithDim updates the minimum gauge value with specified dimensions.
// The gauge will keep track of the lowest value observed.
func (g *minGauge) UpdateWithDim(v Value, dimensions Dimension) {
	r := Record{
		metrics:    g,
		value:      v,
		dimensions: dimensions,
	}
	for _, reporter := range _Reporters {
		reporter.Report(r)
	}
}
