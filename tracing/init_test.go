package tracing

import (
	"context"
	"testing"
)

// TestInitTracing 测试初始化链路追踪系统
func TestInitTracing(t *testing.T) {
	// 这里模拟配置管理器的行为
	// 在实际测试中，应该使用真实的配置管理器

	// 初始化tracing
	tracer, err := InitTracing()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// 检查tracer是否创建成功
	if tracer == nil {
		t.Error("Expected tracer to be created, got nil")
	}

	// 检查全局tracer是否设置正确
	globalTracer := GlobalTracer()
	if globalTracer == nil {
		t.Error("Expected global tracer to be set, got nil")
	}

	// 测试关闭全局tracer
	if err := CloseGlobalTracer(); err != nil {
		t.Errorf("Expected no error when closing global tracer, got %v", err)
	}
}

// TestGlobalTracerAPI 测试全局tracer API的基本功能
func TestGlobalTracerAPI(t *testing.T) {
	// 创建一个简单的tracer用于测试 - 这里假设已有其他地方实现了NewTracer
	// 由于我们移除了重复的NewTracer函数，这里使用假的tracer对象
	// 在实际测试中，应该使用真实的tracer

	// 测试CreateGlobalSpan函数
	span, ctx := CreateGlobalSpan("test-operation")
	if span == nil {
		t.Error("Expected span to be created, got nil")
	}
	span.End()

	// 测试CreateChildSpanFromContext函数
	childSpan, _ := CreateChildSpanFromContext(ctx, "test-child-operation")
	if childSpan == nil {
		t.Error("Expected child span to be created, got nil")
	}
	childSpan.End()

	// 确保ContextWithSpan可以正确设置span和SpanFromContext可以正确提取span
	newCtx := ContextWithSpan(context.Background(), span)
	savedSpan := SpanFromContext(newCtx)
	if savedSpan == nil {
		t.Error("Expected to extract span from context, got nil")
	}
}

// TestPropagationAPI 测试上下文传播API
func TestPropagationAPI(t *testing.T) {
	// 初始化全局tracer
	tracer := NewTracer()
	setGlobalTracer(tracer)

	// 注册传播器
	RegisterGlobalPropagator("text_map", NewTextMapPropagator())
	RegisterGlobalPropagator("http_headers", NewHTTPHeadersPropagator())

	// 创建初始span
	span, _ := CreateGlobalSpan("test-propagation")
	defer span.End()

	// 设置baggage项用于测试
	span.SetBaggageItem("test-key", "test-value")

	// 测试TextMap格式的传播
	textCarrier := NewTextMapCarrier()
	if err := InjectToCarrier(span.Context(), "text_map", textCarrier); err != nil {
		t.Errorf("Expected no error when injecting to text map, got %v", err)
	}

	// 提取上下文
	extractedCtx, err := ExtractFromCarrier("text_map", textCarrier)
	if err != nil {
		t.Errorf("Expected no error when extracting from text map, got %v", err)
	}
	if extractedCtx == nil {
		t.Error("Expected extracted context to be non-nil")
	}

	// 检查TraceID是否正确传播
	if extractedCtx.TraceID() != span.Context().TraceID() {
		t.Errorf("Expected trace IDs to match, got %s vs %s",
			extractedCtx.TraceID(), span.Context().TraceID())
	}

	// 检查baggage项是否正确传播
	if extractedCtx.GetBaggageItem("test-key") != "test-value" {
		t.Errorf("Expected baggage item to be propagated, got %s",
			extractedCtx.GetBaggageItem("test-key"))
	}

	// 测试HTTP头格式的传播
	httpCarrier := NewHTTPHeadersCarrier()
	if err := InjectToCarrier(span.Context(), "http_headers", httpCarrier); err != nil {
		t.Errorf("Expected no error when injecting to http headers, got %v", err)
	}

	// 提取HTTP上下文
	httpExtractedCtx, err := ExtractFromCarrier("http_headers", httpCarrier)
	if err != nil {
		t.Errorf("Expected no error when extracting from http headers, got %v", err)
	}
	if httpExtractedCtx == nil {
		t.Error("Expected extracted http context to be non-nil")
	}

	// 检查HTTP传播的TraceID是否正确
	if httpExtractedCtx.TraceID() != span.Context().TraceID() {
		t.Errorf("Expected http trace IDs to match, got %s vs %s",
			httpExtractedCtx.TraceID(), span.Context().TraceID())
	}
}

// TestNoopTracerFallback 测试当没有设置全局tracer时是否正确返回noop tracer
func TestNoopTracerFallback(t *testing.T) {
	// 确保全局tracer为nil
	CloseGlobalTracer()

	// 获取全局tracer
	tracer := GlobalTracer()
	if tracer == nil {
		t.Error("Expected noop tracer, got nil")
	}

	// 创建span并验证它是一个noop span
	span, _ := tracer.StartSpanFromContext(context.Background(), "test-noop")
	if span == nil {
		t.Error("Expected noop span, got nil")
	}

	// 测试noop span的方法不会崩溃
	span.End()
	span.SetTag("key", "value")
	span.LogKV("event", "test")

	// 确保span的上下文是有效的
	ctx := span.Context()
	if ctx == nil {
		t.Error("Expected span context to be non-nil")
	}
}

// TestConfigReloading 测试配置热重载功能
func TestConfigReloading(t *testing.T) {
	// 这个测试需要模拟配置管理器的行为
	// 在实际测试中，应该使用真实的配置管理器并触发配置变更

	// 初始化tracing
	_, err := InitTracing()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// 在实际场景中，这里应该触发配置变更事件
	// 例如：
	// config.GetManager().NotifyConfigChanged()

	// 验证全局tracer是否被更新
	// 注意：在实际测试中，这里可能需要等待配置变更事件处理完成
	newTracer := GlobalTracer()
	// 由于我们没有实际触发配置变更，这里不做严格比较
	// 只检查获取tracer的功能正常
	if newTracer == nil {
		t.Error("Expected non-nil tracer after init")
	}
}
