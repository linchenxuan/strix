package metrics

import "fmt"

// Record 表示一条指标采样记录及其元数据。
// 其中包含指标定义、采样值、计数（用于平均值）以及维度标签。
type Record struct {
	metrics    Metrics   // 当前记录对应的指标定义
	value      Value     // 采样值
	cnt        int       // 采样计数（用于平均值计算）
	dimensions Dimension // 指标维度键值对
}

// Clone 深拷贝一条 Record，包括所有字段与维度信息。
func (r *Record) Clone() *Record {
	cp := &Record{
		metrics: r.metrics,
		value:   r.value,
		cnt:     r.cnt,
	}
	cp.dimensions = make(Dimension, len(r.dimensions))
	for k, v := range r.dimensions {
		cp.dimensions[k] = v
	}
	return cp
}

// SetMetrics 设置记录关联的指标定义。
func (r *Record) SetMetrics(m Metrics) {
	r.metrics = m
}

// SetValue 设置记录值。
func (r *Record) SetValue(v Value) {
	r.value = v
}

// SetDimension 设置记录维度（标签）。
func (r *Record) SetDimension(d Dimension) {
	r.dimensions = d.Clone()
}

// Metrics 返回该记录关联的指标定义。
func (r *Record) Metrics() Metrics {
	return r.metrics
}

// Value 按指标聚合策略返回处理后的值。
// 对于 Policy_Avg 与 Policy_Stopwatch，返回 value/count；
// 其他策略直接返回原始 value。
func (r *Record) Value() Value {
	switch r.metrics.Policy() {
	case Policy_Avg, Policy_Stopwatch:
		if r.cnt != 0 {
			return r.value / Value(r.cnt)
		}
		return r.value
	}
	return r.value
}

// RawData 返回未经处理的原始 value 和 cnt。
// 该方法适用于需要底层累积数据的聚合逻辑。
func (r *Record) RawData() (Value, int) {
	return r.value, r.cnt
}

// Dimensions 返回该记录的维度键值对。
func (r *Record) Dimensions() map[string]string {
	return r.dimensions
}

// Merge 按指标聚合策略将另一条记录合并到当前记录。
// 两条记录必须拥有相同的指标名、分组、策略和维度。
// 实际合并方式由策略决定（sum/max/min/avg 等）。
func (r *Record) Merge(other Record) error {
	// 校验两条记录是否属于同一指标。
	if r.metrics.Name() != other.metrics.Name() {
		return fmt.Errorf("metrics name(%s,%s) not equal", r.metrics.Name(), other.metrics.Name())
	}
	if r.metrics.Group() != other.metrics.Group() {
		return fmt.Errorf("metrics group(%s,%s) not equal", r.metrics.Group(), other.metrics.Group())
	}
	if r.metrics.Policy() != other.metrics.Policy() {
		return fmt.Errorf("metrics policy(%v,%v) not equal", r.metrics.Policy(), other.metrics.Policy())
	}

	if len(r.dimensions) != len(other.dimensions) {
		return fmt.Errorf("metrics dimensions(%d,%d) not equal", len(r.dimensions), len(other.dimensions))
	}

	for k, v := range r.dimensions {
		v2, exist := other.dimensions[k]
		if !exist {
			return fmt.Errorf("metrics dimensions(%s) not exist", k)
		}
		if v != v2 {
			return fmt.Errorf("metrics dimensions(%s,%s) not equal", v, v2)
		}
	}

	// 按策略执行合并。
	switch r.metrics.Policy() {
	case Policy_Set:
		r.value = other.value
	case Policy_Sum:
		r.value += other.value
	case Policy_Max:
		if other.value > r.value {
			r.value = other.value
		}
	case Policy_Min:
		if other.value < r.value {
			r.value = other.value
		}
	case Policy_Stopwatch, Policy_Avg:
		r.value += other.value
		r.cnt += other.cnt
	default:
		return fmt.Errorf("metrics policy(%v) not supported", r.metrics.Policy())
	}
	return nil
}
