package tracing

import (
	"context"
	"sync"
	"time"
)

// Span represents a unit of work or operation in the tracing system
// It captures timing information, metadata (tags), structured logs, and relationships between operations
// Spans form the building blocks of traces, which are causal graphs of spans
type Span interface {
	// Context returns the span's context, which contains trace identifiers and baggage
	Context() SpanContext

	// End finishes the span and marks it as completed
	// Optional FinishOptions can specify custom finish time
	End(options ...FinishOption)

	// SetTag adds a key-value pair of metadata to the span
	// Tags are used for filtering and grouping spans in the tracing backend
	SetTag(key string, value interface{}) Span

	// SetOperationName updates the name of the operation being tracked
	SetOperationName(operationName string) Span

	// LogFields adds structured log data to the span
	// Each LogField represents a key-value pair with type information
	LogFields(fields ...LogField)

	// LogKV adds key-value log data to the span using variadic arguments
	// This is a convenience method for quick logging
	LogKV(keyValues ...interface{}) Span

	// SetBaggageItem stores a key-value pair in the span's baggage
	// Baggage is propagated across process boundaries with the trace context
	SetBaggageItem(key, value string) Span

	// BaggageItem retrieves a value from the span's baggage by key
	// Returns empty string if the key is not found
	BaggageItem(key string) string

	// Tracer returns the tracer that created this span
	Tracer() Tracer

	// LogEvent records a named event with optional payload
	// This is a convenience method for recording significant moments in the span's lifecycle
	LogEvent(event string, payload ...interface{})
}

// Tracer is the central interface for creating and managing spans
// It also handles context propagation across process boundaries
// Tracers are typically created once per application and reused
type Tracer interface {
	// StartSpan creates a new span with the specified operation name and options
	// Use SpanOptions to configure parent relationships, tags, and other properties
	StartSpan(operationName string, options ...SpanOption) Span

	// StartSpanFromContext creates a new span from a context.Context
	// If the context contains a span, it will be used as the parent
	// Returns both the new span and an updated context containing it
	StartSpanFromContext(ctx context.Context, operationName string, options ...SpanOption) (Span, context.Context)

	// Extract retrieves a SpanContext from a carrier
	// Used to continue a trace across process boundaries (e.g., when receiving an RPC call)
	Extract(format interface{}, carrier interface{}) (SpanContext, error)

	// Inject injects a SpanContext into a carrier
	// Used to propagate a trace context across process boundaries (e.g., when making an RPC call)
	Inject(ctx SpanContext, format interface{}, carrier interface{}) error

	// RegisterPropagator associates a propagator with a specific format
	// Allows custom context propagation mechanisms to be registered
	RegisterPropagator(format interface{}, propagator Propagator)

	// Close shuts down the tracer and releases any resources
	// Should be called when the application exits
	Close() error
}

// SpanContext contains the identifying information for a span within a trace
// It carries trace IDs, span IDs, and baggage items across process boundaries
// Unlike Span, SpanContext is immutable and safe to pass between processes
type SpanContext interface {
	// TraceID returns the unique identifier for the entire trace
	TraceID() string

	// SpanID returns the unique identifier for this specific span
	SpanID() string

	// ParentSpanID returns the identifier of this span's parent span
	// Returns empty string if this is a root span
	ParentSpanID() string

	// IsValid checks if the span context contains valid trace information
	IsValid() bool

	// ForeachBaggageItem iterates over all baggage items
	// The handler function returns false to stop iteration
	ForeachBaggageItem(handler func(key, value string) bool)

	// SetBaggageItem stores a key-value pair in the baggage
	SetBaggageItem(key, value string)

	// GetBaggageItem retrieves a value from the baggage by key
	// Returns empty string if the key is not found
	GetBaggageItem(key string) string
}

// Reporter is responsible for processing and sending completed spans to the backend
// Different implementations support various backends like console, file, or remote services
// Reporters are typically used by tracers to offload span processing
type Reporter interface {
	// Report processes a completed span's data
	// This method is called by the tracer when a span is finished
	Report(span SpanData) error

	// Close shuts down the reporter and flushes any buffered spans
	Close() error
}

// Sampler decides whether a particular trace should be sampled
// Used to control the volume of traces collected and reduce overhead
// Different sampling strategies balance observability and performance
type Sampler interface {
	// ShouldSample determines if a span should be sampled based on its context
	// Returns true if the span should be sampled
	ShouldSample(ctx SpanContext) bool
}

// Propagator handles the encoding and decoding of SpanContext to/from carriers
// Different propagators support different formats like HTTP headers or binary
// Used for cross-process context propagation
type Propagator interface {
	// Inject encodes a SpanContext into a carrier for propagation
	Inject(ctx SpanContext, carrier Carrier) error

	// Extract decodes a SpanContext from a carrier
	Extract(carrier Carrier) (SpanContext, error)
}

// Carrier is used to transport span context data across process boundaries
// Common implementations include HTTP headers, message headers, or custom protocols
type Carrier interface {
	// Get retrieves a value by key
	Get(key string) string

	// Set stores a key-value pair
	Set(key, value string)
}

// LogField represents a single field in a structured log entry
// Used to add typed metadata to span logs
type LogField struct {
	Key   string      // Name of the field
	Value interface{} // Value of the field
}

// FinishOption is an interface for configuring how a span is finished
// Implemented using the functional options pattern
type FinishOption interface {
	apply(*finishOptions)
}

// finishOptions contains internal options for finishing a span
// Not intended for direct use by users
type finishOptions struct {
	finishTime time.Time // Custom finish time for the span
}

// finishOptionFunc is a function that configures finishOptions
// Implements the FinishOption interface
type finishOptionFunc func(*finishOptions)

// apply implements the FinishOption interface for finishOptionFunc
func (f finishOptionFunc) apply(opts *finishOptions) {
	f(opts)
}

// TracerOption is an interface for configuring a tracer
// Implemented using the functional options pattern
type TracerOption interface {
	apply(*tracer)
}

// tracerOptionFunc is a function that configures a tracer
// Implements the TracerOption interface
type tracerOptionFunc func(*tracer)

// apply implements the TracerOption interface for tracerOptionFunc
func (f tracerOptionFunc) apply(t *tracer) {
	f(t)
}

// WithReporter configures a tracer with a specific reporter
// The reporter is responsible for processing completed spans
func WithReporter(reporter Reporter) TracerOption {
	return tracerOptionFunc(func(t *tracer) {
		t.reporter = reporter
	})
}

// WithSampler configures a tracer with a specific sampler
// The sampler determines which spans are sampled
func WithSampler(sampler Sampler) TracerOption {
	return tracerOptionFunc(func(t *tracer) {
		t.sampler = sampler
	})
}

// NewNoopReporter creates a reporter that does nothing
// Useful in tests or when tracing is disabled
func NewNoopReporter() Reporter {
	return &noopReporter{}
}

// noopReporter is a no-op implementation of the Reporter interface
// All methods return immediately without doing anything
type noopReporter struct{}

// tracer is an internal implementation of the Tracer interface
// It manages the lifecycle of spans and handles span reporting
type tracer struct {
	reporter       Reporter                   // Reporter for sending completed spans
	sampler        Sampler                    // Sampler for determining which spans to sample
	maxSpans       int                        // Maximum number of spans to keep in memory
	spanTimeout    time.Duration              // Maximum time a span can remain active
	reportInterval time.Duration              // How often to report batches of spans
	spanPool       *SpanPool                  // Pool for reusing span objects
	propagators    map[interface{}]Propagator // Map of format to propagator
	mu             sync.RWMutex               // Mutex for thread-safe access to propagators
}

// Report implements the Reporter interface for noopReporter
// Does nothing and returns nil
func (r *noopReporter) Report(span SpanData) error { return nil }

// Close implements the Reporter interface for noopReporter
// Does nothing and returns nil
func (r *noopReporter) Close() error { return nil }

// Close implements the Tracer interface
// Closes the associated reporter if it exists
func (t *tracer) Close() error {
	if t.reporter != nil {
		return t.reporter.Close()
	}
	return nil
}

// Extract implements the Tracer interface
// Extracts a SpanContext from the provided carrier using the registered propagator for the format
func (t *tracer) Extract(format interface{}, carrier interface{}) (SpanContext, error) {
	t.mu.RLock()
	propagator, ok := t.propagators[format]
	t.mu.RUnlock()

	if !ok {
		// If no propagator is registered for the format, return an empty span context
		return EmptySpanContext(), nil
	}

	// Ensure carrier implements the Carrier interface
	carrierImpl, ok := carrier.(Carrier)
	if !ok {
		return EmptySpanContext(), nil
	}

	return propagator.Extract(carrierImpl)
}

// Inject implements the Tracer interface
// Injects a SpanContext into the provided carrier using the registered propagator for the format
func (t *tracer) Inject(ctx SpanContext, format interface{}, carrier interface{}) error {
	t.mu.RLock()
	propagator, ok := t.propagators[format]
	t.mu.RUnlock()

	if !ok {
		// If no propagator is registered for the format, return no error
		return nil
	}

	// Ensure carrier implements the Carrier interface
	carrierImpl, ok := carrier.(Carrier)
	if !ok {
		return nil
	}

	return propagator.Inject(ctx, carrierImpl)
}

// RegisterPropagator implements the Tracer interface
// Registers a propagator for a specific format
func (t *tracer) RegisterPropagator(format interface{}, propagator Propagator) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.propagators[format] = propagator
}

// StartSpan implements the Tracer interface
// Creates a new span with the specified operation name and options
// Uses the tracer's span pool for object reuse
func (t *tracer) StartSpan(operationName string, options ...SpanOption) Span {
	// Process span options
	opts := &spanOptions{}
	for _, option := range options {
		option.apply(opts)
	}

	// Create a new span context
	ctx := NewSpanContext(opts.parent)

	// Get a span from the pool
	s := t.spanPool.Get()

	// Initialize the span
	s.tracer = t
	s.context = ctx
	s.operation = operationName
	s.startTime = time.Now()
	s.finished = false

	// Apply initial tags if any
	if len(opts.tags) > 0 {
		s.SetTags(opts.tags)
	}

	return s
}

// StartSpanFromContext implements the Tracer interface
// Creates a new span from a context.Context
// If the context contains a span, it will be used as the parent
// Returns both the new span and an updated context containing it
func (t *tracer) StartSpanFromContext(ctx context.Context, operationName string, options ...SpanOption) (Span, context.Context) {
	// Extract parent span from context
	parentSpan := SpanFromContext(ctx)
	if parentSpan != nil {
		// Add parent span option
		options = append(options, WithParent(parentSpan.Context()))
	}

	// Create new span
	span := t.StartSpan(operationName, options...)

	// Create new context with the span
	return span, ContextWithSpan(ctx, span)
}

// NewProbabilitySampler creates a sampler that samples spans based on a probability
// rate: Sampling rate between 0.0 (no spans) and 1.0 (all spans)
func NewProbabilitySampler(rate float64) Sampler {
	if rate <= 0 {
		return &neverSampler{} // Sample nothing
	}
	if rate >= 1 {
		return &alwaysSampler{} // Sample everything
	}
	return &probabilitySampler{rate: rate} // Sample based on probability
}

// neverSampler is a sampler that never samples any spans
type neverSampler struct{}

// ShouldSample implements the Sampler interface for neverSampler
// Always returns false
func (s *neverSampler) ShouldSample(ctx SpanContext) bool { return false }

// alwaysSampler is a sampler that always samples all spans

type alwaysSampler struct{}

// ShouldSample implements the Sampler interface for alwaysSampler
// Always returns true
func (s *alwaysSampler) ShouldSample(ctx SpanContext) bool { return true }

// probabilitySampler is a sampler that samples spans based on a probability rate
type probabilitySampler struct {
	rate float64 // Sampling probability between 0.0 and 1.0
}

// ShouldSample implements the Sampler interface for probabilitySampler
// Uses a simple hash of the trace ID to determine sampling
func (s *probabilitySampler) ShouldSample(ctx SpanContext) bool {
	// Simple implementation - in real code, use better hash
	traceID := ctx.TraceID()
	hash := 0
	for _, c := range traceID {
		hash = hash*31 + int(c)
	}
	return float64(hash%1000) < s.rate*1000
}

// WithMaxSpans configures the maximum number of spans a tracer can manage
func WithMaxSpans(maxSpans int) TracerOption {
	return tracerOptionFunc(func(t *tracer) {
		t.maxSpans = maxSpans
	})
}

// WithSpanTimeout configures the maximum time a span can remain active
func WithSpanTimeout(timeout time.Duration) TracerOption {
	return tracerOptionFunc(func(t *tracer) {
		t.spanTimeout = timeout
	})
}

// WithReportInterval configures how often the tracer reports batches of spans
func WithReportInterval(interval time.Duration) TracerOption {
	return tracerOptionFunc(func(t *tracer) {
		t.reportInterval = interval
	})
}

// WithFinishTime configures a custom finish time for a span
// By default, the current time is used when End() is called
func WithFinishTime(finishTime time.Time) FinishOption {
	return finishOptionFunc(func(opts *finishOptions) {
		opts.finishTime = finishTime
	})
}

// SpanOption is an interface for configuring span creation
// Implemented using the functional options pattern
type SpanOption interface {
	apply(*spanOptions)
}

// spanOptions contains internal options for creating a span
// Not intended for direct use by users
type spanOptions struct {
	parent     SpanContext            // Parent span context
	tags       map[string]interface{} // Initial tags for the span
	startTime  time.Time              // Custom start time
	references []SpanReference        // References to other spans
}

// spanOptionFunc is a function that configures spanOptions
// Implements the SpanOption interface
type spanOptionFunc func(*spanOptions)

// apply implements the SpanOption interface for spanOptionFunc
func (f spanOptionFunc) apply(opts *spanOptions) {
	f(opts)
}

// NewTracer creates a new tracer with the given options
// Creates a properly initialized tracer with default settings
// and applies all provided options
func NewTracer(opts ...TracerOption) Tracer {
	// Create a new tracer with default values
	t := &tracer{
		reporter:       NewNoopReporter(),
		sampler:        &alwaysSampler{},
		maxSpans:       1000,
		spanTimeout:    5 * time.Minute,
		reportInterval: 5 * time.Second,
		spanPool:       NewSpanPool(),
		propagators:    make(map[interface{}]Propagator),
	}

	// Apply all provided options
	for _, opt := range opts {
		opt.apply(t)
	}

	return t
}

// WithParent configures a span to be a child of another span
func WithParent(parent SpanContext) SpanOption {
	return spanOptionFunc(func(opts *spanOptions) {
		opts.parent = parent
	})
}

// WithTag configures a span with an initial tag
func WithTag(key string, value interface{}) SpanOption {
	return spanOptionFunc(func(opts *spanOptions) {
		if opts.tags == nil {
			opts.tags = make(map[string]interface{})
		}
		opts.tags[key] = value
	})
}

// WithStartTime configures a span with a custom start time
// By default, the current time is used when the span is created
func WithStartTime(startTime time.Time) SpanOption {
	return spanOptionFunc(func(opts *spanOptions) {
		opts.startTime = startTime
	})
}

// WithSpanReference configures a span with a reference to another span
// Can be used for more complex span relationships beyond parent-child
func WithSpanReference(ref SpanReference) SpanOption {
	return spanOptionFunc(func(opts *spanOptions) {
		opts.references = append(opts.references, ref)
	})
}

// SpanReference represents a reference from one span to another
// Used to establish relationships between spans beyond direct parent-child
type SpanReference struct {
	Type              SpanReferenceType // Type of reference (child-of, follows-from)
	ReferencedContext SpanContext       // Context of the referenced span
}

// SpanReferenceType represents the type of relationship between spans
// Different types indicate different causal relationships
type SpanReferenceType int

const (
	// SpanReferenceChildOf indicates a direct parent-child relationship
	// The child span is dependent on the parent span
	SpanReferenceChildOf SpanReferenceType = iota

	// SpanReferenceFollowsFrom indicates a causal but non-dependent relationship
	// The span follows from another span but isn't directly dependent on it completing
	SpanReferenceFollowsFrom
)
