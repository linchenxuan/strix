package metrics

// avggauge implements a gauge that calculates the average of multiple values.
// It accumulates values and counts, then reports the average when Value() is called.
type avggauge struct {
	name  string // Metric name
	group string // Metric group for categorization
}

// Name returns the metric name.
func (g *avggauge) Name() string {
	return g.name
}

// Group returns the metric group.
func (g *avggauge) Group() string {
	return g.group
}

// Policy returns the aggregation policy for this gauge (Policy_Avg).
func (g *avggauge) Policy() Policy {
	return Policy_Avg
}

// Update updates the average gauge value without dimensions.
func (g *avggauge) Update(v Value) {
	g.UpdateWithDim(v, nil)
}

// UpdateWithDim updates the average gauge value with specified dimensions.
// Each update contributes to the average calculation with equal weight.
func (g *avggauge) UpdateWithDim(v Value, dimensions Dimension) {
	r := Record{
		metrics:    g,
		value:      v,
		cnt:        1,
		dimensions: dimensions,
	}
	for _, reporter := range _Reporters {
		reporter.Report(r)
	}
}
