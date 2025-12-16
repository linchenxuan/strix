package tracing

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// noopSpan is a no-op implementation of the Span interface
// All methods do nothing and return immediately
// Useful in testing or when tracing is disabled
type noopSpan struct{}

// NewNoopSpan creates a new span that doesn't perform any tracing operations
// Returns a span that implements the Span interface but ignores all operations
func NewNoopSpan() Span {
	return &noopSpan{}
}

// End implements the Span interface for noopSpan
// Does nothing
func (s *noopSpan) End(options ...FinishOption) {}

// SetTag implements the Span interface for noopSpan
// Does nothing and returns the same noopSpan
func (s *noopSpan) SetTag(key string, value interface{}) Span { return s }

// SetTags implements the Span interface for noopSpan
// Does nothing and returns the same noopSpan
func (s *noopSpan) SetTags(tags map[string]interface{}) Span { return s }

// LogFields implements the Span interface for noopSpan
// Does nothing
func (s *noopSpan) LogFields(fields ...LogField) {}

// LogKV implements the Span interface for noopSpan
// Does nothing and returns the same noopSpan
func (s *noopSpan) LogKV(keyValues ...interface{}) Span { return s }

// SetOperationName implements the Span interface for noopSpan
// Does nothing and returns the same noopSpan
func (s *noopSpan) SetOperationName(name string) Span { return s }

// Tracer implements the Span interface for noopSpan
// Returns nil
func (s *noopSpan) Tracer() Tracer { return nil }

// Context implements the Span interface for noopSpan
// Returns an empty span context
func (s *noopSpan) Context() SpanContext { return EmptySpanContext() }

// SetBaggageItem implements the Span interface for noopSpan
// Does nothing and returns the same noopSpan
func (s *noopSpan) SetBaggageItem(key, value string) Span { return s }

// BaggageItem implements the Span interface for noopSpan
// Always returns an empty string
func (s *noopSpan) BaggageItem(key string) string { return "" }

// LogEvent implements the Span interface for noopSpan
// Does nothing
func (s *noopSpan) LogEvent(event string, payload ...interface{}) {}

// httpHeadersCarrier is a thread-safe implementation of the Carrier interface
// Designed specifically for HTTP headers context propagation
type httpHeadersCarrier struct {
	headers map[string]string // HTTP headers storage
	mu      sync.RWMutex      // Mutex for thread-safe access
}

// NewHTTPHeadersCarrier creates a new thread-safe HTTP headers carrier
// Returns a Carrier implementation optimized for HTTP header propagation
func NewHTTPHeadersCarrier() Carrier {
	return &httpHeadersCarrier{
		headers: make(map[string]string),
	}
}

// Get retrieves a header value by key
// Returns the header value associated with the key, or empty string if not found
func (c *httpHeadersCarrier) Get(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.headers[key]
}

// Set stores a header key-value pair
// Overwrites any existing header with the same key
func (c *httpHeadersCarrier) Set(key string, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.headers[key] = value
}

// Keys returns all header keys present in the carrier
// Returns a slice containing all header keys in no particular order
func (c *httpHeadersCarrier) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.headers))
	for k := range c.headers {
		keys = append(keys, k)
	}
	return keys
}

// Format returns the carrier format identifier
// Returns "http_headers" for this implementation
func (c *httpHeadersCarrier) Format() string {
	return "http_headers"
}

// SpanData represents the serialized form of a completed span for reporting purposes
// It contains all essential information about a span in a format suitable for transmission
// Used by reporters to send span data to external tracing systems
type SpanData struct {
	TraceID      string                 `json:"trace_id"`                 // Unique identifier for the trace
	SpanID       string                 `json:"span_id"`                  // Unique identifier for this span
	ParentSpanID string                 `json:"parent_span_id,omitempty"` // Optional parent span ID
	Operation    string                 `json:"operation"`                // Name of the operation
	StartTime    time.Time              `json:"start_time"`               // Time when the span was started
	Duration     time.Duration          `json:"duration"`                 // Time taken for the span to complete
	Tags         map[string]interface{} `json:"tags,omitempty"`           // Metadata tags
	Logs         []LogData              `json:"logs,omitempty"`           // Structured log entries
	Baggage      map[string]string      `json:"baggage,omitempty"`        // Baggage items
}

// ToSpanData converts a span to its serialized SpanData representation
// Extracts all relevant information from the span for reporting
func ToSpanData(span *span) SpanData {
	return SpanData{
		TraceID:      span.context.TraceID(),
		SpanID:       span.context.SpanID(),
		ParentSpanID: span.context.ParentSpanID(),
		Operation:    span.operation,
		StartTime:    span.startTime,
		Duration:     span.GetDuration(),
		Tags:         span.GetTags(),
		Logs:         span.GetLogs(),
		Baggage:      span.GetBaggage(),
	}
}

// SpanDataToJSON converts span data to JSON format
// Useful for serialization when sending span data over the network
func SpanDataToJSON(data SpanData) ([]byte, error) {
	return json.Marshal(data)
}

// JSONToSpanData converts JSON data back to SpanData
// Useful for deserialization when receiving span data
func JSONToSpanData(data []byte) (SpanData, error) {
	var spanData SpanData
	err := json.Unmarshal(data, &spanData)
	return spanData, err
}

// TracerFromContext extracts a tracer from a context.Context
// Returns nil if no tracer is found in the context
func TracerFromContext(ctx context.Context) Tracer {
	if tracer, ok := ctx.Value(tracerKey).(Tracer); ok {
		return tracer
	}
	return nil
}

// ContextWithTracer adds a tracer to a context.Context
// Returns a new context with the tracer stored as a value
func ContextWithTracer(ctx context.Context, tracer Tracer) context.Context {
	return context.WithValue(ctx, tracerKey, tracer)
}

// SpanFromContext extracts the active span from a context.Context
// Returns nil if no active span is found in the context
func SpanFromContext(ctx context.Context) Span {
	if span, ok := ctx.Value(activeSpanKey).(Span); ok {
		return span
	}
	return nil
}

// ContextWithSpan adds a span to a context.Context
// Returns a new context with the span stored as the active span
func ContextWithSpan(ctx context.Context, span Span) context.Context {
	return context.WithValue(ctx, activeSpanKey, span)
}

// StartSpanFromContext starts a new span from a context
// If a tracer exists in the context, it uses that tracer to start the span
// Otherwise, returns a no-op span
func StartSpanFromContext(ctx context.Context, operationName string, opts ...SpanOption) (context.Context, Span) {
	if tracer := TracerFromContext(ctx); tracer != nil {
		span, ctx := tracer.StartSpanFromContext(ctx, operationName, opts...)
		return ctx, span
	}
	return ctx, NewNoopSpan()
}

// LogFieldsToContext logs structured fields to the active span in context
// Does nothing if no active span is found in the context
func LogFieldsToContext(ctx context.Context, fields ...LogField) {
	if span := SpanFromContext(ctx); span != nil {
		span.LogFields(fields...)
	}
}

// LogKV logs key-value pairs to the active span in context
// Does nothing if no active span is found in the context
func LogKV(ctx context.Context, keyValues ...interface{}) {
	if span := SpanFromContext(ctx); span != nil {
		span.LogKV(keyValues...)
	}
}

// SetTag sets a tag on the active span in context
// Does nothing if no active span is found in the context
func SetTag(ctx context.Context, key string, value interface{}) {
	if span := SpanFromContext(ctx); span != nil {
		span.SetTag(key, value)
	}
}

// SetTags sets multiple tags on the active span in context
// Does nothing if no active span is found in the context
func SetTags(ctx context.Context, tags map[string]interface{}) {
	if span := SpanFromContext(ctx); span != nil {
		for k, v := range tags {
			span.SetTag(k, v)
		}
	}
}

// contextKey is a private type used as a key for context values
// Prevents collisions with other packages using context values

type contextKey struct {
	name string
}

// context keys for storing tracer and active span in contexts
var (
	activeSpanKey = &contextKey{"active-span"} // Key for storing the active span
	tracerKey     = &contextKey{"tracer"}      // Key for storing the tracer
)
