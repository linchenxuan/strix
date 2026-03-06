package metrics

// minGauge 是最小值 gauge 的实现。
// 它始终保留并上报当前观测到的最小值。
type minGauge struct {
	name  string // 指标名称
	group string // 指标分组
}

// Name 返回指标名称。
func (g *minGauge) Name() string {
	return g.name
}

// Group 返回指标分组。
func (g *minGauge) Group() string {
	return g.group
}

// Policy 返回该 gauge 的聚合策略（Policy_Min）。
func (g *minGauge) Policy() Policy {
	return Policy_Min
}

// Update 在不带维度时更新最小值 gauge。
func (g *minGauge) Update(v Value) {
	g.UpdateWithDim(v, nil)
}

// UpdateWithDim 在指定维度下更新最小值 gauge。
// 该 gauge 会持续保留当前观测到的最低值。
func (g *minGauge) UpdateWithDim(v Value, dimensions Dimension) {
	r := Record{
		metrics:    g,
		value:      v,
		dimensions: dimensions.Clone(),
	}
	for _, reporter := range _Reporters {
		reporter.Report(r)
	}
}
