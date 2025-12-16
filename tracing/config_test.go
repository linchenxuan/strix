package tracing

import (
	"testing"
	"time"
)

// 测试SpanPool
func TestSpanPool(t *testing.T) {
	// 创建SpanPool
	pool := NewSpanPool()

	// 从池中获取span
	span1 := pool.Get()

	// 验证获取的span不是nil
	if span1 == nil {
		t.Fatalf("Expected non-nil span")
	}

	// 验证span的字段已初始化
	if span1.tags == nil {
		t.Errorf("Expected tags map to be initialized")
	}
	if span1.logs == nil {
		t.Errorf("Expected logs slice to be initialized")
	}
	if span1.baggage == nil {
		t.Errorf("Expected baggage map to be initialized")
	}

	// 修改span的字段
	span1.context = EmptySpanContext()
	span1.operation = "test-operation"
	span1.startTime = time.Now()
	span1.finishTime = time.Now().Add(time.Second)
	span1.tags["tag1"] = "value1"
	span1.logs = append(span1.logs, LogData{Timestamp: time.Now(), Fields: []LogField{{Key: "log1", Value: "log-value1"}}})
	span1.baggage["baggage1"] = "baggage-value1"
	span1.finished = true

	// 将span放回池中
	pool.Put(span1)

	// 再次从池中获取span
	span2 := pool.Get()

	// 验证获取的span已被重置
	if span2.tracer != nil {
		t.Errorf("Expected tracer to be reset")
	}
	if span2.context != nil {
		t.Errorf("Expected context to be reset")
	}
	if span2.operation != "" {
		t.Errorf("Expected operation to be reset")
	}
	if !span2.startTime.IsZero() {
		t.Errorf("Expected startTime to be reset")
	}
	if !span2.finishTime.IsZero() {
		t.Errorf("Expected finishTime to be reset")
	}
	if len(span2.tags) != 0 {
		t.Errorf("Expected tags to be cleared, got %d tags", len(span2.tags))
	}
	if len(span2.logs) != 0 {
		t.Errorf("Expected logs to be cleared, got %d logs", len(span2.logs))
	}
	if len(span2.baggage) != 0 {
		t.Errorf("Expected baggage to be cleared, got %d items", len(span2.baggage))
	}
	if span2.finished {
		t.Errorf("Expected finished to be reset")
	}

	// 验证span1和span2是同一个对象（复用）
	if span1 != span2 {
		t.Errorf("Expected span to be reused")
	}
}

// 测试SpanContextPool
func TestSpanContextPool(t *testing.T) {
	// 创建SpanContextPool
	pool := NewSpanContextPool()

	// 从池中获取span context
	ctx1 := pool.Get()

	// 验证获取的context不是nil
	if ctx1 == nil {
		t.Fatalf("Expected non-nil span context")
	}

	// 验证context的字段已初始化
	if ctx1.baggage == nil {
		t.Errorf("Expected baggage map to be initialized")
	}

	// 修改context的字段
	ctx1.traceID = "test-trace-id"
	ctx1.spanID = "test-span-id"
	ctx1.parentSpanID = "test-parent-span-id"
	ctx1.baggage["baggage1"] = "baggage-value1"

	// 将context放回池中
	pool.Put(ctx1)

	// 再次从池中获取context
	ctx2 := pool.Get()

	// 验证获取的context已被重置
	if ctx2.traceID != "" {
		t.Errorf("Expected traceID to be reset")
	}
	if ctx2.spanID != "" {
		t.Errorf("Expected spanID to be reset")
	}
	if ctx2.parentSpanID != "" {
		t.Errorf("Expected parentSpanID to be reset")
	}
	if len(ctx2.baggage) != 0 {
		t.Errorf("Expected baggage to be cleared, got %d items", len(ctx2.baggage))
	}

	// 验证ctx1和ctx2是同一个对象（复用）
	if ctx1 != ctx2 {
		t.Errorf("Expected span context to be reused")
	}
}

// 测试LogFieldPool
func TestLogFieldPool(t *testing.T) {
	// 创建LogFieldPool
	pool := NewLogFieldPool()

	// 从池中获取log fields切片
	fields1 := pool.Get()

	// 验证获取的切片不是nil
	if fields1 == nil {
		t.Errorf("Expected non-nil log fields slice")
	}

	// 验证切片的容量
	if cap(fields1) < 10 {
		t.Errorf("Expected log fields slice to have capacity at least 10")
	}

	// 添加字段
	fields1 = append(fields1, LogField{Key: "key1", Value: "value1"})
	fields1 = append(fields1, LogField{Key: "key2", Value: "value2"})

	// 将切片放回池中
	pool.Put(fields1)

	// 再次从池中获取切片
	fields2 := pool.Get()

	// 验证获取的切片已被重置（长度为0）
	if len(fields2) != 0 {
		t.Errorf("Expected log fields slice to be cleared, got %d fields", len(fields2))
	}

	// 验证容量保持不变（复用底层数组）
	if cap(fields2) != cap(fields1) {
		t.Errorf("Expected log fields slice to have same capacity after reuse")
	}
}

// 测试DefaultTracerConfig
func TestDefaultTracerConfig(t *testing.T) {
	// 获取默认配置
	config := DefaultTracerConfig()

	// 验证默认配置的字段
	if config.ServiceName != "asura-service" {
		t.Errorf("Expected default service name to be 'asura-service'")
	}
	if !config.Enabled {
		t.Errorf("Expected tracing to be enabled by default")
	}
	if config.SampleRate != 1.0 {
		t.Errorf("Expected sample rate to be 1.0 by default")
	}
	if config.MaxSpans != 10000 {
		t.Errorf("Expected max spans to be 10000 by default")
	}
	if config.SpanTimeout != 5*time.Minute {
		t.Errorf("Expected span timeout to be 5 minutes by default")
	}
	if config.ReportInterval != 1*time.Second {
		t.Errorf("Expected report interval to be 1 second by default")
	}
	if config.ReporterType != "console" {
		t.Errorf("Expected reporter type to be 'console' by default")
	}
	if config.ReporterConfig == nil {
		t.Errorf("Expected reporter config to be initialized")
	}
	if len(config.PropagationFormats) != 2 || config.PropagationFormats[0] != "text_map" || config.PropagationFormats[1] != "http_headers" {
		t.Errorf("Expected propagation formats to include 'text_map' and 'http_headers'")
	}
}

// 测试TracerConfig的Validate方法
func TestTracerConfigValidate(t *testing.T) {
	// 定义测试用例
	testCases := []struct {
		name      string
		config    TracerConfig
		expectErr bool
	}{{
		name: "valid config",
		config: TracerConfig{
			ServiceName:        "test-service",
			Enabled:            true,
			SampleRate:         0.5,
			MaxSpans:           1000,
			SpanTimeout:        time.Minute,
			ReportInterval:     time.Second,
			ReporterType:       "console",
			ReporterConfig:     make(map[string]interface{}),
			PropagationFormats: []string{"text_map"},
		},
		expectErr: false,
	}, {
		name: "empty service name",
		config: TracerConfig{
			ServiceName:    "", // 无效
			Enabled:        true,
			SampleRate:     0.5,
			MaxSpans:       1000,
			SpanTimeout:    time.Minute,
			ReportInterval: time.Second,
			ReporterType:   "console",
		},
		expectErr: true,
	}, {
		name: "invalid sample rate (negative)",
		config: TracerConfig{
			ServiceName:    "test-service",
			Enabled:        true,
			SampleRate:     -0.1, // 无效
			MaxSpans:       1000,
			SpanTimeout:    time.Minute,
			ReportInterval: time.Second,
			ReporterType:   "console",
		},
		expectErr: true,
	}, {
		name: "invalid sample rate (greater than 1)",
		config: TracerConfig{
			ServiceName:    "test-service",
			Enabled:        true,
			SampleRate:     1.1, // 无效
			MaxSpans:       1000,
			SpanTimeout:    time.Minute,
			ReportInterval: time.Second,
			ReporterType:   "console",
		},
		expectErr: true,
	}, {
		name: "invalid max spans (zero)",
		config: TracerConfig{
			ServiceName:    "test-service",
			Enabled:        true,
			SampleRate:     0.5,
			MaxSpans:       0, // 无效
			SpanTimeout:    time.Minute,
			ReportInterval: time.Second,
			ReporterType:   "console",
		},
		expectErr: true,
	}, {
		name: "invalid span timeout (zero)",
		config: TracerConfig{
			ServiceName:    "test-service",
			Enabled:        true,
			SampleRate:     0.5,
			MaxSpans:       1000,
			SpanTimeout:    0, // 无效
			ReportInterval: time.Second,
			ReporterType:   "console",
		},
		expectErr: true,
	}, {
		name: "invalid report interval (zero)",
		config: TracerConfig{
			ServiceName:    "test-service",
			Enabled:        true,
			SampleRate:     0.5,
			MaxSpans:       1000,
			SpanTimeout:    time.Minute,
			ReportInterval: 0, // 无效
			ReporterType:   "console",
		},
		expectErr: true,
	}, {
		name: "empty reporter type",
		config: TracerConfig{
			ServiceName:    "test-service",
			Enabled:        true,
			SampleRate:     0.5,
			MaxSpans:       1000,
			SpanTimeout:    time.Minute,
			ReportInterval: time.Second,
			ReporterType:   "", // 无效
		},
		expectErr: true,
	}}

	// 运行测试用例
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()
			if (err != nil) != tc.expectErr {
				t.Errorf("Expected error: %v, got: %v", tc.expectErr, err)
			}
		})
	}
}

// 测试TracerBuilder
func TestTracerBuilder(t *testing.T) {
	// 测试构建启用的tracer
	config := DefaultTracerConfig()
	builder := NewTracerBuilder(config)
	_, err := builder.Build()
	if err != nil {
		t.Fatalf("Failed to build tracer: %v", err)
	}
	// Note: Currently, NewTracer returns nil as it's a placeholder implementation

	// 测试构建禁用的tracer
	disabledConfig := DefaultTracerConfig()
	disabledConfig.Enabled = false
	disabledBuilder := NewTracerBuilder(disabledConfig)
	_, err = disabledBuilder.Build()
	if err != nil {
		t.Fatalf("Failed to build disabled tracer: %v", err)
	}
	// Note: Currently, NewTracer returns nil as it's a placeholder implementation

	// 测试构建器返回错误当配置无效时
	invalidConfig := DefaultTracerConfig()
	invalidConfig.ServiceName = "" // 无效
	invalidBuilder := NewTracerBuilder(invalidConfig)
	_, err = invalidBuilder.Build()
	if err == nil {
		t.Errorf("Expected error when building tracer with invalid config")
	}
}

// 测试buildReporter方法支持的reporter类型
func TestBuildReporter(t *testing.T) {
	// 定义测试用例
	testCases := []struct {
		name         string
		reporterType string
		config       map[string]interface{}
		expectErr    bool
	}{{
		name:         "console reporter",
		reporterType: "console",
		config:       map[string]interface{}{},
		expectErr:    false,
	}, {
		name:         "in_memory reporter",
		reporterType: "in_memory",
		config:       map[string]interface{}{},
		expectErr:    false,
	}, {
		name:         "http reporter with valid config",
		reporterType: "http",
		config: map[string]interface{}{
			"endpoint": "http://example.com/api/traces",
		},
		expectErr: false,
	}, {
		name:         "http reporter with missing endpoint",
		reporterType: "http",
		config:       map[string]interface{}{},
		expectErr:    true,
	}, {
		name:         "http reporter with headers",
		reporterType: "http",
		config: map[string]interface{}{
			"endpoint": "http://example.com/api/traces",
			"headers": map[string]interface{}{
				"Authorization": "Bearer token",
				"Content-Type":  "application/json",
			},
		},
		expectErr: false,
	}, {
		name:         "http reporter with batch size",
		reporterType: "http",
		config: map[string]interface{}{
			"endpoint":   "http://example.com/api/traces",
			"batch_size": 50,
		},
		expectErr: false,
	}, {
		name:         "zipkin reporter with valid config",
		reporterType: "zipkin",
		config: map[string]interface{}{
			"endpoint": "http://zipkin.example.com/api/v2/spans",
		},
		expectErr: false,
	}, {
		name:         "zipkin reporter with missing endpoint",
		reporterType: "zipkin",
		config:       map[string]interface{}{},
		expectErr:    true,
	}, {
		name:         "zipkin reporter with batch size",
		reporterType: "zipkin",
		config: map[string]interface{}{
			"endpoint":   "http://zipkin.example.com/api/v2/spans",
			"batch_size": 50,
		},
		expectErr: false,
	}, {
		name:         "unsupported reporter type",
		reporterType: "unsupported",
		config:       map[string]interface{}{},
		expectErr:    true,
	}}

	// 运行测试用例
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := DefaultTracerConfig()
			config.ReporterType = tc.reporterType
			config.ReporterConfig = tc.config
			builder := NewTracerBuilder(config)
			_, err := builder.Build()
			if tc.expectErr {
				if err == nil {
					t.Errorf("Expected error for %s, but got nil", tc.name)
				}
			} else {
				if err != nil {
					t.Errorf("Failed to build %s: %v", tc.name, err)
				}
				// Note: Currently, NewTracer returns nil as it's a placeholder implementation
				// This will be fixed when the actual tracer implementation is added
			}
		})
	}
}
