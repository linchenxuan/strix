package metrics

// maxGauge 是最大值 gauge 的实现。
// 它始终保留并上报当前观测到的最大值。
type maxGauge struct {
	name  string // 指标名称
	group string // 指标分组
}

// Name 返回指标名称。
func (g *maxGauge) Name() string {
	return g.name
}

// Group 返回指标分组。
func (g *maxGauge) Group() string {
	return g.group
}

// Policy 返回该 gauge 的聚合策略（Policy_Max）。
func (g *maxGauge) Policy() Policy {
	return Policy_Max
}

// Update 在不带维度时更新最大值 gauge。
func (g *maxGauge) Update(v Value) {
	g.UpdateWithDim(v, nil)
}

// UpdateWithDim 在指定维度下更新最大值 gauge。
// 该 gauge 会持续保留当前观测到的最高值。
func (g *maxGauge) UpdateWithDim(v Value, dimensions Dimension) {
	r := Record{
		metrics:    g,
		value:      v,
		dimensions: dimensions.Clone(),
	}
	for _, reporter := range _Reporters {
		reporter.Report(r)
	}
}
