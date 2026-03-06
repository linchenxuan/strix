package metrics

import (
	"sync"
	"testing"
)

// 测试Gauge接口的基本功能
func TestGauge(t *testing.T) {
	// 创建MockReporter
	mockReporter := NewMockReporter()
	_Reporters = []Reporter{mockReporter}
	defer func() {
		_Reporters = []Reporter{}
	}()

	// 创建计量器
	gauge := getGauge("test_gauge", "test_group")

	// 测试基本更新功能
	t.Run("TestGaugeUpdate", func(t *testing.T) {
		gauge.Update(100)
		records := mockReporter.GetReportedRecords()
		if len(records) != 1 {
			t.Fatalf("Expected 1 record, got %d", len(records))
		}

		record := records[0]
		if record.Value() != 100 {
			t.Errorf("Expected value 100, got %v", record.Value())
		}
		if record.Metrics().Name() != "test_gauge" {
			t.Errorf("Expected name 'test_gauge', got '%s'", record.Metrics().Name())
		}
		if record.Metrics().Group() != "test_group" {
			t.Errorf("Expected group 'test_group', got '%s'", record.Metrics().Group())
		}
		if record.Metrics().Policy() != Policy_Set {
			t.Errorf("Expected policy Policy_Set, got %v", record.Metrics().Policy())
		}
	})

	// 测试带维度的更新功能
	t.Run("TestGaugeUpdateWithDim", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		dimensions := Dimension{"instance": "instance1", "zone": "zone-a"}
		gauge.UpdateWithDim(200, dimensions)
		records := mockReporter.GetReportedRecords()
		if len(records) != 1 {
			t.Fatalf("Expected 1 record, got %d", len(records))
		}

		record := records[0]
		if record.Value() != 200 {
			t.Errorf("Expected value 200, got %v", record.Value())
		}
		dim := record.Dimensions()
		if dim["instance"] != "instance1" {
			t.Errorf("Expected dimension instance 'instance1', got '%s'", dim["instance"])
		}
		if dim["zone"] != "zone-a" {
			t.Errorf("Expected dimension zone 'zone-a', got '%s'", dim["zone"])
		}
	})

	// 测试多次更新功能
	t.Run("TestGaugeMultipleUpdates", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		values := []Value{50, 150, 75, 200, 100}
		for _, v := range values {
			gauge.Update(v)
		}

		records := mockReporter.GetReportedRecords()
		if len(records) != len(values) {
			t.Fatalf("Expected %d records, got %d", len(values), len(records))
		}

		// 验证每个记录的值
		for i, v := range values {
			if records[i].Value() != v {
				t.Errorf("Expected record %d to have value %v, got %v", i, v, records[i].Value())
			}
		}
	})

	// 测试并发更新功能
	t.Run("TestGaugeConcurrent", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		var wg sync.WaitGroup
		concurrency := 5
		iterations := 20
		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					gauge.Update(Value(id*100 + j))
				}
			}(i)
		}
		wg.Wait()

		expectedRecords := concurrency * iterations
		records := mockReporter.GetReportedRecords()
		if len(records) != expectedRecords {
			t.Errorf("Expected %d records, got %d", expectedRecords, len(records))
		}
	})
}

// 测试Gauge的工具函数
func TestGaugeHelperFunctions(t *testing.T) {
	// 创建MockReporter
	mockReporter := NewMockReporter()
	_Reporters = []Reporter{mockReporter}
	defer func() {
		_Reporters = []Reporter{}
	}()

	// 测试UpdateGaugeWithGroup函数
	UpdateGaugeWithGroup("helper_gauge", "helper_group", 300)
	records := mockReporter.GetReportedRecords()
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	record := records[0]
	if record.Value() != 300 {
		t.Errorf("Expected value 300, got %v", record.Value())
	}
	if record.Metrics().Name() != "helper_gauge" {
		t.Errorf("Expected name 'helper_gauge', got '%s'", record.Metrics().Name())
	}
	if record.Metrics().Group() != "helper_group" {
		t.Errorf("Expected group 'helper_group', got '%s'", record.Metrics().Group())
	}

	// 测试UpdateGaugeWithDimGroup函数
	mockReporter.reportedRecords = []Record{}
	dimensions := Dimension{"service": "api", "env": "dev"}
	UpdateGaugeWithDimGroup("dim_gauge", "dim_group", 400, dimensions)
	records = mockReporter.GetReportedRecords()
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	record = records[0]
	if record.Value() != 400 {
		t.Errorf("Expected value 400, got %v", record.Value())
	}
	if record.Metrics().Name() != "dim_gauge" {
		t.Errorf("Expected name 'dim_gauge', got '%s'", record.Metrics().Name())
	}
	dim := record.Dimensions()
	if dim["service"] != "api" {
		t.Errorf("Expected dimension service 'api', got '%s'", dim["service"])
	}
	if dim["env"] != "dev" {
		t.Errorf("Expected dimension env 'dev', got '%s'", dim["env"])
	}
}
