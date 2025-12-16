package tracing

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

// 测试ConsoleReporter
func TestConsoleReporter(t *testing.T) {
	// 创建一个缓冲区来捕获日志输出
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	// 创建ConsoleReporter
	reporter := NewConsoleReporter(logger).(*ConsoleReporter)

	// 创建测试用的SpanData
	spanData := SpanData{
		TraceID:      "test-trace-id",
		SpanID:       "test-span-id",
		ParentSpanID: "test-parent-span-id",
		Operation:    "test-operation",
		StartTime:    time.Now().Add(-time.Second),
		Duration:     time.Second,
		Tags:         map[string]interface{}{"tag1": "value1"},
		Logs: []LogData{
			{
				Timestamp: time.Now(),
				Fields:    []LogField{{Key: "log1", Value: "log-value1"}},
			},
		},
		Baggage: map[string]string{"baggage1": "baggage-value1"},
	}

	// 测试Report方法
	err := reporter.Report(spanData)
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}

	// 验证日志输出包含预期内容
	output := buf.String()
	if !strings.Contains(output, "=== Span Report ===") {
		t.Errorf("Output does not contain expected header")
	}
	if !strings.Contains(output, "TraceID: test-trace-id") {
		t.Errorf("Output does not contain expected TraceID")
	}
	if !strings.Contains(output, "SpanID: test-span-id") {
		t.Errorf("Output does not contain expected SpanID")
	}
	if !strings.Contains(output, "ParentSpanID: test-parent-span-id") {
		t.Errorf("Output does not contain expected ParentSpanID")
	}
	if !strings.Contains(output, "Operation: test-operation") {
		t.Errorf("Output does not contain expected Operation")
	}
	if !strings.Contains(output, "tag1: value1") {
		t.Errorf("Output does not contain expected tag")
	}
	if !strings.Contains(output, "log1: log-value1") {
		t.Errorf("Output does not contain expected log")
	}
	if !strings.Contains(output, "baggage1: baggage-value1") {
		t.Errorf("Output does not contain expected baggage")
	}

	// 测试Close方法
	err = reporter.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// 测试InMemoryReporter
func TestInMemoryReporter(t *testing.T) {
	// 创建InMemoryReporter
	reporter := NewInMemoryReporter()

	// 创建测试用的SpanData
	spanData1 := SpanData{
		TraceID:   "trace-1",
		SpanID:    "span-1",
		Operation: "operation-1",
	}

	spanData2 := SpanData{
		TraceID:   "trace-2",
		SpanID:    "span-2",
		Operation: "operation-2",
	}

	// 测试Report方法
	err := reporter.Report(spanData1)
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}

	err = reporter.Report(spanData2)
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}

	// 测试GetSpans方法
	spans := reporter.GetSpans()
	if len(spans) != 2 {
		t.Errorf("Expected 2 spans, got %d", len(spans))
	}

	// 验证返回的spans是原始数据的副本
	if spans[0].TraceID != spanData1.TraceID || spans[0].SpanID != spanData1.SpanID {
		t.Errorf("Returned span does not match original")
	}

	// 测试GetSpanCount方法
	count := reporter.GetSpanCount()
	if count != 2 {
		t.Errorf("Expected span count 2, got %d", count)
	}

	// 测试Clear方法
	reporter.Clear()
	if reporter.GetSpanCount() != 0 {
		t.Errorf("Expected 0 spans after clear, got %d", reporter.GetSpanCount())
	}

	// 测试Close方法
	err = reporter.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// 模拟一个Reporter，用于测试BatchReporter
type mockReporter struct {
	reportedSpans []SpanData
	err           error
	mu            sync.Mutex
}

func newMockReporter() *mockReporter {
	return &mockReporter{
		reportedSpans: make([]SpanData, 0),
	}
}

func (m *mockReporter) Report(span SpanData) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return m.err
	}

	m.reportedSpans = append(m.reportedSpans, span)
	return nil
}

func (m *mockReporter) Close() error {
	return nil
}

func (m *mockReporter) GetReportedSpans() []SpanData {
	m.mu.Lock()
	defer m.mu.Unlock()

	spans := make([]SpanData, len(m.reportedSpans))
	copy(spans, m.reportedSpans)
	return spans
}

func (m *mockReporter) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.err = err
}

// 测试BatchReporter
func TestBatchReporter(t *testing.T) {
	// 创建mockReporter
	mock := newMockReporter()

	// 创建BatchReporter，设置较小的批处理大小和刷新间隔以便测试
	batchSize := 2
	flushInterval := 100 * time.Millisecond
	reporter := NewBatchReporter(mock, batchSize, flushInterval)

	// 创建测试用的SpanData
	spanData1 := SpanData{
		TraceID:   "trace-1",
		SpanID:    "span-1",
		Operation: "operation-1",
	}

	spanData2 := SpanData{
		TraceID:   "trace-2",
		SpanID:    "span-2",
		Operation: "operation-2",
	}

	spanData3 := SpanData{
		TraceID:   "trace-3",
		SpanID:    "span-3",
		Operation: "operation-3",
	}

	// 测试批次未满时的行为
	err := reporter.Report(spanData1)
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}

	// 此时应该还没有报告span，因为批次未满
	time.Sleep(50 * time.Millisecond) // 小于刷新间隔
	reportedSpans := mock.GetReportedSpans()
	if len(reportedSpans) != 0 {
		t.Errorf("Expected 0 reported spans, got %d", len(reportedSpans))
	}

	// 添加第二个span，这应该触发批量报告
	err = reporter.Report(spanData2)
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}

	// 等待报告完成
	time.Sleep(10 * time.Millisecond)
	reportedSpans = mock.GetReportedSpans()
	if len(reportedSpans) != 2 {
		t.Errorf("Expected 2 reported spans, got %d", len(reportedSpans))
	}

	// 添加第三个span，然后等待定时刷新
	err = reporter.Report(spanData3)
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}

	// 等待定时刷新
	time.Sleep(flushInterval + 10*time.Millisecond)
	reportedSpans = mock.GetReportedSpans()
	if len(reportedSpans) != 3 {
		t.Errorf("Expected 3 reported spans, got %d", len(reportedSpans))
	}

	// 测试Close方法
	err = reporter.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// 测试BatchReporter错误处理
func TestBatchReporterErrorHandling(t *testing.T) {
	// 创建mockReporter
	mock := newMockReporter()

	// 创建BatchReporter
	batchSize := 2
	flushInterval := 100 * time.Millisecond
	reporter := NewBatchReporter(mock, batchSize, flushInterval)

	// 设置mockReporter返回错误
	expectedErr := errors.New("report error")
	mock.SetError(expectedErr)

	// 创建测试用的SpanData
	spanData := SpanData{
		TraceID:   "trace-1",
		SpanID:    "span-1",
		Operation: "operation-1",
	}

	// 由于AsyncReporter直接调用reporter.Report，这里不会有错误返回
	err := reporter.Report(spanData)
	if err != nil {
		t.Errorf("Report should not return error, got %v", err)
	}

	// 测试Close方法
	err = reporter.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// 测试AsyncReporter
func TestAsyncReporter(t *testing.T) {
	// 创建mockReporter
	mock := newMockReporter()

	// 创建AsyncReporter
	reporter := NewAsyncReporter(mock, 10)

	// 创建测试用的SpanData
	spanData := SpanData{
		TraceID:   "trace-1",
		SpanID:    "span-1",
		Operation: "operation-1",
	}

	// 测试Report方法
	err := reporter.Report(spanData)
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}

	// 验证span被报告
	reportedSpans := mock.GetReportedSpans()
	if len(reportedSpans) != 1 {
		t.Errorf("Expected 1 reported span, got %d", len(reportedSpans))
	}

	if reportedSpans[0].TraceID != spanData.TraceID || reportedSpans[0].SpanID != spanData.SpanID {
		t.Errorf("Reported span does not match original")
	}

	// 测试Close方法
	err = reporter.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}
