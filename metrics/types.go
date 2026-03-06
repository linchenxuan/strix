// Package metrics 定义指标采集与上报使用的类型和常量。
package metrics

// Policy 定义指标值的聚合策略。
// 它决定同一指标在一个时间窗口内如何合并多次上报值。
type Policy int

const (
	Policy_None      Policy = iota // Policy_None 表示未指定聚合策略，系统可使用默认策略。
	Policy_Set                     // Policy_Set 表示瞬时值，最新一次上报覆盖历史值。
	Policy_Sum                     // Policy_Sum 表示累计值，所有上报值求和。
	Policy_Avg                     // Policy_Avg 表示平均值，按累计值和计数计算。
	Policy_Max                     // Policy_Max 表示最大值，取上报值中的最大值。
	Policy_Min                     // Policy_Min 表示最小值，取上报值中的最小值。
	Policy_Mid                     // Policy_Mid 表示中位数。
	Policy_Stopwatch               // Policy_Stopwatch 用于耗时类指标。
	Policy_Histogram               // Policy_Histogram 用于直方图分布统计。
)

// Value 表示指标值，底层类型为 float64。
type Value float64

// Dimension 表示指标维度键值对。
// 维度用于附加上下文信息，例如机器名、地域、版本等。
type Dimension map[string]string

// Clone 返回维度的深拷贝。
// 返回结果与原维度互不影响，可安全用于异步上报等场景。
func (d Dimension) Clone() Dimension {
	if d == nil {
		return nil
	}
	cp := make(Dimension, len(d))
	for k, v := range d {
		cp[k] = v
	}
	return cp
}

const (
	// KB 表示 1KB（1024 字节）。
	KB = 1024.0
	// MB 表示 1MB（1024 * 1024 字节）。
	MB = 1024.0 * 1024.0
)

// 指标分组常量，统一以 Group 前缀命名。
const (
	// GroupStrix 表示 strix 相关指标分组。
	GroupStrix = "strix"
)
