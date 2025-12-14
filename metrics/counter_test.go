package metrics

import (
	"sync"
	"testing"
)

// MockReporter 用于测试的Mock Reporter实现
type MockReporter struct {
	reportedRecords []Record
	mu              sync.Mutex
}

// NewMockReporter 创建一个新的MockReporter
func NewMockReporter() *MockReporter {
	return &MockReporter{
		reportedRecords: []Record{},
	}
}

// Report 实现Reporter接口的Report方法
func (mr *MockReporter) Report(r Record) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.reportedRecords = append(mr.reportedRecords, *r.Clone())
}

// GetReportedRecords 获取所有上报的记录
func (mr *MockReporter) GetReportedRecords() []Record {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	return append([]Record{}, mr.reportedRecords...)
}

// 测试Counter接口的基本功能
func TestCounter(t *testing.T) {
	// 创建MockReporter
	mockReporter := NewMockReporter()
	_Reporters = []Reporter{mockReporter}
	defer func() {
		_Reporters = []Reporter{}
	}()

	// 创建计数器
	counter := getCounter("test_counter", "test_group")

	// 测试基本计数功能
	t.Run("TestCounterIncr", func(t *testing.T) {
		counter.Incr(10)
		records := mockReporter.GetReportedRecords()
		if len(records) != 1 {
			t.Fatalf("Expected 1 record, got %d", len(records))
		}

		record := records[0]
		if record.Value() != 10 {
			t.Errorf("Expected value 10, got %v", record.Value())
		}
		if record.Metrics().Name() != "test_counter" {
			t.Errorf("Expected name 'test_counter', got '%s'", record.Metrics().Name())
		}
		if record.Metrics().Group() != "test_group" {
			t.Errorf("Expected group 'test_group', got '%s'", record.Metrics().Group())
		}
		if record.Metrics().Policy() != Policy_Sum {
			t.Errorf("Expected policy Policy_Sum, got %v", record.Metrics().Policy())
		}
	})

	// 测试带维度的计数功能
	t.Run("TestCounterIncrWithDim", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		dimensions := Dimension{"host": "server1", "region": "us-west"}
		counter.IncrWithDim(5, dimensions)
		records := mockReporter.GetReportedRecords()
		if len(records) != 1 {
			t.Fatalf("Expected 1 record, got %d", len(records))
		}

		record := records[0]
		if record.Value() != 5 {
			t.Errorf("Expected value 5, got %v", record.Value())
		}
		dim := record.Dimensions()
		if dim["host"] != "server1" {
			t.Errorf("Expected dimension host 'server1', got '%s'", dim["host"])
		}
		if dim["region"] != "us-west" {
			t.Errorf("Expected dimension region 'us-west', got '%s'", dim["region"])
		}
	})

	// 测试并发计数功能
	t.Run("TestCounterConcurrent", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		var wg sync.WaitGroup
		concurrency := 10
		iterations := 100
		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					counter.Incr(1)
				}
			}()
		}
		wg.Wait()

		expectedRecords := concurrency * iterations
		records := mockReporter.GetReportedRecords()
		if len(records) != expectedRecords {
			t.Errorf("Expected %d records, got %d", expectedRecords, len(records))
		}
	})
}

// 测试Counter的工具函数
func TestCounterHelperFunctions(t *testing.T) {
	// 创建MockReporter
	mockReporter := NewMockReporter()
	_Reporters = []Reporter{mockReporter}
	defer func() {
		_Reporters = []Reporter{}
	}()

	// 测试IncrCounterWithGroup函数
	IncrCounterWithGroup("helper_counter", "helper_group", 20)
	records := mockReporter.GetReportedRecords()
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	record := records[0]
	if record.Value() != 20 {
		t.Errorf("Expected value 20, got %v", record.Value())
	}
	if record.Metrics().Name() != "helper_counter" {
		t.Errorf("Expected name 'helper_counter', got '%s'", record.Metrics().Name())
	}
	if record.Metrics().Group() != "helper_group" {
		t.Errorf("Expected group 'helper_group', got '%s'", record.Metrics().Group())
	}

	// 测试IncrCounterWithDimGroup函数
	mockReporter.reportedRecords = []Record{}
	dimensions := Dimension{"app": "test", "version": "1.0"}
	IncrCounterWithDimGroup("dim_counter", "dim_group", 15, dimensions)
	records = mockReporter.GetReportedRecords()
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	record = records[0]
	if record.Value() != 15 {
		t.Errorf("Expected value 15, got %v", record.Value())
	}
	if record.Metrics().Name() != "dim_counter" {
		t.Errorf("Expected name 'dim_counter', got '%s'", record.Metrics().Name())
	}
	dim := record.Dimensions()
	if dim["app"] != "test" {
		t.Errorf("Expected dimension app 'test', got '%s'", dim["app"])
	}
	if dim["version"] != "1.0" {
		t.Errorf("Expected dimension version '1.0', got '%s'", dim["version"])
	}
}
