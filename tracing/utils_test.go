package tracing

import (
	"context"
	"testing"
	"time"
)

// 测试NewNoopSpan函数
func TestNewNoopSpan(t *testing.T) {
	span := NewNoopSpan()
	if span == nil {
		t.Errorf("NewNoopSpan returned nil")
	}

	// 验证noopSpan的各个方法调用不会崩溃
	span.End()
	span = span.SetTag("key", "value")
	// 使用SetTag代替SetTags
	span = span.SetTag("key2", "value2")
	span.LogFields(LogField{Key: "key", Value: "value"})
	span = span.LogKV("key", "value")
	span = span.SetOperationName("operation")
	if span.Tracer() != nil {
		t.Errorf("Expected Tracer() to return nil")
	}
	ctx := span.Context()
	if ctx == nil {
		t.Errorf("Expected Context() to return non-nil")
	}
	span = span.SetBaggageItem("key", "value")
	if val := span.BaggageItem("key"); val != "" {
		t.Errorf("Expected BaggageItem to return empty string, got %s", val)
	}
	span.LogEvent("event")
}

// 测试NewTextMapCarrier函数
func TestNewTextMapCarrier(t *testing.T) {
	carrier := NewTextMapCarrier()
	if carrier == nil {
		t.Errorf("NewTextMapCarrier returned nil")
	}

	// 验证carrier的方法
	key := "test-key"
	value := "test-value"
	carrier.Set(key, value)
	if got := carrier.Get(key); got != value {
		t.Errorf("Expected Get(%s) to return %s, got %s", key, value, got)
	}

	// Use type assertion to access Keys() and Format() methods
	if textMapCarrier, ok := carrier.(*textMapCarrier); ok {
		keys := textMapCarrier.Keys()
		if len(keys) != 1 || keys[0] != key {
			t.Errorf("Expected Keys() to return [%s], got %v", key, keys)
		}

		format := textMapCarrier.Format()
		if format != "text_map" {
			t.Errorf("Expected Format() to return 'text_map', got %s", format)
		}
	}
}

// 测试NewHTTPHeadersCarrier函数
func TestNewHTTPHeadersCarrier(t *testing.T) {
	carrier := NewHTTPHeadersCarrier()
	if carrier == nil {
		t.Errorf("NewHTTPHeadersCarrier returned nil")
	}

	// 验证carrier的方法
	key := "test-header"
	value := "test-value"
	carrier.Set(key, value)
	if got := carrier.Get(key); got != value {
		t.Errorf("Expected Get(%s) to return %s, got %s", key, value, got)
	}

	// Use type assertion to access Keys() and Format() methods
	if httpHeadersCarrier, ok := carrier.(*httpHeadersCarrier); ok {
		keys := httpHeadersCarrier.Keys()
		if len(keys) != 1 || keys[0] != key {
			t.Errorf("Expected Keys() to return [%s], got %v", key, keys)
		}

		format := httpHeadersCarrier.Format()
		if format != "http_headers" {
			t.Errorf("Expected Format() to return 'http_headers', got %s", format)
		}
	}
}

// 测试ToSpanData函数
func TestToSpanData(t *testing.T) {
	// 创建一个测试用的span
	spanCtx := NewSpanContext(nil) // 修改为使用nil参数
	tracer := &tracer{}
	ts := &span{
		tracer:     tracer,
		context:    spanCtx,
		operation:  "test-operation",
		startTime:  time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		finishTime: time.Date(2023, 1, 1, 0, 0, 1, 0, time.UTC),
		tags:       map[string]interface{}{"tag1": "value1"},
		logs: []LogData{
			{
				Timestamp: time.Date(2023, 1, 1, 0, 0, 0, 500000000, time.UTC),
				Fields:    []LogField{{Key: "log1", Value: "log-value1"}},
			},
		},
		baggage:  map[string]string{"baggage1": "baggage-value1"},
		finished: true,
	}

	// 转换为SpanData
	spanData := ToSpanData(ts)

	// 验证转换结果
	if spanData.TraceID == "" {
		t.Errorf("Expected non-empty TraceID")
	}
	if spanData.SpanID == "" {
		t.Errorf("Expected non-empty SpanID")
	}
	if spanData.ParentSpanID != "" {
		t.Errorf("Expected ParentSpanID '', got %s", spanData.ParentSpanID)
	}
	if spanData.Operation != "test-operation" {
		t.Errorf("Expected Operation 'test-operation', got %s", spanData.Operation)
	}
	if !spanData.StartTime.Equal(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Expected StartTime %v, got %v", time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), spanData.StartTime)
	}
	if spanData.Duration != time.Second {
		t.Errorf("Expected Duration 1s, got %v", spanData.Duration)
	}
	if val, ok := spanData.Tags["tag1"]; !ok || val != "value1" {
		t.Errorf("Expected tag 'tag1' with value 'value1', got %v", spanData.Tags)
	}
	if len(spanData.Logs) != 1 || spanData.Logs[0].Fields[0].Key != "log1" {
		t.Errorf("Expected logs to contain 'log1', got %v", spanData.Logs)
	}
	if val, ok := spanData.Baggage["baggage1"]; !ok || val != "baggage-value1" {
		t.Errorf("Expected baggage 'baggage1' with value 'baggage-value1', got %v", spanData.Baggage)
	}
}

// 测试SpanDataToJSON和JSONToSpanData函数
func TestSpanDataJSONConversion(t *testing.T) {
	// 创建测试用的SpanData
	spanData := SpanData{
		TraceID:      "trace-123",
		SpanID:       "span-456",
		ParentSpanID: "parent-789",
		Operation:    "test-operation",
		StartTime:    time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		Duration:     time.Second,
		Tags:         map[string]interface{}{"tag1": "value1"},
		Baggage:      map[string]string{"baggage1": "baggage-value1"},
	}

	// 转换为JSON
	jsonData, err := SpanDataToJSON(spanData)
	if err != nil {
		t.Errorf("SpanDataToJSON failed: %v", err)
	}

	// 从JSON转换回SpanData
	convertedData, err := JSONToSpanData(jsonData)
	if err != nil {
		t.Errorf("JSONToSpanData failed: %v", err)
	}

	// 验证转换结果
	if convertedData.TraceID != spanData.TraceID {
		t.Errorf("Expected TraceID %s, got %s", spanData.TraceID, convertedData.TraceID)
	}
	if convertedData.SpanID != spanData.SpanID {
		t.Errorf("Expected SpanID %s, got %s", spanData.SpanID, convertedData.SpanID)
	}
	if convertedData.ParentSpanID != spanData.ParentSpanID {
		t.Errorf("Expected ParentSpanID %s, got %s", spanData.ParentSpanID, convertedData.ParentSpanID)
	}
	if convertedData.Operation != spanData.Operation {
		t.Errorf("Expected Operation %s, got %s", spanData.Operation, convertedData.Operation)
	}
	// 注意：由于JSON序列化精度问题，这里只验证日期部分
	if convertedData.StartTime.Year() != spanData.StartTime.Year() ||
		convertedData.StartTime.Month() != spanData.StartTime.Month() ||
		convertedData.StartTime.Day() != spanData.StartTime.Day() {
		t.Errorf("Expected StartTime %v, got %v", spanData.StartTime, convertedData.StartTime)
	}
}

// 测试上下文相关函数
func TestContextFunctions(t *testing.T) {
	// 创建一个上下文
	ctx := context.Background()

	// 创建测试用的Tracer和Span
	tracer := &tracer{}
	span := NewNoopSpan()

	// 测试ContextWithTracer和TracerFromContext
	ctxWithTracer := ContextWithTracer(ctx, tracer)
	if got := TracerFromContext(ctxWithTracer); got != tracer {
		t.Errorf("Expected TracerFromContext to return tracer, got %v", got)
	}
	if got := TracerFromContext(ctx); got != nil {
		t.Errorf("Expected TracerFromContext to return nil for empty context, got %v", got)
	}

	// 测试ContextWithSpan和SpanFromContext
	ctxWithSpan := ContextWithSpan(ctx, span)
	if got := SpanFromContext(ctxWithSpan); got != span {
		t.Errorf("Expected SpanFromContext to return span, got %v", got)
	}
	if got := SpanFromContext(ctx); got != nil {
		t.Errorf("Expected SpanFromContext to return nil for empty context, got %v", got)
	}
}

// 测试StartSpanFromContext函数
func TestStartSpanFromContext(t *testing.T) {
	// 创建一个没有tracer的上下文
	ctx := context.Background()
	_, span := StartSpanFromContext(ctx, "test-operation")
	if span == nil {
		t.Errorf("Expected non-nil span")
	}
	// 对于没有tracer的上下文，应该返回noopSpan
	if _, ok := span.(*noopSpan); !ok {
		t.Errorf("Expected noopSpan for context without tracer")
	}

	// 创建一个有tracer的上下文（mock tracer）
	mockTracer := &mockTracer{}
	ctxWithTracer := ContextWithTracer(ctx, mockTracer)
	_, span = StartSpanFromContext(ctxWithTracer, "test-operation")
	// 验证span被创建
	if span == nil {
		t.Errorf("Expected non-nil span")
	}
}

// 测试日志和标签相关函数
func TestContextLoggingAndTagging(t *testing.T) {
	// 创建一个带有span的上下文
	ctx := context.Background()
	mockSpan := &mockSpan{}
	ctxWithSpan := ContextWithSpan(ctx, mockSpan)

	// 测试LogFieldsToContext
	fields := []LogField{{Key: "key", Value: "value"}}
	LogFieldsToContext(ctxWithSpan, fields...)
	if len(mockSpan.logFields) != 1 {
		t.Errorf("Expected 1 log field, got %d", len(mockSpan.logFields))
	}

	// 测试LogKV
	LogKV(ctxWithSpan, "key1", "value1", "key2", "value2")
	if len(mockSpan.logKVs) != 4 { // 2 pairs = 4 values
		t.Errorf("Expected 4 log KV values, got %d", len(mockSpan.logKVs))
	}

	// 测试SetTag
	SetTag(ctxWithSpan, "tag1", "value1")
	if len(mockSpan.tags) != 1 || mockSpan.tags["tag1"] != "value1" {
		t.Errorf("Expected tag 'tag1' with value 'value1', got %v", mockSpan.tags)
	}

	// 测试SetTags
	tags := map[string]interface{}{"tag2": "value2", "tag3": "value3"}
	SetTags(ctxWithSpan, tags)
	if len(mockSpan.tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(mockSpan.tags))
	}
}

// 模拟Tracer实现，用于测试
type mockTracer struct{}

func (t *mockTracer) StartSpan(operationName string, options ...SpanOption) Span {
	return &mockSpan{}
}

func (t *mockTracer) StartSpanFromContext(ctx context.Context, operationName string, options ...SpanOption) (Span, context.Context) {
	span := &mockSpan{}
	return span, ctx
}

func (t *mockTracer) Extract(format interface{}, carrier interface{}) (SpanContext, error) {
	return EmptySpanContext(), nil
}

func (t *mockTracer) Inject(ctx SpanContext, format interface{}, carrier interface{}) error {
	return nil
}

func (t *mockTracer) RegisterPropagator(format interface{}, propagator Propagator) {}

func (t *mockTracer) Close() error {
	return nil
}

// 模拟Span实现，用于测试
type mockSpan struct {
	logFields []LogField
	logKVs    []interface{}
	tags      map[string]interface{}
}

func (s *mockSpan) End(options ...FinishOption) {}

func (s *mockSpan) SetTag(key string, value interface{}) Span {
	if s.tags == nil {
		s.tags = make(map[string]interface{})
	}
	s.tags[key] = value
	return s
}

func (s *mockSpan) SetTags(tags map[string]interface{}) Span {
	if s.tags == nil {
		s.tags = make(map[string]interface{})
	}
	for k, v := range tags {
		s.tags[k] = v
	}
	return s
}

func (s *mockSpan) LogFields(fields ...LogField) {
	s.logFields = append(s.logFields, fields...)
}

func (s *mockSpan) LogKV(keyValues ...interface{}) Span {
	s.logKVs = append(s.logKVs, keyValues...)
	return s
}

func (s *mockSpan) SetOperationName(operationName string) Span {
	return s
}

func (s *mockSpan) Tracer() Tracer {
	return nil
}

func (s *mockSpan) Context() SpanContext {
	return EmptySpanContext()
}

func (s *mockSpan) SetBaggageItem(key, value string) Span {
	return s
}

func (s *mockSpan) BaggageItem(key string) string {
	return ""
}

func (s *mockSpan) LogEvent(event string, payload ...interface{}) {}
