package metrics

// maxGauge implements a gauge that tracks the maximum value observed.
// It always reports the highest value that has been recorded.
type maxGauge struct {
	name  string // Metric name
	group string // Metric group for categorization
}

// Name returns the metric name.
func (g *maxGauge) Name() string {
	return g.name
}

// Group returns the metric group.
func (g *maxGauge) Group() string {
	return g.group
}

// Policy returns the aggregation policy for this gauge (Policy_Max).
func (g *maxGauge) Policy() Policy {
	return Policy_Max
}

// Update updates the maximum gauge value without dimensions.
func (g *maxGauge) Update(v Value) {
	g.UpdateWithDim(v, nil)
}

// UpdateWithDim updates the maximum gauge value with specified dimensions.
// The gauge will keep track of the highest value observed.
func (g *maxGauge) UpdateWithDim(v Value, dimensions Dimension) {
	r := Record{
		metrics:    g,
		value:      v,
		dimensions: dimensions,
	}
	for _, reporter := range _Reporters {
		reporter.Report(r)
	}
}
