package tracing

import (
	"context"
	"sync"
)

// Global tracer instance
// These variables manage the singleton tracer instance and propagators for the entire application
// globalTracer: The main tracer instance used for all tracing operations
// globalTracerMu: Read-write mutex to ensure thread-safe access to global tracer
// globalPropagator: Registry of propagators for different formats (text_map, http_headers, binary)
var (
	globalTracer     Tracer
	globalTracerMu   sync.RWMutex
	globalPropagator = make(map[string]Propagator)
)

// Register default propagators for different context propagation formats
// Propagators handle encoding/decoding of span context across process boundaries
// Supported formats:
//   - text_map: Generic text-based format for inter-process communication
//   - http_headers: HTTP header format for web-based communication
//   - binary: Binary format for high-performance scenarios
//
// Performance note: Registration is lightweight and happens during initialization
func registerDefaultPropagators() {
	// Text map format propagator for general-purpose context propagation
	RegisterGlobalPropagator("text_map", NewTextMapPropagator())
	// HTTP headers format propagator for web service integration
	RegisterGlobalPropagator("http_headers", NewHTTPHeadersPropagator())
	// Binary format propagator for high-performance scenarios
	RegisterGlobalPropagator("binary", NewBinaryPropagator())
}

// Set global tracer with proper synchronization and resource cleanup
// This function ensures thread-safe updates to the global tracer instance
// Parameters:
//   - tracer: The new tracer instance to set as global
//
// Resource management: Properly closes the old tracer to release resources
// Thread safety: Uses mutex to prevent race conditions during updates
func setGlobalTracer(tracer Tracer) {
	globalTracerMu.Lock()
	defer globalTracerMu.Unlock()

	// Close old tracer to release resources (file handles, network connections, etc.)
	if globalTracer != nil {
		globalTracer.Close()
	}

	globalTracer = tracer

	// Register all existing propagators with the new tracer
	// This ensures propagators remain available after tracer updates
	for format, propagator := range globalPropagator {
		if globalTracer != nil {
			globalTracer.RegisterPropagator(format, propagator)
		}
	}
}

// Get global tracer instance with read lock for thread safety
// Returns a noop tracer if no global tracer is configured
// Returns:
//   - Tracer: The current global tracer instance or noop tracer
//
// Thread safety: Uses read lock to allow concurrent reads
// Fallback mechanism: Returns noop tracer to prevent nil pointer dereferences
func GlobalTracer() Tracer {
	globalTracerMu.RLock()
	tracer := globalTracer
	globalTracerMu.RUnlock()

	// Return noop tracer if no global tracer is set to ensure safe usage
	if tracer == nil {
		return NewNoopTracer()
	}

	return tracer
}

// Close global tracer and release all associated resources
// This should be called during application shutdown to clean up resources
// Returns:
//   - error: Any error encountered during tracer closure
//
// Resource cleanup: Releases file handles, network connections, and memory
// Thread safety: Uses mutex to prevent race conditions during shutdown
func CloseGlobalTracer() error {
	globalTracerMu.Lock()
	defer globalTracerMu.Unlock()

	if globalTracer != nil {
		err := globalTracer.Close()
		globalTracer = nil
		return err
	}

	return nil
}

// Create global span with the specified operation name and options
// This is a convenience function for creating root spans without existing context
// Parameters:
//   - operationName: Descriptive name for the span operation
//   - options: Optional span configuration (tags, start time, etc.)
//
// Returns:
//   - Span: The created span instance
//   - context.Context: Context containing the span for propagation
//
// Usage: Typically used for top-level operations like request handling
func CreateGlobalSpan(operationName string, options ...SpanOption) (Span, context.Context) {
	tracer := GlobalTracer()
	span, ctx := tracer.StartSpanFromContext(context.Background(), operationName, options...)
	return span, ctx
}

// Create child span from existing context for distributed tracing
// This function creates a span that is a child of the span in the provided context
// Parameters:
//   - ctx: Parent context containing the parent span
//   - operationName: Descriptive name for the child span operation
//   - options: Optional span configuration
//
// Returns:
//   - Span: The created child span
//   - context.Context: New context containing the child span
//
// Distributed tracing: Maintains parent-child relationships across service boundaries
func CreateChildSpanFromContext(ctx context.Context, operationName string, options ...SpanOption) (Span, context.Context) {
	tracer := GlobalTracer()
	span, newCtx := tracer.StartSpanFromContext(ctx, operationName, options...)
	return span, newCtx
}

// Register global propagator for a specific format
// Propagators are used to encode/decode span context for cross-process communication
// Parameters:
//   - format: The format identifier (e.g., "text_map", "http_headers")
//   - propagator: The propagator implementation for the specified format
//
// Thread safety: Registration is immediately effective for the current global tracer
func RegisterGlobalPropagator(format string, propagator Propagator) {
	globalPropagator[format] = propagator

	// Immediately register to current global tracer for immediate availability
	tracer := GlobalTracer()
	tracer.RegisterPropagator(format, propagator)
}

// Extract span context from carrier using the specified format
// This is used to continue tracing across process boundaries
// Parameters:
//   - format: The format of the carrier data
//   - carrier: The carrier containing the encoded span context
//
// Returns:
//   - SpanContext: The extracted span context
//   - error: Extraction errors (invalid format, corrupted data, etc.)
//
// Cross-process tracing: Essential for distributed systems and microservices
func ExtractFromCarrier(format string, carrier Carrier) (SpanContext, error) {
	tracer := GlobalTracer()
	return tracer.Extract(format, carrier)
}

// Inject span context into carrier for cross-process propagation
// Parameters:
//   - ctx: The span context to inject
//   - format: The target format for injection
//   - carrier: The carrier to inject the context into
//
// Returns:
//   - error: Injection errors (unsupported format, etc.)
//
// Usage: Called before sending requests to downstream services
func InjectToCarrier(ctx SpanContext, format string, carrier Carrier) error {
	tracer := GlobalTracer()
	return tracer.Inject(ctx, format, carrier)
}

// Create a noop tracer that performs no operations
// Used when tracing is disabled or as a safe default return value
// Returns:
//   - Tracer: A no-operation tracer instance
//
// Performance: Noop tracer has minimal overhead for performance-critical scenarios
// Safety: Prevents nil pointer dereferences in tracing-disabled environments
func NewNoopTracer() Tracer {
	return &noopTracer{}
}

// noopTracer implements Tracer interface but performs no operations
// This is a null object pattern implementation for tracing-disabled scenarios
// Benefits:
//   - Eliminates conditional checks for tracing enablement
//   - Provides consistent API regardless of tracing configuration
//   - Minimal performance overhead
type noopTracer struct{}

// StartSpan creates a noop span with the specified operation name
// Parameters:
//   - operationName: The name of the span operation (ignored in noop implementation)
//   - options: Span options (ignored in noop implementation)
//
// Returns:
//   - Span: A noop span instance
//
// Performance: Returns lightweight noop span without allocation overhead
func (t *noopTracer) StartSpan(operationName string, options ...SpanOption) Span {
	return NewNoopSpan()
}

// StartSpanFromContext creates a noop span and adds it to the context
// Parameters:
//   - ctx: The parent context
//   - operationName: The name of the span operation
//   - options: Span options
//
// Returns:
//   - Span: A noop span instance
//   - context.Context: Context containing the noop span
//
// Context propagation: Maintains API consistency even when tracing is disabled
func (t *noopTracer) StartSpanFromContext(ctx context.Context, operationName string, options ...SpanOption) (Span, context.Context) {
	span := NewNoopSpan()
	return span, context.WithValue(ctx, spanContextKey{}, span.Context())
}

// Extract always returns an empty span context for noop tracer
// Parameters:
//   - format: The carrier format (ignored)
//   - carrier: The carrier data (ignored)
//
// Returns:
//   - SpanContext: Empty span context
//   - error: Always nil
//
// Cross-process tracing: Returns safe defaults when tracing is disabled
func (t *noopTracer) Extract(format interface{}, carrier interface{}) (SpanContext, error) {
	return EmptySpanContext(), nil
}

// Inject performs no operation for noop tracer
// Parameters:
//   - ctx: The span context to inject (ignored)
//   - format: The target format (ignored)
//   - carrier: The carrier to inject into (ignored)
//
// Returns:
//   - error: Always nil
//
// Performance: No operation minimizes overhead in tracing-disabled scenarios
func (t *noopTracer) Inject(ctx SpanContext, format interface{}, carrier interface{}) error {
	return nil
}

// RegisterPropagator performs no operation for noop tracer
// Parameters:
//   - format: The propagator format (ignored)
//   - propagator: The propagator instance (ignored)
//
// Design: Maintains interface compliance while disabling functionality
func (t *noopTracer) RegisterPropagator(format interface{}, propagator Propagator) {
}

// Close performs no operation for noop tracer
// Returns:
//   - error: Always nil
//
// Resource management: No resources to clean up in noop implementation
func (t *noopTracer) Close() error {
	return nil
}

// Context key type for storing span context in Go context
// This is an internal implementation detail for context propagation
type spanContextKey struct{}
