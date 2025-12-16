package tracing

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// 测试span结构体的基本功能
func TestSpanBasicFunctions(t *testing.T) {
	// 创建测试用的span
	spanCtx := NewSpanContext(nil) // 修改为使用nil参数
	tracer := &tracer{}
	s := &span{
		tracer:    tracer,
		context:   spanCtx,
		operation: "initial-operation",
		startTime: time.Now(),
		tags:      make(map[string]interface{}),
		logs:      make([]LogData, 0),
		baggage:   make(map[string]string),
		finished:  false,
	}

	// 测试Context方法
	ctx := s.Context()
	if ctx == nil {
		t.Errorf("Context() returned nil")
	}
	if ctx.TraceID() == "" {
		t.Errorf("Expected non-empty TraceID")
	}

	// 测试Tracer方法
	tracerFromSpan := s.Tracer()
	if tracerFromSpan != tracer {
		t.Errorf("Expected Tracer to return the original tracer")
	}

	// 测试SetOperationName方法
	s = s.SetOperationName("new-operation").(*span)
	if s.operation != "new-operation" {
		t.Errorf("Expected operation to be 'new-operation', got %s", s.operation)
	}

	// 测试SetTag方法
	s = s.SetTag("key1", "value1").(*span)
	if val, ok := s.tags["key1"]; !ok || val != "value1" {
		t.Errorf("Expected tag 'key1' with value 'value1', got %v", s.tags)
	}

	// 测试SetTags方法
	s = s.SetTags(map[string]interface{}{"key2": "value2", "key3": 3}).(*span)
	if val, ok := s.tags["key2"]; !ok || val != "value2" {
		t.Errorf("Expected tag 'key2' with value 'value2', got %v", s.tags)
	}
	if val, ok := s.tags["key3"]; !ok || val != 3 {
		t.Errorf("Expected tag 'key3' with value 3, got %v", s.tags)
	}

	// 测试SetBaggageItem方法
	s = s.SetBaggageItem("baggage1", "baggage-value1").(*span)
	if s.baggage["baggage1"] != "baggage-value1" {
		t.Errorf("Expected baggage 'baggage1' with value 'baggage-value1', got %v", s.baggage)
	}

	// 测试BaggageItem方法
	val := s.BaggageItem("baggage1")
	if val != "baggage-value1" {
		t.Errorf("Expected BaggageItem('baggage1') to return 'baggage-value1', got %s", val)
	}

	// 测试获取不存在的baggage项
	val = s.BaggageItem("non-existent")
	if val != "" {
		t.Errorf("Expected BaggageItem('non-existent') to return empty string, got %s", val)
	}
}

// 测试LogFields和LogKV方法
func TestSpanLogging(t *testing.T) {
	s := &span{
		logs:     make([]LogData, 0),
		finished: false,
	}

	// 测试LogFields方法
	s.LogFields(
		LogField{Key: "key1", Value: "value1"},
		LogField{Key: "key2", Value: "value2"},
	)
	if len(s.logs) != 1 {
		t.Errorf("Expected 1 log entry, got %d", len(s.logs))
	}
	if len(s.logs[0].Fields) != 2 {
		t.Errorf("Expected 2 log fields, got %d", len(s.logs[0].Fields))
	}

	// 测试LogKV方法
	s = s.LogKV("key3", "value3", "key4", 4).(*span)
	if len(s.logs) != 2 {
		t.Errorf("Expected 2 log entries, got %d", len(s.logs))
	}
	if len(s.logs[1].Fields) != 2 {
		t.Errorf("Expected 2 log fields from LogKV, got %d", len(s.logs[1].Fields))
	}

	// 测试奇数个KV参数
	s = s.LogKV("key5").(*span)
	if len(s.logs) != 3 {
		t.Errorf("Expected 3 log entries after odd KV, got %d", len(s.logs))
	}
	if len(s.logs[2].Fields) != 1 {
		t.Errorf("Expected 1 log field for odd KV, got %d", len(s.logs[2].Fields))
	}
	// 检查是否添加了缺失值占位符
	if field := s.logs[2].Fields[0]; field.Key != "key5" || field.Value != "<missing-value>" {
		t.Errorf("Expected missing value placeholder, got %v", field)
	}
}

// 测试LogEvent方法
func TestSpanLogEvent(t *testing.T) {
	s := &span{
		logs:     make([]LogData, 0),
		finished: false,
	}

	// 测试不带payload的LogEvent
	s.LogEvent("test-event")
	if len(s.logs) != 1 {
		t.Errorf("Expected 1 log entry, got %d", len(s.logs))
	}
	if len(s.logs[0].Fields) != 1 {
		t.Errorf("Expected 1 log field, got %d", len(s.logs[0].Fields))
	}
	if field := s.logs[0].Fields[0]; field.Key != "event" || field.Value != "test-event" {
		t.Errorf("Expected event field, got %v", field)
	}

	// 测试带payload的LogEvent
	s.LogEvent("test-event-with-payload", "payload-value")
	if len(s.logs) != 2 {
		t.Errorf("Expected 2 log entries, got %d", len(s.logs))
	}
	if len(s.logs[1].Fields) != 2 {
		t.Errorf("Expected 2 log fields for event with payload, got %d", len(s.logs[1].Fields))
	}
	if field := s.logs[1].Fields[0]; field.Key != "event" || field.Value != "test-event-with-payload" {
		t.Errorf("Expected event field, got %v", field)
	}
}

// 测试End方法和完成后的行为
func TestSpanEndAndFinishedBehavior(t *testing.T) {
	startTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	s := &span{
		startTime: startTime,
		finished:  false,
		tags:      make(map[string]interface{}),
		logs:      make([]LogData, 0),
		baggage:   make(map[string]string),
	}

	// 测试未完成时的行为
	if s.isFinished() {
		t.Errorf("Expected span to be unfinished")
	}
	if s.GetDuration() != 0 {
		t.Errorf("Expected duration 0 for unfinished span, got %v", s.GetDuration())
	}

	// 测试使用默认时间的End方法
	s.End()
	if !s.isFinished() {
		t.Errorf("Expected span to be finished after End()")
	}
	if s.finishTime.IsZero() {
		t.Errorf("Expected finishTime to be set after End()")
	}

	// 创建另一个span测试自定义结束时间
	finishTime := startTime.Add(time.Second)
	s2 := &span{
		startTime: startTime,
		finished:  false,
	}
	// 使用自定义结束时间
	finishOption := finishOptionFunc(func(opts *finishOptions) {
		opts.finishTime = finishTime
	})
	s2.End(finishOption)
	if !s2.finishTime.Equal(finishTime) {
		t.Errorf("Expected finishTime to be %v, got %v", finishTime, s2.finishTime)
	}
	if s2.GetDuration() != time.Second {
		t.Errorf("Expected duration 1s, got %v", s2.GetDuration())
	}

	// 测试完成后的方法调用是否无操作
	s = &span{finished: true}
	s.SetTag("key", "value")
	s.LogFields(LogField{Key: "key", Value: "value"})
	s.SetBaggageItem("key", "value")
	s.SetOperationName("new-operation")
	// 这些调用不应该产生任何效果，但也不应该崩溃
}

// 测试获取span数据的方法
func TestSpanGetters(t *testing.T) {
	// 创建一个带有各种数据的span
	startTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	finishTime := startTime.Add(time.Second)
	s := &span{
		startTime:  startTime,
		finishTime: finishTime,
		tags: map[string]interface{}{
			"tag1": "value1",
			"tag2": 2,
		},
		logs: []LogData{
			{
				Timestamp: startTime.Add(500 * time.Millisecond),
				Fields:    []LogField{{Key: "log1", Value: "log-value1"}},
			},
		},
		baggage: map[string]string{
			"baggage1": "baggage-value1",
		},
		finished: true,
	}

	// 测试GetDuration方法
	duration := s.GetDuration()
	if duration != time.Second {
		t.Errorf("Expected duration 1s, got %v", duration)
	}

	// 测试GetTags方法
	tags := s.GetTags()
	if len(tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(tags))
	}
	if val, ok := tags["tag1"]; !ok || val != "value1" {
		t.Errorf("Expected tag 'tag1' with value 'value1', got %v", tags)
	}
	// 验证返回的是副本
	tags["tag3"] = "value3"
	if _, ok := s.tags["tag3"]; ok {
		t.Errorf("Modifying returned tags should not affect original span")
	}

	// 测试GetLogs方法
	logs := s.GetLogs()
	if len(logs) != 1 {
		t.Errorf("Expected 1 log entry, got %d", len(logs))
	}
	if len(logs[0].Fields) != 1 {
		t.Errorf("Expected 1 log field, got %d", len(logs[0].Fields))
	}
	// 验证返回的是副本
	logs[0].Fields[0].Value = "modified-value"
	if s.logs[0].Fields[0].Value == "modified-value" {
		t.Errorf("Modifying returned logs should not affect original span")
	}

	// 测试GetBaggage方法
	baggage := s.GetBaggage()
	if len(baggage) != 1 {
		t.Errorf("Expected 1 baggage item, got %d", len(baggage))
	}
	if baggage["baggage1"] != "baggage-value1" {
		t.Errorf("Expected baggage 'baggage1' with value 'baggage-value1', got %v", baggage)
	}
	// 验证返回的是副本
	baggage["baggage2"] = "baggage-value2"
	if _, ok := s.baggage["baggage2"]; ok {
		t.Errorf("Modifying returned baggage should not affect original span")
	}
}

// 测试并发安全性（通过基本的并发访问测试）
func TestSpanConcurrentSafety(t *testing.T) {
	s := &span{
		context:  NewSpanContext(nil),
		tags:     make(map[string]interface{}),
		logs:     make([]LogData, 0),
		baggage:  make(map[string]string),
		finished: false,
	}

	// 创建多个goroutine同时访问span的方法
	const numGoroutines = 10
	const operationsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				// 执行各种操作
				s.SetTag(fmt.Sprintf("tag-%d-%d", id, j), j)
				s.LogKV(fmt.Sprintf("key-%d-%d", id, j), j)
				s.SetBaggageItem(fmt.Sprintf("baggage-%d-%d", id, j), fmt.Sprintf("value-%d-%d", id, j))
				s.BaggageItem(fmt.Sprintf("baggage-%d-%d", id, j))
				s.Context()
				s.Tracer()
			}
		}(i)
	}

	// 等待所有goroutine完成
	wg.Wait()

	// 验证span没有崩溃并可以正常结束
	s.End()
	if !s.isFinished() {
		t.Errorf("Expected span to be finished after concurrent operations")
	}
}
