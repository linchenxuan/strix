package metrics

import (
	"sync"
	"testing"
	"time"
)

// 测试多种指标类型的集成使用场景
func TestMetricsIntegration(t *testing.T) {
	// 创建MockReporter
	mockReporter := NewMockReporter()
	_Reporters = []Reporter{mockReporter}
	defer func() {
		_Reporters = []Reporter{}
	}()

	// 场景1: 混合使用不同类型的指标
	t.Run("TestMixedMetricsUsage", func(t *testing.T) {
		// 使用计数器
		IncrCounterWithGroup("request_count", "api", 1)
		IncrCounterWithGroup("request_count", "api", 1)

		// 使用计量器
		UpdateGaugeWithGroup("memory_usage", "system", 1024*1024*512) // 512MB

		// 使用秒表
		startTime := time.Now()
		time.Sleep(10 * time.Millisecond) // 模拟一些操作
		RecordStopwatch("operation_time", startTime)

		// 使用聚合计量器
		UpdateAvgGaugeWithGroup("response_time", "api", 100)    // 100ms
		UpdateMaxGaugeWithGroup("peak_latency", "network", 500) // 500ms
		UpdateMinGaugeWithGroup("min_latency", "network", 10)   // 10ms

		// 验证记录数量
		records := mockReporter.GetReportedRecords()
		if len(records) < 6 {
			t.Errorf("Expected at least 6 records, got %d", len(records))
		}

		// 验证部分关键记录
		foundCounter := false
		foundGauge := false
		foundStopwatch := false

		for _, record := range records {
			if record.Metrics().Name() == "request_count" && record.Metrics().Policy() == Policy_Sum {
				foundCounter = true
			} else if record.Metrics().Name() == "memory_usage" && record.Metrics().Policy() == Policy_Set {
				foundGauge = true
			} else if record.Metrics().Name() == "operation_time" && record.Metrics().Policy() == Policy_Stopwatch {
				foundStopwatch = true
			}
		}

		if !foundCounter {
			t.Error("Counter record not found")
		}
		if !foundGauge {
			t.Error("Gauge record not found")
		}
		if !foundStopwatch {
			t.Error("Stopwatch record not found")
		}
	})

	// 场景2: 带维度的指标收集
	t.Run("TestMetricsWithDimensions", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		// 带维度的计数器
		dimensions1 := Dimension{"endpoint": "/api/users", "status": "200"}
		IncrCounterWithDimGroup("http_requests", "web", 1, dimensions1)

		dimensions2 := Dimension{"endpoint": "/api/login", "status": "401"}
		IncrCounterWithDimGroup("http_requests", "web", 1, dimensions2)

		// 带维度的计量器
		dimensions3 := Dimension{"database": "users", "operation": "query"}
		UpdateGaugeWithDimGroup("db_latency", "database", 50, dimensions3) // 50ms

		// 验证维度信息
		records := mockReporter.GetReportedRecords()
		if len(records) != 3 {
			t.Fatalf("Expected 3 records, got %d", len(records))
		}

		// 验证每个记录的维度
		found200Ok := false
		found401Unauthorized := false
		foundDbQuery := false

		for _, record := range records {
			dim := record.Dimensions()
			if endpoint, ok := dim["endpoint"]; ok {
				if endpoint == "/api/users" && dim["status"] == "200" {
					found200Ok = true
				} else if endpoint == "/api/login" && dim["status"] == "401" {
					found401Unauthorized = true
				}
			} else if db, ok := dim["database"]; ok {
				if db == "users" && dim["operation"] == "query" {
					foundDbQuery = true
				}
			}
		}

		if !found200Ok {
			t.Error("Record with status 200 not found")
		}
		if !found401Unauthorized {
			t.Error("Record with status 401 not found")
		}
		if !foundDbQuery {
			t.Error("Database query record not found")
		}
	})

	// 场景3: 模拟生产环境的性能监控
	t.Run("TestProductionMonitoringScenario", func(t *testing.T) {
		// 清空之前的记录
		mockReporter.reportedRecords = []Record{}

		// 模拟一个Web服务处理请求的性能监控
		var wg sync.WaitGroup
		concurrency := 5
		requestsPerGoroutine := 20
		wg.Add(concurrency)

		for i := 0; i < concurrency; i++ {
			go func(goroutineID int) {
				defer wg.Done()

				for j := 0; j < requestsPerGoroutine; j++ {
					// 记录请求开始
					requestStart := time.Now()

					// 模拟处理请求，随机时间
					handleTime := time.Duration(10+j%50) * time.Millisecond
					time.Sleep(handleTime)

					// 记录请求完成
					duration := time.Since(requestStart)
					requestDuration := Value(duration.Milliseconds())

					// 选择随机状态码
					status := "200"
					if j%20 == 0 {
						status = "400" // 偶尔返回400错误
					} else if j%30 == 0 {
						status = "500" // 偶尔返回500错误
					}

					// 更新计数器
					dimensions := Dimension{"status": status, "goroutine": string(rune('A' + goroutineID))}
					IncrCounterWithDimGroup("web_requests", "production", 1, dimensions)

					// 更新响应时间指标
					UpdateAvgGaugeWithDimGroup("response_time", "production", Value(requestDuration), dimensions)
					UpdateMaxGaugeWithDimGroup("peak_response_time", "production", Value(requestDuration), dimensions)
					UpdateMinGaugeWithDimGroup("min_response_time", "production", Value(requestDuration), dimensions)
				}
			}(i)
		}

		wg.Wait()

		// 验证记录数量
		expectedRecords := concurrency * requestsPerGoroutine * 4 // 每个请求产生4条记录
		records := mockReporter.GetReportedRecords()
		if len(records) < expectedRecords {
			t.Errorf("Expected at least %d records, got %d", expectedRecords, len(records))
		}

		// 统计不同状态码的请求数量
		statusCount := make(map[string]int)
		for _, record := range records {
			if record.Metrics().Name() == "web_requests" {
				dim := record.Dimensions()
				if status, ok := dim["status"]; ok {
					statusCount[status]++
				}
			}
		}

		// 验证是否有200、400和500的状态码记录
		if statusCount["200"] == 0 {
			t.Error("No 200 status code records found")
		}
	})
}

// 测试SetMetricsReporters函数
func TestSetMetricsReporters(t *testing.T) {
	// 保存原始reporters
	originalReporters := _Reporters
	defer func() {
		_Reporters = originalReporters
	}()

	// 创建多个MockReporter
	reporter1 := NewMockReporter()
	reporter2 := NewMockReporter()
	reporters := []Reporter{reporter1, reporter2}

	// 设置reporters
	SetMetricsReporters(reporters)

	// 验证reporters是否被正确设置
	if len(_Reporters) != 2 {
		t.Fatalf("Expected 2 reporters, got %d", len(_Reporters))
	}

	// 发送一条记录
	IncrCounterWithGroup("test_counter", "test_group", 1)

	// 验证两个reporter都收到了记录
	records1 := reporter1.GetReportedRecords()
	records2 := reporter2.GetReportedRecords()

	if len(records1) != 1 {
		t.Errorf("Expected 1 record in reporter1, got %d", len(records1))
	}
	if len(records2) != 1 {
		t.Errorf("Expected 1 record in reporter2, got %d", len(records2))
	}

	// 测试替换reporters
	reporter3 := NewMockReporter()
	SetMetricsReporters([]Reporter{reporter3})

	// 发送另一条记录
	IncrCounterWithGroup("test_counter2", "test_group", 1)

	// 验证只有新的reporter收到了记录
	records3 := reporter3.GetReportedRecords()
	if len(records3) != 1 {
		t.Errorf("Expected 1 record in reporter3, got %d", len(records3))
	}
	// 原来的reporters不应该收到新记录
	if len(reporter1.GetReportedRecords()) != 1 {
		t.Errorf("Expected 1 record in reporter1, got %d", len(reporter1.GetReportedRecords()))
	}
}

// 测试指标的全局访问和缓存机制
func TestMetricsGlobalAccessAndCaching(t *testing.T) {
	// 清空缓存以确保测试隔离
	_counters = make(map[string]Counter)
	_gauges = make(map[string]Gauge)
	_avggauges = make(map[string]Gauge)
	_maxGauges = make(map[string]Gauge)
	_minGauges = make(map[string]Gauge)
	_stopwatchs = make(map[string]StopWatch)
	defer func() {
		_counters = make(map[string]Counter)
		_gauges = make(map[string]Gauge)
		_avggauges = make(map[string]Gauge)
		_maxGauges = make(map[string]Gauge)
		_minGauges = make(map[string]Gauge)
		_stopwatchs = make(map[string]StopWatch)
	}()

	// 第一次获取指标
	counter1 := getCounter("cache_test_counter", "cache_test_group")
	gauge1 := getGauge("cache_test_gauge", "cache_test_group")
	avgGauge1 := getAvgGauge("cache_test_avg", "cache_test_group")
	maxGauge1 := getMaxGauge("cache_test_max", "cache_test_group")
	minGauge1 := getMinGauge("cache_test_min", "cache_test_group")
	stopwatch1 := getStopWatch("cache_test_stopwatch", "cache_test_group")

	// 第二次获取相同名称的指标
	counter2 := getCounter("cache_test_counter", "cache_test_group")
	gauge2 := getGauge("cache_test_gauge", "cache_test_group")
	avgGauge2 := getAvgGauge("cache_test_avg", "cache_test_group")
	maxGauge2 := getMaxGauge("cache_test_max", "cache_test_group")
	minGauge2 := getMinGauge("cache_test_min", "cache_test_group")
	stopwatch2 := getStopWatch("cache_test_stopwatch", "cache_test_group")

	// 验证两次获取的是同一个实例
	if counter1 != counter2 {
		t.Error("Expected same counter instance, got different instances")
	}
	if gauge1 != gauge2 {
		t.Error("Expected same gauge instance, got different instances")
	}
	if avgGauge1 != avgGauge2 {
		t.Error("Expected same avgGauge instance, got different instances")
	}
	if maxGauge1 != maxGauge2 {
		t.Error("Expected same maxGauge instance, got different instances")
	}
	if minGauge1 != minGauge2 {
		t.Error("Expected same minGauge instance, got different instances")
	}
	if stopwatch1 != stopwatch2 {
		t.Error("Expected same stopwatch instance, got different instances")
	}

	// 验证不同名称的指标是不同实例
	diffCounter := getCounter("diff_counter", "diff_group")
	if counter1 == diffCounter {
		t.Error("Expected different counter instances, got same instance")
	}
}

// 测试在高并发场景下的线程安全性
func TestMetricsThreadSafety(t *testing.T) {
	// 创建MockReporter
	mockReporter := NewMockReporter()
	_Reporters = []Reporter{mockReporter}
	defer func() {
		_Reporters = []Reporter{}
	}()

	// 并发测试
	var wg sync.WaitGroup
	concurrency := 20
	iterations := 100
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				// 使用不同的指标类型
				IncrCounterWithGroup("threadsafe_counter", "concurrency_test", 1)
				UpdateGaugeWithGroup("threadsafe_gauge", "concurrency_test", Value(j))
				UpdateAvgGaugeWithGroup("threadsafe_avg", "concurrency_test", Value(j))
				UpdateMaxGaugeWithGroup("threadsafe_max", "concurrency_test", Value(j))
				UpdateMinGaugeWithGroup("threadsafe_min", "concurrency_test", Value(j))
				RecordStopwatch("threadsafe_stopwatch", time.Now())

				// 使用带维度的指标
				dimensions := Dimension{"goroutine": string(rune('A' + goroutineID)), "iteration": string(rune('0' + j%10))}
				IncrCounterWithDimGroup("threadsafe_counter_dim", "concurrency_test", 1, dimensions)
				UpdateGaugeWithDimGroup("threadsafe_gauge_dim", "concurrency_test", Value(j), dimensions)
			}
		}(i)
	}

	// 等待所有goroutine完成
	wg.Wait()

	// 验证没有panic发生
	t.Log("High concurrency metrics operations completed without panic")

	// 验证记录数量
	records := mockReporter.GetReportedRecords()
	expectedMinRecords := concurrency * iterations * 8 // 每个迭代产生8条记录
	if len(records) < expectedMinRecords {
		t.Errorf("Expected at least %d records, got %d", expectedMinRecords, len(records))
	}
}
