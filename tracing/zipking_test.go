package tracing

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// 测试NewZipkinReporter函数
func TestNewZipkinReporter(t *testing.T) {
	endpoint := "http://localhost:9411"
	serviceName := "test-service"
	batchSize := 5

	reporter := NewZipkinReporter(endpoint, serviceName, batchSize)
	if reporter == nil {
		t.Fatal("NewZipkinReporter returned nil")
	}

	// 验证基本属性设置正确
	if reporter.endpoint != endpoint {
		t.Errorf("Expected endpoint %s, got %s", endpoint, reporter.endpoint)
	}
	if reporter.serviceName != serviceName {
		t.Errorf("Expected serviceName %s, got %s", serviceName, reporter.serviceName)
	}
	if reporter.batchSize != batchSize {
		t.Errorf("Expected batchSize %d, got %d", batchSize, reporter.batchSize)
	}

	// 测试默认batchSize
	reporter = NewZipkinReporter(endpoint, serviceName, 0)
	if reporter.batchSize != 100 {
		t.Errorf("Expected default batchSize 100, got %d", reporter.batchSize)
	}
}

// 测试convertToZipkin方法
func TestConvertToZipkin(t *testing.T) {
	reporter := NewZipkinReporter("http://localhost:9411", "test-service", 10)

	// 创建测试用的SpanData
	startTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	spanData := SpanData{
		TraceID:      "trace-123",
		SpanID:       "span-456",
		ParentSpanID: "parent-789",
		Operation:    "test-operation",
		StartTime:    startTime,
		Duration:     time.Second,
		Tags:         map[string]interface{}{"tag1": "value1", "tag2": 123},
	}

	// 转换为ZipkinSpan
	zipkinSpan := reporter.convertToZipkin(spanData)

	// 验证转换结果
	if zipkinSpan.TraceID != "trace-123" {
		t.Errorf("Expected TraceID 'trace-123', got %s", zipkinSpan.TraceID)
	}
	if zipkinSpan.ID != "span-456" {
		t.Errorf("Expected ID 'span-456', got %s", zipkinSpan.ID)
	}
	if zipkinSpan.ParentID != "parent-789" {
		t.Errorf("Expected ParentID 'parent-789', got %s", zipkinSpan.ParentID)
	}
	if zipkinSpan.Name != "test-operation" {
		t.Errorf("Expected Name 'test-operation', got %s", zipkinSpan.Name)
	}
	// 验证时间戳和持续时间（微秒）
	expectedTimestamp := startTime.UnixNano() / 1000
	if zipkinSpan.Timestamp != expectedTimestamp {
		t.Errorf("Expected Timestamp %d, got %d", expectedTimestamp, zipkinSpan.Timestamp)
	}
	expectedDuration := int64(time.Second) / 1000
	if zipkinSpan.Duration != expectedDuration {
		t.Errorf("Expected Duration %d, got %d", expectedDuration, zipkinSpan.Duration)
	}
	// 验证服务名
	if zipkinSpan.LocalEndpoint.ServiceName != "test-service" {
		t.Errorf("Expected ServiceName 'test-service', got %s", zipkinSpan.LocalEndpoint.ServiceName)
	}
	// 验证标签转换（所有值都应该转换为字符串）
	if zipkinSpan.Tags["tag1"] != "value1" {
		t.Errorf("Expected tag1 'value1', got %s", zipkinSpan.Tags["tag1"])
	}
	if zipkinSpan.Tags["tag2"] != "123" {
		t.Errorf("Expected tag2 '123', got %s", zipkinSpan.Tags["tag2"])
	}
}

// 测试Report和flush方法（使用mock HTTP服务器）
func TestZipkinReporterReportAndFlush(t *testing.T) {
	// 创建mock HTTP服务器
	var receivedSpans []ZipkinSpan
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 记录请求路径
		if r.URL.Path != "/api/v2/spans" {
			t.Errorf("Expected request to /api/v2/spans, got %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// 验证请求方法
		if r.Method != "POST" {
			t.Errorf("Expected POST method, got %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// 验证Content-Type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}

		// 读取并解析请求体
		decoder := json.NewDecoder(r.Body)
		var spans []ZipkinSpan
		if err := decoder.Decode(&spans); err != nil {
			t.Errorf("Failed to decode spans: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// 保存接收到的spans
		mu.Lock()
		receivedSpans = append(receivedSpans, spans...)
		mu.Unlock()

		// 返回成功响应
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 创建ZipkinReporter指向mock服务器
	reporter := NewZipkinReporter(server.URL, "test-service", 2)
	defer reporter.Close()

	// 创建测试用的SpanData
	spanData1 := SpanData{
		TraceID:   "trace-1",
		SpanID:    "span-1",
		Operation: "operation-1",
		StartTime: time.Now(),
		Duration:  time.Second,
	}

	spanData2 := SpanData{
		TraceID:   "trace-2",
		SpanID:    "span-2",
		Operation: "operation-2",
		StartTime: time.Now(),
		Duration:  time.Second,
	}

	// 测试单个span不会触发flush（因为批次未满）
	err := reporter.Report(spanData1)
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}

	// 此时应该还没有发送span，因为批次未满
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	if len(receivedSpans) != 0 {
		t.Errorf("Expected 0 received spans, got %d", len(receivedSpans))
	}
	mu.Unlock()

	// 添加第二个span，这应该触发批量报告
	err = reporter.Report(spanData2)
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}

	// 等待报告完成
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	if len(receivedSpans) != 2 {
		t.Errorf("Expected 2 received spans, got %d", len(receivedSpans))
	}
	mu.Unlock()
}

// 测试定时刷新功能
func TestZipkinReporterPeriodicFlush(t *testing.T) {
	// 创建mock HTTP服务器
	var receivedSpans []ZipkinSpan
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 解析请求体
		decoder := json.NewDecoder(r.Body)
		var spans []ZipkinSpan
		if err := decoder.Decode(&spans); err != nil {
			t.Errorf("Failed to decode spans: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// 保存接收到的spans
		mu.Lock()
		receivedSpans = append(receivedSpans, spans...)
		mu.Unlock()

		// 返回成功响应
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 修改reporter的reportLoop方法，使其使用更短的间隔进行测试
	reporter := NewZipkinReporter(server.URL, "test-service", 100) // 大的batchSize确保不会因为批次满而触发flush
	defer reporter.Close()

	// 创建测试用的SpanData
	spanData := SpanData{
		TraceID:   "trace-1",
		SpanID:    "span-1",
		Operation: "operation-1",
		StartTime: time.Now(),
		Duration:  time.Second,
	}

	// 添加span
	err := reporter.Report(spanData)
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}

	// 等待定时刷新（ZipkinReporter的reportLoop使用5秒间隔）
	t.Log("Waiting for periodic flush...")
	time.Sleep(5200 * time.Millisecond) // 等待超过5秒

	// 验证span被发送
	mu.Lock()
	if len(receivedSpans) < 1 {
		t.Errorf("Expected at least 1 received span after periodic flush, got %d", len(receivedSpans))
	}
	mu.Unlock()
}

// 测试Close方法
func TestZipkinReporterClose(t *testing.T) {
	// 创建mock HTTP服务器
	var receivedSpans []ZipkinSpan
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 解析请求体
		decoder := json.NewDecoder(r.Body)
		var spans []ZipkinSpan
		if err := decoder.Decode(&spans); err != nil {
			t.Errorf("Failed to decode spans: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// 保存接收到的spans
		mu.Lock()
		receivedSpans = append(receivedSpans, spans...)
		mu.Unlock()

		// 返回成功响应
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 创建ZipkinReporter
	reporter := NewZipkinReporter(server.URL, "test-service", 100)

	// 添加span
	spanData := SpanData{
		TraceID:   "trace-close",
		SpanID:    "span-close",
		Operation: "operation-close",
		StartTime: time.Now(),
		Duration:  time.Second,
	}
	err := reporter.Report(spanData)
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}

	// 调用Close方法
	err = reporter.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// 验证span在close时被发送
	mu.Lock()
	if len(receivedSpans) != 1 {
		t.Errorf("Expected 1 received span after Close, got %d", len(receivedSpans))
	}
	mu.Unlock()

	// 验证多次调用Close是安全的
	err = reporter.Close()
	if err != nil {
		t.Errorf("Second Close failed: %v", err)
	}
}

// 测试错误处理
func TestZipkinReporterErrorHandling(t *testing.T) {
	// 创建一个返回错误的mock服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	// 创建ZipkinReporter，设置较大的batchSize避免立即flush
	reporter := NewZipkinReporter(server.URL, "test-service", 100)
	defer reporter.Close()

	// 创建测试用的SpanData
	spanData := SpanData{
		TraceID:   "trace-error",
		SpanID:    "span-error",
		Operation: "operation-error",
		StartTime: time.Now(),
		Duration:  time.Second,
	}

	// 由于batchSize大于1，这里的Report调用不会立即触发flush，因此不会返回错误
	err := reporter.Report(spanData)
	if err != nil {
		t.Errorf("Report should not return error immediately, got %v", err)
	}

	// 等待reportLoop尝试发送（应该会失败）
	time.Sleep(100 * time.Millisecond)

	// 验证Close方法仍然被调用
	err = reporter.Close()
	// 由于服务器返回错误，Close方法可能会返回错误，这是可以接受的
	// 我们只需要确保Close方法能够正常退出，不会导致panic
}
