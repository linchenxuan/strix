package metrics

import "fmt"

// Record represents a single metric measurement with its metadata.
// It contains the metric definition, measured value, count (for averaging),
// and associated dimensions for labeling.
type Record struct {
	metrics    Metrics   // The metric being recorded
	value      Value     // The measured value
	cnt        int       // Count of values (used for averaging calculations)
	dimensions Dimension // Key-value pairs for metric labeling
}

// Clone creates a deep copy of the Record with all its fields and dimensions.
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

// SetMetrics sets the metric definition for this record.
func (r *Record) SetMetrics(m Metrics) {
	r.metrics = m
}

// SetValue sets the measured value for this record.
func (r *Record) SetValue(v Value) {
	r.value = v
}

// SetDimension sets the dimensions (labels) for this record.
func (r *Record) SetDimension(d Dimension) {
	r.dimensions = d
}

// Metrics returns the metric definition associated with this record.
func (r *Record) Metrics() Metrics {
	return r.metrics
}

// Value returns the processed value based on the metric's aggregation policy.
// For Policy_Avg and Policy_Stopwatch, it returns the average value (value/count).
// For other policies, it returns the raw value.
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

// RawData returns the raw value and count without any processing.
// This is useful for aggregation operations that need the underlying data.
func (r *Record) RawData() (Value, int) {
	return r.value, r.cnt
}

// Dimensions returns the key-value pairs used for metric labeling.
func (r *Record) Dimensions() map[string]string {
	return r.dimensions
}

// Merge combines another Record into this one based on the metric's aggregation policy.
// Both records must have the same metric name, group, policy, and dimensions.
// The merge operation depends on the metric policy (sum, max, min, avg, etc.).
func (r *Record) Merge(other Record) error {
	// Validate that both records are for the same metric
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

	// Perform the merge based on the metric policy
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
		return fmt.Errorf("")
	}
	return nil
}
