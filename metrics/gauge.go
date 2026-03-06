package metrics

// Gauge 定义瞬时值指标接口。
// Gauge 常用于可升可降的观测值，例如温度、内存使用量、在线连接数等。
type Gauge interface {
	Metrics
	// Update 设置 gauge 的绝对值。
	Update(value Value)
	// UpdateWithDim 在指定维度下设置 gauge 的绝对值。
	UpdateWithDim(value Value, dimensions Dimension)
}

// gauge 是 Gauge 接口的实现，聚合策略为 Policy_Set。
// 它表示可被重复覆盖的瞬时观测值。
type gauge struct {
	name  string // 指标名称
	group string // 指标分组
}

// Name 返回指标名称。
func (g *gauge) Name() string {
	return g.name
}

// Group 返回指标分组。
func (g *gauge) Group() string {
	return g.group
}

// Policy 返回该 gauge 的聚合策略（Policy_Set）。
// Policy_Set 表示以最新一次上报值覆盖历史值。
func (g *gauge) Policy() Policy {
	return Policy_Set
}

// Update 在不带维度时更新 gauge 值。
func (g *gauge) Update(v Value) {
	g.UpdateWithDim(v, nil)
}

// UpdateWithDim 在指定维度下更新 gauge 值。
// 指标会被投递给所有已注册的 reporter。
func (g *gauge) UpdateWithDim(v Value, dimensions Dimension) {
	r := Record{
		metrics:    g,
		value:      v,
		dimensions: dimensions.Clone(),
	}
	for _, reporter := range _Reporters {
		reporter.Report(r)
	}
}
