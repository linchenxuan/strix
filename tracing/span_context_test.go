package tracing

import (
	"strings"
	"testing"
)

// 测试NewSpanContext函数
func TestNewSpanContext(t *testing.T) {
	// 测试没有父上下文的情况
	ctx := NewSpanContext(nil).(*spanContext)

	// 验证生成了有效的traceID和spanID
	if ctx.traceID == "" {
		t.Errorf("Expected non-empty traceID")
	}
	if ctx.spanID == "" {
		t.Errorf("Expected non-empty spanID")
	}
	if ctx.parentSpanID != "" {
		t.Errorf("Expected empty parentSpanID for root span")
	}
	if len(ctx.baggage) != 0 {
		t.Errorf("Expected empty baggage for new span")
	}
}

// 测试从父上下文创建新上下文
func TestNewSpanContextWithParent(t *testing.T) {
	// 创建父上下文
	parent := &spanContext{
		traceID:      "parent-trace-id",
		spanID:       "parent-span-id",
		parentSpanID: "grandparent-span-id",
		baggage:      map[string]string{"key1": "value1"},
	}

	// 从父上下文创建新上下文
	child := NewSpanContext(parent).(*spanContext)

	// 验证继承了traceID
	if child.traceID != "parent-trace-id" {
		t.Errorf("Expected child to inherit traceID")
	}

	// 验证spanID是新生成的
	if child.spanID == "" || child.spanID == "parent-span-id" {
		t.Errorf("Expected child to have new spanID")
	}

	// 验证parentSpanID是父的spanID
	if child.parentSpanID != "parent-span-id" {
		t.Errorf("Expected child's parentSpanID to be parent's spanID")
	}

	// 验证继承了baggage
	if value, ok := child.baggage["key1"]; !ok || value != "value1" {
		t.Errorf("Expected child to inherit baggage")
	}

	// 验证修改子上下文的baggage不会影响父上下文
	child.baggage["key2"] = "value2"
	if _, ok := parent.baggage["key2"]; ok {
		t.Errorf("Modifying child's baggage should not affect parent's baggage")
	}
}

// 测试spanContext的方法
func TestSpanContextMethods(t *testing.T) {
	ctx := &spanContext{
		traceID:      "test-trace-id",
		spanID:       "test-span-id",
		parentSpanID: "test-parent-span-id",
		baggage:      make(map[string]string),
	}

	// 测试TraceID方法
	if ctx.TraceID() != "test-trace-id" {
		t.Errorf("TraceID method returned incorrect value")
	}

	// 测试SpanID方法
	if ctx.SpanID() != "test-span-id" {
		t.Errorf("SpanID method returned incorrect value")
	}

	// 测试ParentSpanID方法
	if ctx.ParentSpanID() != "test-parent-span-id" {
		t.Errorf("ParentSpanID method returned incorrect value")
	}

	// 测试IsValid方法
	if !ctx.IsValid() {
		t.Errorf("IsValid should return true for valid context")
	}

	// 测试无效上下文
	invalidCtx := &spanContext{}
	if invalidCtx.IsValid() {
		t.Errorf("IsValid should return false for invalid context")
	}

	// 测试SetBaggageItem和GetBaggageItem方法
	ctx.SetBaggageItem("key1", "value1")
	if value := ctx.GetBaggageItem("key1"); value != "value1" {
		t.Errorf("GetBaggageItem returned incorrect value")
	}

	// 测试ForeachBaggageItem方法
	count := 0
	ctx.ForeachBaggageItem(func(key, value string) bool {
		count++
		if key != "key1" || value != "value1" {
			t.Errorf("ForeachBaggageItem called with incorrect values")
		}
		return true
	})
	if count != 1 {
		t.Errorf("ForeachBaggageItem should be called once")
	}

	// 测试ForeachBaggageItem提前退出
	count = 0
	ctx.ForeachBaggageItem(func(key, value string) bool {
		count++
		return false // 提前退出
	})
	if count != 1 {
		t.Errorf("ForeachBaggageItem should stop after returning false")
	}

	// 测试String方法
	str := ctx.String()
	if !strings.Contains(str, "TraceID: test-trace-id") {
		t.Errorf("String representation does not contain expected traceID")
	}
	if !strings.Contains(str, "SpanID: test-span-id") {
		t.Errorf("String representation does not contain expected spanID")
	}
	if !strings.Contains(str, "ParentSpanID: test-parent-span-id") {
		t.Errorf("String representation does not contain expected parentSpanID")
	}
}

// 测试generateTraceID函数
func TestGenerateTraceID(t *testing.T) {
	// 生成多个traceID并验证它们的格式和唯一性
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		traceID := generateTraceID()

		// 验证traceID不为空
		if traceID == "" {
			t.Errorf("generateTraceID returned empty string")
		}

		// 验证traceID格式（UUID v4）
		parts := strings.Split(traceID, "-")
		if len(parts) != 5 {
			t.Errorf("TraceID does not have UUID format")
		}
		if len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
			t.Errorf("TraceID parts have incorrect length")
		}

		// 验证版本位（UUID v4的第13位应该是4）
		if parts[2][0] != '4' {
			t.Errorf("TraceID is not version 4 UUID")
		}

		// 验证唯一性
		if seen[traceID] {
			t.Errorf("Duplicate traceID generated: %s", traceID)
		}
		seen[traceID] = true
	}
}

// 测试generateSpanID函数
func TestGenerateSpanID(t *testing.T) {
	// 生成多个spanID并验证它们的格式和唯一性
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		spanID := generateSpanID()

		// 验证spanID不为空
		if spanID == "" {
			t.Errorf("generateSpanID returned empty string")
		}

		// 验证spanID格式（16个十六进制字符）
		if len(spanID) != 16 {
			t.Errorf("SpanID does not have correct length: %d", len(spanID))
		}

		// 验证唯一性
		if seen[spanID] {
			t.Errorf("Duplicate spanID generated: %s", spanID)
		}
		seen[spanID] = true
	}
}

// 测试parseTraceID函数
func TestParseTraceID(t *testing.T) {
	// 测试不包含'-'的有效输入
	traceID, err := parseTraceID("testtraceid")
	if err != nil {
		t.Errorf("parseTraceID failed for valid input: %v", err)
	}
	if traceID != "testtraceid" {
		t.Errorf("parseTraceID returned incorrect value")
	}

	// 测试UUID格式
	traceID, err = parseTraceID("123e4567-e89b-12d3-a456-426614174000")
	if err != nil {
		t.Errorf("parseTraceID failed for UUID format: %v", err)
	}
	if traceID != "123e4567-e89b-12d3-a456-426614174000" {
		t.Errorf("parseTraceID returned incorrect value for UUID format")
	}

	// 测试无效输入
	_, err = parseTraceID("")
	if err == nil {
		t.Errorf("parseTraceID should return error for empty input")
	}

	// 测试包含'-'但不是有效UUID格式的输入
	_, err = parseTraceID("test-trace-id")
	if err == nil {
		t.Errorf("parseTraceID should return error for input with '-' but not valid UUID format")
	}
}

// 测试parseSpanID函数
func TestParseSpanID(t *testing.T) {
	// 测试有效输入
	spanID, err := parseSpanID("test-span-id")
	if err != nil {
		t.Errorf("parseSpanID failed for valid input: %v", err)
	}
	if spanID != "test-span-id" {
		t.Errorf("parseSpanID returned incorrect value")
	}

	// 测试十六进制格式
	hexSpanID := "1234567890abcdef"
	spanID, err = parseSpanID(hexSpanID)
	if err != nil {
		t.Errorf("parseSpanID failed for hex format: %v", err)
	}
	if spanID != hexSpanID {
		t.Errorf("parseSpanID returned incorrect value for hex format")
	}

	// 测试无效输入
	_, err = parseSpanID("")
	if err == nil {
		t.Errorf("parseSpanID should return error for empty input")
	}

	// 测试无效的十六进制格式
	_, err = parseSpanID("1234567890abcdeg") // 'g'不是有效的十六进制字符
	if err == nil {
		t.Errorf("parseSpanID should return error for invalid hex format")
	}
}

// 测试EmptySpanContext函数
func TestEmptySpanContext(t *testing.T) {
	ctx := EmptySpanContext().(*spanContext)

	if ctx.traceID != "" {
		t.Errorf("Expected empty traceID")
	}
	if ctx.spanID != "" {
		t.Errorf("Expected empty spanID")
	}
	if ctx.parentSpanID != "" {
		t.Errorf("Expected empty parentSpanID")
	}
	if ctx.baggage == nil {
		t.Errorf("Expected non-nil baggage map")
	}
	if ctx.IsValid() {
		t.Errorf("Empty span context should not be valid")
	}
}

// 测试SpanContextFromString函数
func TestSpanContextFromString(t *testing.T) {
	// 测试有效格式
	str := "TraceID: test-trace-id, SpanID: test-span-id, ParentSpanID: test-parent-span-id"
	ctx, err := SpanContextFromString(str)
	if err != nil {
		t.Errorf("SpanContextFromString failed for valid format: %v", err)
	}

	if ctx.TraceID() != "test-trace-id" {
		t.Errorf("TraceID does not match expected value")
	}
	if ctx.SpanID() != "test-span-id" {
		t.Errorf("SpanID does not match expected value")
	}
	if ctx.ParentSpanID() != "test-parent-span-id" {
		t.Errorf("ParentSpanID does not match expected value")
	}

	// 测试最小有效格式（只有TraceID和SpanID）
	minimalStr := "TraceID: minimal-trace, SpanID: minimal-span"
	minimalCtx, err := SpanContextFromString(minimalStr)
	if err != nil {
		t.Errorf("SpanContextFromString failed for minimal format: %v", err)
	}

	if minimalCtx.TraceID() != "minimal-trace" {
		t.Errorf("TraceID does not match expected value for minimal format")
	}
	if minimalCtx.SpanID() != "minimal-span" {
		t.Errorf("SpanID does not match expected value for minimal format")
	}

	// 测试无效格式
	invalidStr := "invalid-format"
	_, err = SpanContextFromString(invalidStr)
	if err == nil {
		t.Errorf("SpanContextFromString should return error for invalid format")
	}

	// 测试无效的上下文（缺少必要字段）
	invalidCtxStr := "TraceID: , SpanID: invalid-span"
	_, err = SpanContextFromString(invalidCtxStr)
	if err == nil {
		t.Errorf("SpanContextFromString should return error for invalid context")
	}
}
