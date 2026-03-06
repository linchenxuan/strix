package metrics

import (
	"sync"
	"testing"
	"time"
)

// 测试StopWatch接口的基本功能
func TestStopWatch(t *testing.T) {
	// 创建MockReporter
	mockReporter := NewMockReporter()
	_Reporters = []Reporter{mockReporter}
	defer func() {
		_Reporters = []Reporter{}
	}()

	// 创建秒表
	stopwatch := getStopWatch("test_stopwatch", "test_group")

	// 测试基本计时功能
	t.Run("TestStopWatchRecord", func(t *testing.T) {
		startTime := time.Now()
		time.Sleep(10 * time.Millisecond) // 模拟一些操作
		duration := stopwatch.RecordWithDim(nil, startTime)

		// 检查返回的持续时间是否合理
		if duration < 10*time.Millisecond || duration > 100*time.Millisecond {
			t.Errorf("Expected duration between 10ms and 100ms, got %v", duration)
		}

		// 检查记录是否正确上报
		records := mockReporter.GetReportedRecords()
		if len(records) != 1 {
			t.Fatalf("Expected 1 record, got %d", len(records))
		}

		record := records[0]
		// 转换为毫秒检查值是否合理
		recordValue := float64(duration.Microseconds()) / 1000
		if Value(recordValue) < 10 || Value(recordValue) > 100 {
			t.Errorf("Expected record value between 10 and 100, got %v", record.Value())
		}
		if record.Metrics().Name() != "test_stopwatch" {
			t.Errorf("Expected name 'test_stopwatch', got '%s'", record.Metrics().Name())
		}
		if record.Metrics().Group() != "test_group" {
			t.Errorf("Expected group 'test_group', got '%s'", record.Metrics().Group())
		}
		if record.Metrics().Policy() != Policy_Stopwatch {
			t.Errorf("Expected policy Policy_Stopwatch, got %v", record.Metrics().Policy())
		}
		if record.cnt != 1 {
			t.Errorf("Expected count 1, got %d", record.cnt)
		}
	})

	// 测试带维度的计时功能
	t.Run("TestStopWatchRecordWithDim", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		startTime := time.Now()
		time.Sleep(5 * time.Millisecond) // 模拟一些操作
		dimensions := Dimension{"endpoint": "/api/test", "method": "GET"}
		duration := stopwatch.RecordWithDim(dimensions, startTime)

		// 检查返回的持续时间是否合理
		if duration < 5*time.Millisecond || duration > 50*time.Millisecond {
			t.Errorf("Expected duration between 5ms and 50ms, got %v", duration)
		}

		// 检查记录是否正确上报
		records := mockReporter.GetReportedRecords()
		if len(records) != 1 {
			t.Fatalf("Expected 1 record, got %d", len(records))
		}

		record := records[0]
		dim := record.Dimensions()
		if dim["endpoint"] != "/api/test" {
			t.Errorf("Expected dimension endpoint '/api/test', got '%s'", dim["endpoint"])
		}
		if dim["method"] != "GET" {
			t.Errorf("Expected dimension method 'GET', got '%s'", dim["method"])
		}
	})

	// 测试多次计时功能
	t.Run("TestStopWatchMultipleRecords", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		// 进行多次计时
		iterations := 3
		for i := 0; i < iterations; i++ {
			startTime := time.Now()
			time.Sleep(time.Duration(i+1) * 5 * time.Millisecond) // 每次睡眠时间递增
			stopwatch.RecordWithDim(nil, startTime)
		}

		// 检查记录数量
		records := mockReporter.GetReportedRecords()
		if len(records) != iterations {
			t.Fatalf("Expected %d records, got %d", iterations, len(records))
		}

		// 验证每次记录的时间递增
		for i := 1; i < iterations; i++ {
			prevValue := records[i-1].Value()
			currValue := records[i].Value()
			if currValue <= prevValue {
				t.Errorf("Expected record %d to be greater than record %d, got %v <= %v",
					i, i-1, currValue, prevValue)
			}
		}
	})

	// 测试并发计时功能
	t.Run("TestStopWatchConcurrent", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		var wg sync.WaitGroup
		concurrency := 5
		wg.Add(concurrency)
		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				startTime := time.Now()
				time.Sleep(2 * time.Millisecond) // 模拟一些操作
				stopwatch.RecordWithDim(nil, startTime)
			}()
		}
		wg.Wait()

		// 等待一小段时间确保所有报告都已完成
		time.Sleep(10 * time.Millisecond)

		records := mockReporter.GetReportedRecords()
		if len(records) != concurrency {
			t.Errorf("Expected %d records, got %d", concurrency, len(records))
		}

		// 验证所有记录的值都大于0
		for i, record := range records {
			if record.Value() <= 0 {
				t.Errorf("Expected record %d value to be greater than 0, got %v", i, record.Value())
			}
		}
	})
}

// 测试StopWatch的工具函数
func TestStopWatchHelperFunctions(t *testing.T) {
	// 创建MockReporter
	mockReporter := NewMockReporter()
	_Reporters = []Reporter{mockReporter}
	defer func() {
		_Reporters = []Reporter{}
	}()

	// 测试RecordStopwatch函数
	startTime1 := time.Now()
	time.Sleep(8 * time.Millisecond)
	duration1 := RecordStopwatch("helper_stopwatch", startTime1)

	// 检查返回的持续时间是否合理
	if duration1 < 8*time.Millisecond || duration1 > 80*time.Millisecond {
		t.Errorf("Expected duration between 8ms and 80ms, got %v", duration1)
	}

	// 检查记录是否正确上报
	records := mockReporter.GetReportedRecords()
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	record := records[0]
	if record.Metrics().Name() != "helper_stopwatch" {
		t.Errorf("Expected name 'helper_stopwatch', got '%s'", record.Metrics().Name())
	}
	if record.Metrics().Group() != "" {
		t.Errorf("Expected empty group, got '%s'", record.Metrics().Group())
	}

	// 测试RecordStopwatchWithGroup函数
	mockReporter.reportedRecords = []Record{}
	startTime2 := time.Now()
	time.Sleep(6 * time.Millisecond)
	duration2 := RecordStopwatchWithGroup("group_stopwatch", "timer_group", startTime2)

	// 检查返回的持续时间是否合理
	if duration2 < 6*time.Millisecond || duration2 > 60*time.Millisecond {
		t.Errorf("Expected duration between 6ms and 60ms, got %v", duration2)
	}

	// 检查记录是否正确上报
	records = mockReporter.GetReportedRecords()
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	record = records[0]
	if record.Metrics().Name() != "group_stopwatch" {
		t.Errorf("Expected name 'group_stopwatch', got '%s'", record.Metrics().Name())
	}
	if record.Metrics().Group() != "timer_group" {
		t.Errorf("Expected group 'timer_group', got '%s'", record.Metrics().Group())
	}

	// 测试RecordStopwatchWithDimGroup函数
	mockReporter.reportedRecords = []Record{}
	startTime3 := time.Now()
	time.Sleep(4 * time.Millisecond)
	dimensions := Dimension{"operation": "query", "database": "users"}
	duration3 := RecordStopwatchWithDimGroup("dim_stopwatch", "perf_group", startTime3, dimensions)

	// 检查返回的持续时间是否合理
	if duration3 < 4*time.Millisecond || duration3 > 40*time.Millisecond {
		t.Errorf("Expected duration between 4ms and 40ms, got %v", duration3)
	}

	// 检查记录是否正确上报
	records = mockReporter.GetReportedRecords()
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	record = records[0]
	if record.Metrics().Name() != "dim_stopwatch" {
		t.Errorf("Expected name 'dim_stopwatch', got '%s'", record.Metrics().Name())
	}
	if record.Metrics().Group() != "perf_group" {
		t.Errorf("Expected group 'perf_group', got '%s'", record.Metrics().Group())
	}
	dim := record.Dimensions()
	if dim["operation"] != "query" {
		t.Errorf("Expected dimension operation 'query', got '%s'", dim["operation"])
	}
	if dim["database"] != "users" {
		t.Errorf("Expected dimension database 'users', got '%s'", dim["database"])
	}
}
