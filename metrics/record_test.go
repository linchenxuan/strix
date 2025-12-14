package metrics

import (
	"testing"
)

// 测试Record结构体的基本功能
func TestRecord(t *testing.T) {
	// 创建一个测试用的Metrics
	counter := getCounter("test_counter", "test_group")
	dimensions := Dimension{"key1": "value1", "key2": "value2"}

	// 创建记录
	record := Record{
		metrics:    counter,
		value:      42.5,
		cnt:        3,
		dimensions: dimensions,
	}

	// 测试Clone方法
	t.Run("TestRecordClone", func(t *testing.T) {
		clone := record.Clone()

		// 验证克隆后的记录与原记录内容相同
		if clone.metrics != record.metrics {
			t.Error("Expected cloned record to reference the same metrics")
		}
		if clone.value != record.value {
			t.Errorf("Expected cloned value %v, got %v", record.value, clone.value)
		}
		if clone.cnt != record.cnt {
			t.Errorf("Expected cloned count %d, got %d", record.cnt, clone.cnt)
		}
		if len(clone.dimensions) != len(record.dimensions) {
			t.Errorf("Expected cloned dimensions length %d, got %d", len(record.dimensions), len(clone.dimensions))
		}
		// 验证维度是深拷贝
		for k, v := range record.dimensions {
			if clone.dimensions[k] != v {
				t.Errorf("Expected cloned dimension %s = %s, got %s", k, v, clone.dimensions[k])
			}
		}
		// 修改克隆的维度不应该影响原记录
		clone.dimensions["key1"] = "modified"
		if record.dimensions["key1"] != "value1" {
			t.Error("Modifying cloned dimensions should not affect original record")
		}
	})

	// 测试Getter方法
	t.Run("TestRecordGetters", func(t *testing.T) {
		if record.Metrics() != counter {
			t.Error("Expected Metrics() to return the same counter")
		}
		if record.Value() != 42.5 {
			t.Errorf("Expected Value() to return 42.5, got %v", record.Value())
		}
		value, cnt := record.RawData()
		if value != 42.5 {
			t.Errorf("Expected RawData() to return value 42.5, got %v", value)
		}
		if cnt != 3 {
			t.Errorf("Expected RawData() to return count 3, got %d", cnt)
		}
		dim := record.Dimensions()
		if len(dim) != 2 {
			t.Errorf("Expected Dimensions() to return 2 dimensions, got %d", len(dim))
		}
		if dim["key1"] != "value1" || dim["key2"] != "value2" {
			t.Error("Expected Dimensions() to return correct values")
		}
	})

	// 测试Setter方法
	t.Run("TestRecordSetters", func(t *testing.T) {
		newCounter := getCounter("new_counter", "new_group")
		newDimensions := Dimension{"new_key": "new_value"}

		record.SetMetrics(newCounter)
		record.SetValue(100.0)
		record.SetDimension(newDimensions)

		if record.Metrics() != newCounter {
			t.Error("Expected SetMetrics() to update metrics")
		}
		if record.Value() != 100.0 {
			t.Errorf("Expected SetValue() to update value to 100.0, got %v", record.Value())
		}
		dim := record.Dimensions()
		if len(dim) != 1 {
			t.Errorf("Expected SetDimension() to update dimensions length to 1, got %d", len(dim))
		}
		if dim["new_key"] != "new_value" {
			t.Error("Expected SetDimension() to update dimensions values")
		}
	})

	// 测试不同策略下的Value方法行为
	t.Run("TestRecordValueWithDifferentPolicies", func(t *testing.T) {
		// 测试PolicyAvg
		avgGauge := getAvgGauge("test_avg", "test_group")
		avgRecord := Record{
			metrics: avgGauge,
			value:   90,
			cnt:     3,
		}
		if avgRecord.Value() != 30 { // 90 / 3 = 30
			t.Errorf("Expected Value() with Policy_Avg to return 30, got %v", avgRecord.Value())
		}

		// 测试PolicySum (应该直接返回value)
		sumRecord := Record{
			metrics: counter, // counter has Policy_Sum
			value:   50,
			cnt:     2,
		}
		if sumRecord.Value() != 50 {
			t.Errorf("Expected Value() with Policy_Sum to return 50, got %v", sumRecord.Value())
		}

		// 测试PolicyStopwatch
		stopwatch := getStopWatch("test_stopwatch", "test_group")
		stopwatchRecord := Record{
			metrics: stopwatch,
			value:   1000,
			cnt:     4,
		}
		if stopwatchRecord.Value() != 250 { // 1000 / 4 = 250
			t.Errorf("Expected Value() with Policy_Stopwatch to return 250, got %v", stopwatchRecord.Value())
		}

		// 测试cnt为0的情况
		zeroRecord := Record{
			metrics: avgGauge,
			value:   75,
			cnt:     0,
		}
		if zeroRecord.Value() != 75 {
			t.Errorf("Expected Value() with cnt 0 to return raw value 75, got %v", zeroRecord.Value())
		}
	})
}

// 测试Record的Merge方法
func TestRecordMerge(t *testing.T) {
	tests := []struct {
		name        string
		policy      Policy
		original    Record
		other       Record
		expected    Value
		expectError bool
	}{}

	// 创建不同策略的Metrics
	counter := getCounter("test_counter", "test_group") // Policy_Sum
	gauge := getGauge("test_gauge", "test_group")       // Policy_Set
	maxGauge := getMaxGauge("test_max", "test_group")   // Policy_Max
	minGauge := getMinGauge("test_min", "test_group")   // Policy_Min
	avgGauge := getAvgGauge("test_avg", "test_group")   // Policy_Avg

	dimensions := Dimension{"host": "server1"}

	// 添加测试用例
	tests = append(tests, struct {
		name        string
		policy      Policy
		original    Record
		other       Record
		expected    Value
		expectError bool
	}{"Policy_Sum Merge", Policy_Sum, Record{
		metrics:    counter,
		value:      10,
		cnt:        1,
		dimensions: dimensions,
	}, Record{
		metrics:    counter,
		value:      20,
		cnt:        1,
		dimensions: dimensions,
	}, 30, false})

	tests = append(tests, struct {
		name        string
		policy      Policy
		original    Record
		other       Record
		expected    Value
		expectError bool
	}{"Policy_Set Merge", Policy_Set, Record{
		metrics:    gauge,
		value:      10,
		cnt:        1,
		dimensions: dimensions,
	}, Record{
		metrics:    gauge,
		value:      20,
		cnt:        1,
		dimensions: dimensions,
	}, 20, false})

	tests = append(tests, struct {
		name        string
		policy      Policy
		original    Record
		other       Record
		expected    Value
		expectError bool
	}{"Policy_Max Merge", Policy_Max, Record{
		metrics:    maxGauge,
		value:      15,
		cnt:        1,
		dimensions: dimensions,
	}, Record{
		metrics:    maxGauge,
		value:      25,
		cnt:        1,
		dimensions: dimensions,
	}, 25, false})

	tests = append(tests, struct {
		name        string
		policy      Policy
		original    Record
		other       Record
		expected    Value
		expectError bool
	}{"Policy_Min Merge", Policy_Min, Record{
		metrics:    minGauge,
		value:      30,
		cnt:        1,
		dimensions: dimensions,
	}, Record{
		metrics:    minGauge,
		value:      20,
		cnt:        1,
		dimensions: dimensions,
	}, 20, false})

	tests = append(tests, struct {
		name        string
		policy      Policy
		original    Record
		other       Record
		expected    Value
		expectError bool
	}{"Policy_Avg Merge", Policy_Avg, Record{
		metrics:    avgGauge,
		value:      40,
		cnt:        2,
		dimensions: dimensions,
	}, Record{
		metrics:    avgGauge,
		value:      60,
		cnt:        3,
		dimensions: dimensions,
	}, 20, false}) // (40+60)/(2+3) = 20

	tests = append(tests, struct {
		name        string
		policy      Policy
		original    Record
		other       Record
		expected    Value
		expectError bool
	}{"Different Names", Policy_Sum, Record{
		metrics:    getCounter("counter1", "test_group"),
		value:      10,
		cnt:        1,
		dimensions: dimensions,
	}, Record{
		metrics:    getCounter("counter2", "test_group"),
		value:      20,
		cnt:        1,
		dimensions: dimensions,
	}, 0, true})

	tests = append(tests, struct {
		name        string
		policy      Policy
		original    Record
		other       Record
		expected    Value
		expectError bool
	}{"Different Groups", Policy_Sum, Record{
		metrics:    getCounter("test_counter1", "group1"),
		value:      10,
		cnt:        1,
		dimensions: dimensions,
	}, Record{
		metrics:    getCounter("test_counter2", "group2"),
		value:      20,
		cnt:        1,
		dimensions: dimensions,
	}, 0, true})

	tests = append(tests, struct {
		name        string
		policy      Policy
		original    Record
		other       Record
		expected    Value
		expectError bool
	}{"Different Policies", Policy_Sum, Record{
		metrics:    counter,
		value:      10,
		cnt:        1,
		dimensions: dimensions,
	}, Record{
		metrics:    gauge,
		value:      20,
		cnt:        1,
		dimensions: dimensions,
	}, 0, true})

	tests = append(tests, struct {
		name        string
		policy      Policy
		original    Record
		other       Record
		expected    Value
		expectError bool
	}{"Different Dimensions", Policy_Sum, Record{
		metrics:    counter,
		value:      10,
		cnt:        1,
		dimensions: Dimension{"host": "server1"},
	}, Record{
		metrics:    counter,
		value:      20,
		cnt:        1,
		dimensions: Dimension{"host": "server2"},
	}, 0, true})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.original.Merge(tt.other)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				// 对于PolicyAvg，我们需要检查平均值是否正确
				if tt.policy == Policy_Avg || tt.policy == Policy_Stopwatch {
					value, cnt := tt.original.RawData()
					expectedValue, expectedCount := tt.original.value, tt.original.cnt
					if value != expectedValue || cnt != expectedCount {
						t.Errorf("Expected merged value=%v, cnt=%d, got value=%v, cnt=%d",
							expectedValue, expectedCount, value, cnt)
					}
				} else if tt.original.Value() != tt.expected {
					t.Errorf("Expected merged value to be %v, got %v", tt.expected, tt.original.Value())
				}
			}
		})
	}
}
