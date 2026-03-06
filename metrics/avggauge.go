package metrics

// avggauge 是平均值 gauge 的实现。
// 它累计上报值与计数，最终按 value/count 计算平均值。
type avggauge struct {
	name  string // 指标名称
	group string // 指标分组
}

// Name 返回指标名称。
func (g *avggauge) Name() string {
	return g.name
}

// Group 返回指标分组。
func (g *avggauge) Group() string {
	return g.group
}

// Policy 返回该 gauge 的聚合策略（Policy_Avg）。
func (g *avggauge) Policy() Policy {
	return Policy_Avg
}

// Update 在不带维度时更新平均值 gauge。
func (g *avggauge) Update(v Value) {
	g.UpdateWithDim(v, nil)
}

// UpdateWithDim 在指定维度下更新平均值 gauge。
// 每次上报都按等权重计入平均值计算。
func (g *avggauge) UpdateWithDim(v Value, dimensions Dimension) {
	r := Record{
		metrics:    g,
		value:      v,
		cnt:        1,
		dimensions: dimensions.Clone(),
	}
	for _, reporter := range _Reporters {
		reporter.Report(r)
	}
}
