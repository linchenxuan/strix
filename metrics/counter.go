package metrics

// Counter 定义累加型指标接口。
// 计数器通常用于统计请求数、错误数、处理字节数等只增不减的累计值。
type Counter interface {
	Metrics
	// IncrWithDim 在带维度的情况下按 delta 增加计数值。
	IncrWithDim(delta Value, dimensions Dimension)
	// Incr 在不带维度的情况下按 delta 增加计数值。
	Incr(delta Value)
}

// counter 是 Counter 接口的实现，聚合策略为 Policy_Sum。
type counter struct {
	name  string // 指标名称
	group string // 指标分组
}

// Name 返回指标名称。
func (c *counter) Name() string {
	return c.name
}

// Group 返回指标分组。
func (c *counter) Group() string {
	return c.group
}

// Policy 返回该计数器的聚合策略（Policy_Sum）。
func (c *counter) Policy() Policy {
	return Policy_Sum
}

// Incr 在不带维度时按 v 增加计数值。
func (c *counter) Incr(v Value) {
	c.IncrWithDim(v, nil)
}

// IncrWithDim 在带维度时按 v 增加计数值。
// 指标会被投递给所有已注册的 reporter。
func (c *counter) IncrWithDim(v Value, dimensions Dimension) {
	r := Record{
		metrics:    c,
		value:      v,
		dimensions: dimensions.Clone(),
	}
	for _, reporter := range _Reporters {
		reporter.Report(r)
	}
}
