package metrics

import (
	"sync"
	"testing"
)

// 测试平均计量器(avgGauge)的功能
func TestAvgGauge(t *testing.T) {
	// 创建MockReporter
	mockReporter := NewMockReporter()
	_Reporters = []Reporter{mockReporter}
	defer func() {
		_Reporters = []Reporter{}
	}()

	// 创建平均计量器
	avgGauge := getAvgGauge("test_avg", "test_group")

	// 测试基本更新功能
	t.Run("TestAvgGaugeUpdate", func(t *testing.T) {
		avgGauge.Update(100)
		records := mockReporter.GetReportedRecords()
		if len(records) != 1 {
			t.Fatalf("Expected 1 record, got %d", len(records))
		}

		record := records[0]
		// 对于avgGauge，单个值的平均值就是它本身
		if record.Value() != 100 {
			t.Errorf("Expected value 100, got %v", record.Value())
		}
		if record.Metrics().Name() != "test_avg" {
			t.Errorf("Expected name 'test_avg', got '%s'", record.Metrics().Name())
		}
		if record.Metrics().Policy() != Policy_Avg {
			t.Errorf("Expected policy Policy_Avg, got %v", record.Metrics().Policy())
		}
		if record.cnt != 1 {
			t.Errorf("Expected count 1, got %d", record.cnt)
		}
	})

	// 测试带维度的更新功能
	t.Run("TestAvgGaugeUpdateWithDim", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		dimensions := Dimension{"source": "api", "version": "v1"}
		avgGauge.UpdateWithDim(200, dimensions)
		records := mockReporter.GetReportedRecords()
		if len(records) != 1 {
			t.Fatalf("Expected 1 record, got %d", len(records))
		}

		record := records[0]
		if record.Value() != 200 {
			t.Errorf("Expected value 200, got %v", record.Value())
		}
		dim := record.Dimensions()
		if dim["source"] != "api" {
			t.Errorf("Expected dimension source 'api', got '%s'", dim["source"])
		}
		if dim["version"] != "v1" {
			t.Errorf("Expected dimension version 'v1', got '%s'", dim["version"])
		}
	})

	// 测试聚合逻辑（通过Merge方法）
	t.Run("TestAvgGaugeAggregation", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		// 创建两个记录进行合并测试
		dimensions := Dimension{"type": "measurement"}
		avgGauge.UpdateWithDim(50, dimensions)
		avgGauge.UpdateWithDim(150, dimensions)

		// 注意：这里我们不能直接测试avgGauge的聚合功能，因为聚合通常是由reporter完成的
		// 但我们可以验证记录中的value和cnt是否正确，以便reporter能够正确计算平均值
		records := mockReporter.GetReportedRecords()
		if len(records) != 2 {
			t.Fatalf("Expected 2 records, got %d", len(records))
		}

		// 验证每条记录的value和cnt
		for i, record := range records {
			if i == 0 && record.value != 50 {
				t.Errorf("Expected first record value 50, got %v", record.value)
			}
			if i == 1 && record.value != 150 {
				t.Errorf("Expected second record value 150, got %v", record.value)
			}
			if record.cnt != 1 {
				t.Errorf("Expected record count 1, got %d", record.cnt)
			}
		}
	})
}

// 测试最大值计量器(maxGauge)的功能
func TestMaxGauge(t *testing.T) {
	// 创建MockReporter
	mockReporter := NewMockReporter()
	_Reporters = []Reporter{mockReporter}
	defer func() {
		_Reporters = []Reporter{}
	}()

	// 创建最大值计量器
	maxGauge := getMaxGauge("test_max", "test_group")

	// 测试基本更新功能
	t.Run("TestMaxGaugeUpdate", func(t *testing.T) {
		maxGauge.Update(100)
		records := mockReporter.GetReportedRecords()
		if len(records) != 1 {
			t.Fatalf("Expected 1 record, got %d", len(records))
		}

		record := records[0]
		if record.Value() != 100 {
			t.Errorf("Expected value 100, got %v", record.Value())
		}
		if record.Metrics().Name() != "test_max" {
			t.Errorf("Expected name 'test_max', got '%s'", record.Metrics().Name())
		}
		if record.Metrics().Policy() != Policy_Max {
			t.Errorf("Expected policy Policy_Max, got %v", record.Metrics().Policy())
		}
	})

	// 测试带维度的更新功能
	t.Run("TestMaxGaugeUpdateWithDim", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		dimensions := Dimension{"server": "server1", "region": "east"}
		maxGauge.UpdateWithDim(200, dimensions)
		records := mockReporter.GetReportedRecords()
		if len(records) != 1 {
			t.Fatalf("Expected 1 record, got %d", len(records))
		}

		record := records[0]
		if record.Value() != 200 {
			t.Errorf("Expected value 200, got %v", record.Value())
		}
		dim := record.Dimensions()
		if dim["server"] != "server1" {
			t.Errorf("Expected dimension server 'server1', got '%s'", dim["server"])
		}
		if dim["region"] != "east" {
			t.Errorf("Expected dimension region 'east', got '%s'", dim["region"])
		}
	})

	// 测试最大值逻辑（通过Merge方法）
	t.Run("TestMaxGaugeAggregation", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		// 创建多个记录进行最大值测试
		dimensions := Dimension{"metric": "performance"}
		values := []Value{50, 150, 75, 200, 125}
		for _, v := range values {
			maxGauge.UpdateWithDim(v, dimensions)
		}

		records := mockReporter.GetReportedRecords()
		if len(records) != len(values) {
			t.Fatalf("Expected %d records, got %d", len(values), len(records))
		}

		// 验证包含最大值的记录
		maxFound := false
		for _, record := range records {
			if record.value == 200 {
				maxFound = true
				break
			}
		}
		if !maxFound {
			t.Error("Expected to find a record with value 200")
		}
	})
}

// 测试最小值计量器(minGauge)的功能
func TestMinGauge(t *testing.T) {
	// 创建MockReporter
	mockReporter := NewMockReporter()
	_Reporters = []Reporter{mockReporter}
	defer func() {
		_Reporters = []Reporter{}
	}()

	// 创建最小值计量器
	minGauge := getMinGauge("test_min", "test_group")

	// 测试基本更新功能
	t.Run("TestMinGaugeUpdate", func(t *testing.T) {
		minGauge.Update(100)
		records := mockReporter.GetReportedRecords()
		if len(records) != 1 {
			t.Fatalf("Expected 1 record, got %d", len(records))
		}

		record := records[0]
		if record.Value() != 100 {
			t.Errorf("Expected value 100, got %v", record.Value())
		}
		if record.Metrics().Name() != "test_min" {
			t.Errorf("Expected name 'test_min', got '%s'", record.Metrics().Name())
		}
		if record.Metrics().Policy() != Policy_Min {
			t.Errorf("Expected policy Policy_Min, got %v", record.Metrics().Policy())
		}
	})

	// 测试带维度的更新功能
	t.Run("TestMinGaugeUpdateWithDim", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		dimensions := Dimension{"client": "mobile", "platform": "ios"}
		minGauge.UpdateWithDim(50, dimensions)
		records := mockReporter.GetReportedRecords()
		if len(records) != 1 {
			t.Fatalf("Expected 1 record, got %d", len(records))
		}

		record := records[0]
		if record.Value() != 50 {
			t.Errorf("Expected value 50, got %v", record.Value())
		}
		dim := record.Dimensions()
		if dim["client"] != "mobile" {
			t.Errorf("Expected dimension client 'mobile', got '%s'", dim["client"])
		}
		if dim["platform"] != "ios" {
			t.Errorf("Expected dimension platform 'ios', got '%s'", dim["platform"])
		}
	})

	// 测试最小值逻辑（通过Merge方法）
	t.Run("TestMinGaugeAggregation", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		// 创建多个记录进行最小值测试
		dimensions := Dimension{"metric": "response_time"}
		values := []Value{150, 50, 200, 75, 100}
		for _, v := range values {
			minGauge.UpdateWithDim(v, dimensions)
		}

		records := mockReporter.GetReportedRecords()
		if len(records) != len(values) {
			t.Fatalf("Expected %d records, got %d", len(values), len(records))
		}

		// 验证包含最小值的记录
		minFound := false
		for _, record := range records {
			if record.value == 50 {
				minFound = true
				break
			}
		}
		if !minFound {
			t.Error("Expected to find a record with value 50")
		}
	})
}

// 测试聚合计量器的工具函数
func TestAggregatorHelperFunctions(t *testing.T) {
	// 创建MockReporter
	mockReporter := NewMockReporter()
	_Reporters = []Reporter{mockReporter}
	defer func() {
		_Reporters = []Reporter{}
	}()

	// 测试UpdateAvgGaugeWithGroup函数
	UpdateAvgGaugeWithGroup("helper_avg", "helper_group", 300)
	records := mockReporter.GetReportedRecords()
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	record := records[0]
	if record.Value() != 300 {
		t.Errorf("Expected value 300, got %v", record.Value())
	}
	if record.Metrics().Name() != "helper_avg" {
		t.Errorf("Expected name 'helper_avg', got '%s'", record.Metrics().Name())
	}
	if record.cnt != 1 {
		t.Errorf("Expected count 1, got %d", record.cnt)
	}

	// 测试UpdateAvgGaugeWithDimGroup函数
	mockReporter.reportedRecords = []Record{}
	dimensions := Dimension{"type": "latency", "unit": "ms"}
	UpdateAvgGaugeWithDimGroup("dim_avg", "dim_group", 400, dimensions)
	records = mockReporter.GetReportedRecords()
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	record = records[0]
	if record.Value() != 400 {
		t.Errorf("Expected value 400, got %v", record.Value())
	}
	if record.Metrics().Name() != "dim_avg" {
		t.Errorf("Expected name 'dim_avg', got '%s'", record.Metrics().Name())
	}
	if record.cnt != 1 {
		t.Errorf("Expected count 1, got %d", record.cnt)
	}

	// 测试UpdateMaxGaugeWithGroup函数
	mockReporter.reportedRecords = []Record{}
	UpdateMaxGaugeWithGroup("helper_max", "helper_group", 500)
	records = mockReporter.GetReportedRecords()
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	record = records[0]
	if record.Value() != 500 {
		t.Errorf("Expected value 500, got %v", record.Value())
	}
	if record.Metrics().Name() != "helper_max" {
		t.Errorf("Expected name 'helper_max', got '%s'", record.Metrics().Name())
	}

	// 测试UpdateMinGaugeWithGroup函数
	mockReporter.reportedRecords = []Record{}
	UpdateMinGaugeWithGroup("helper_min", "helper_group", 100)
	records = mockReporter.GetReportedRecords()
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	record = records[0]
	if record.Value() != 100 {
		t.Errorf("Expected value 100, got %v", record.Value())
	}
	if record.Metrics().Name() != "helper_min" {
		t.Errorf("Expected name 'helper_min', got '%s'", record.Metrics().Name())
	}
}

// 测试聚合计量器的并发更新
func TestAggregatorsConcurrent(t *testing.T) {
	// 创建MockReporter
	mockReporter := NewMockReporter()
	_Reporters = []Reporter{mockReporter}
	defer func() {
		_Reporters = []Reporter{}
	}()

	// 创建聚合计量器
	avgGauge := getAvgGauge("concurrent_avg", "concurrent_group")
	maxGauge := getMaxGauge("concurrent_max", "concurrent_group")
	minGauge := getMinGauge("concurrent_min", "concurrent_group")

	// 并发测试
	t.Run("TestAggregatorsConcurrentUpdates", func(t *testing.T) {
		var wg sync.WaitGroup
		concurrency := 10
		iterations := 50
		wg.Add(3 * concurrency) // 每个计量器并发10个goroutine

		// 并发更新avgGauge
		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					avgGauge.Update(Value(j % 100))
				}
			}()
		}

		// 并发更新maxGauge
		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					maxGauge.Update(Value(j % 100))
				}
			}()
		}

		// 并发更新minGauge
		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					minGauge.Update(Value(j % 100))
				}
			}()
		}

		wg.Wait()

		// 验证所有记录都被正确上报
		expectedRecords := 3 * concurrency * iterations
		records := mockReporter.GetReportedRecords()
		if len(records) < expectedRecords {
			t.Errorf("Expected at least %d records, got %d", expectedRecords, len(records))
		}

		// 确保没有panic发生
		t.Log("Concurrent updates to aggregators completed without panic")
	})
}
