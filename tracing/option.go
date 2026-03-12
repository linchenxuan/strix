package tracing

// SpanEndOption 是 Span.End() 的可选参数。
type SpanEndOption func(*spanEndConfig)

// SpanStartOption 是 Span.StartChild() 的可选参数（预留扩展）。
type SpanStartOption func(*spanStartConfig)

// spanEndConfig 收集 End 方法的选项参数。
type spanEndConfig struct {
	status StatusCode // 完成状态。
}

// spanStartConfig 收集 StartChild 方法的选项参数（预留扩展）。
type spanStartConfig struct{}

// WithStatusError 标记 Span 为错误状态。
func WithStatusError() SpanEndOption {
	return func(c *spanEndConfig) {
		c.status = StatusError
	}
}

// WithCode 设置 Span 的状态码，非零视为错误。
func WithCode(code int32) SpanEndOption {
	return func(c *spanEndConfig) {
		if code != 0 {
			c.status = StatusError
		}
	}
}

// WithErr 根据 error 设置 Span 状态。
func WithErr(err error) SpanEndOption {
	return func(c *spanEndConfig) {
		if err != nil {
			c.status = StatusError
		}
	}
}
