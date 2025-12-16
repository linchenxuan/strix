package tracing

import (
	"fmt"
	"sync"
	"time"
)

// SpanPool provides object pooling for span objects to minimize garbage collection overhead
// It reuses span instances instead of creating new ones for each trace operation
// This is especially important for high-performance applications with frequent tracing
type SpanPool struct {
	pool sync.Pool // Underlying sync.Pool for thread-safe object reuse
}

// NewSpanPool creates a new span pool with default initialization
// The pool is pre-configured to create empty span objects with initialized maps
func NewSpanPool() *SpanPool {
	return &SpanPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &span{
					tags:    make(map[string]interface{}),
					logs:    make([]LogData, 0),
					baggage: make(map[string]string),
				}
			},
		},
	}
}

// Get retrieves a span object from the pool
// If the pool is empty, it will create a new span object using the New function
func (p *SpanPool) Get() *span {
	return p.pool.Get().(*span)
}

// Put returns a span object back to the pool after resetting its state
// This method carefully resets all span properties to ensure clean reuse
// Maps and slices are cleared but capacity is preserved for efficiency
func (p *SpanPool) Put(s *span) {
	// Reset span state while holding a lock for thread safety
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear core span properties
	s.tracer = nil
	s.context = nil
	s.operation = ""
	s.startTime = time.Time{}
	s.finishTime = time.Time{}

	// Clear maps and slices without reallocating
	for k := range s.tags {
		delete(s.tags, k)
	}
	s.logs = s.logs[:0] // Keep underlying array capacity
	for k := range s.baggage {
		delete(s.baggage, k)
	}
	s.finished = false

	// Return to the pool for reuse
	p.pool.Put(s)
}

// SpanContextPool provides object pooling for span context objects
// Reuses span context instances to reduce memory allocation and garbage collection pressure
type SpanContextPool struct {
	pool sync.Pool // Underlying sync.Pool for thread-safe object reuse
}

// NewSpanContextPool creates a new span context pool with default initialization
func NewSpanContextPool() *SpanContextPool {
	return &SpanContextPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &spanContext{
					baggage: make(map[string]string),
				}
			},
		},
	}
}

// Get retrieves a span context object from the pool
// Creates a new one if the pool is empty
func (p *SpanContextPool) Get() *spanContext {
	return p.pool.Get().(*spanContext)
}

// Put returns a span context object back to the pool after resetting its state
// Clears all identifiers and baggage data for safe reuse
func (p *SpanContextPool) Put(c *spanContext) {
	// Reset context identifiers
	c.traceID = ""
	c.spanID = ""
	c.parentSpanID = ""

	// Clear baggage map
	for k := range c.baggage {
		delete(c.baggage, k)
	}

	// Return to the pool for reuse
	p.pool.Put(c)
}

// LogFieldPool provides object pooling for log field slices
// Reuses slice instances to minimize memory allocation for log operations
type LogFieldPool struct {
	pool sync.Pool // Underlying sync.Pool for thread-safe object reuse
}

// NewLogFieldPool creates a new log field pool with default initialization
// Pre-allocates slice capacity for common logging scenarios (10 fields)
func NewLogFieldPool() *LogFieldPool {
	return &LogFieldPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make([]LogField, 0, 10) // Pre-allocate space for common log field count
			},
		},
	}
}

// Get retrieves a log field slice from the pool
// Creates a new slice with pre-allocated capacity if the pool is empty
func (p *LogFieldPool) Get() []LogField {
	return p.pool.Get().([]LogField)
}

// Put returns a log field slice back to the pool after clearing it
// Preserves underlying array capacity for future use
func (p *LogFieldPool) Put(fields []LogField) {
	// Clear slice by resetting length to 0 while preserving capacity
	fields = fields[:0]
	p.pool.Put(fields)
}

// TracerConfig holds configuration parameters for the tracing system
// This struct supports JSON and YAML unmarshalling for easy configuration loading
type TracerConfig struct {
	// ServiceName uniquely identifies the service in the tracing system
	ServiceName string `json:"service_name" yaml:"service_name"`

	// Enabled toggles tracing functionality on or off
	Enabled bool `json:"enabled" yaml:"enabled"`

	// SampleRate controls the percentage of traces to sample (0.0 to 1.0)
	// 0.0 = no traces, 1.0 = all traces
	SampleRate float64 `json:"sample_rate" yaml:"sample_rate"`

	// MaxSpans limits the maximum number of spans kept in memory
	MaxSpans int `json:"max_spans" yaml:"max_spans"`

	// SpanTimeout sets the maximum time a span can remain active
	SpanTimeout time.Duration `json:"span_timeout" yaml:"span_timeout"`

	// ReportInterval defines how frequently spans are reported to the backend
	ReportInterval time.Duration `json:"report_interval" yaml:"report_interval"`

	// ReporterType specifies which reporter implementation to use (e.g., "console", "zipkin")
	ReporterType string `json:"reporter_type" yaml:"reporter_type"`

	// ReporterConfig contains implementation-specific configuration for the reporter
	ReporterConfig map[string]interface{} `json:"reporter_config" yaml:"reporter_config"`

	// PropagationFormats lists the supported context propagation formats
	// Common formats include "text_map" and "http_headers"
	PropagationFormats []string `json:"propagation_formats" yaml:"propagation_formats"`
}

// DefaultTracerConfig returns a set of sensible default configuration values
// These defaults are designed to work well for most common use cases
func DefaultTracerConfig() TracerConfig {
	return TracerConfig{
		ServiceName:        "asura-service",
		Enabled:            true,
		SampleRate:         1.0,             // Sample all traces by default
		MaxSpans:           10000,           // Reasonable limit for typical applications
		SpanTimeout:        5 * time.Minute, // Spans longer than this are considered timed out
		ReportInterval:     1 * time.Second, // Frequent reporting for near real-time visibility
		ReporterType:       "console",       // Use console reporter by default for simplicity
		ReporterConfig:     make(map[string]interface{}),
		PropagationFormats: []string{"text_map", "http_headers"}, // Support common propagation formats
	}
}

// GetName returns the name of the configuration
// Implements the config.Config interface
func (c *TracerConfig) GetName() string {
	return "tracing"
}

// Validate checks that the tracer configuration contains valid values
// Returns an error if any configuration parameter is invalid
func (c *TracerConfig) Validate() error {
	if c.ServiceName == "" {
		return fmt.Errorf("service name cannot be empty")
	}

	if c.SampleRate < 0 || c.SampleRate > 1 {
		return fmt.Errorf("sample rate must be between 0 and 1")
	}

	if c.MaxSpans <= 0 {
		return fmt.Errorf("max spans must be positive")
	}

	if c.SpanTimeout <= 0 {
		return fmt.Errorf("span timeout must be positive")
	}

	if c.ReportInterval <= 0 {
		return fmt.Errorf("report interval must be positive")
	}

	if c.ReporterType == "" {
		return fmt.Errorf("reporter type cannot be empty")
	}

	return nil
}

// TracerBuilder builds tracers from configuration
// This builder pattern allows for flexible tracer initialization based on settings
type TracerBuilder struct {
	config TracerConfig // The configuration to use for building the tracer
}

// NewTracerBuilder creates a new tracer builder with the specified configuration
// The builder will use this configuration to create a properly initialized tracer
func NewTracerBuilder(config TracerConfig) *TracerBuilder {
	return &TracerBuilder{
		config: config,
	}
}

// Build builds a tracer from the configuration
// Validates the configuration, creates appropriate reporter and sampler,
// and assembles the tracer with all required options
func (b *TracerBuilder) Build() (Tracer, error) {
	// Validate configuration first
	if err := b.config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %v", err)
	}

	// Return no-op tracer if tracing is disabled
	if !b.config.Enabled {
		return NewTracer(WithReporter(NewNoopReporter())), nil
	}

	// Create reporter based on configuration
	reporter, err := b.buildReporter()
	if err != nil {
		return nil, fmt.Errorf("failed to build reporter: %v", err)
	}

	// Create sampler with the specified sample rate
	sampler := NewProbabilitySampler(b.config.SampleRate)

	// Build tracer options from configuration
	opts := []TracerOption{
		WithReporter(reporter),
		WithSampler(sampler),
		WithMaxSpans(b.config.MaxSpans),
		WithSpanTimeout(b.config.SpanTimeout),
		WithReportInterval(b.config.ReportInterval),
	}

	// Create and return the tracer
	return NewTracer(opts...), nil
}

// buildReporter creates a reporter based on the configured reporter type
// Handles type-specific configuration parameters for each reporter implementation
func (b *TracerBuilder) buildReporter() (Reporter, error) {
	switch b.config.ReporterType {
	case "console":
		// Console reporter has no specific configuration
		return NewConsoleReporter(nil), nil
	case "in_memory":
		// In-memory reporter for testing and debugging
		return NewInMemoryReporter(), nil
	case "http":
		// HTTP reporter requires endpoint configuration
		endpoint, ok := b.config.ReporterConfig["endpoint"].(string)
		if !ok {
			return nil, fmt.Errorf("missing endpoint for HTTP reporter")
		}

		// Extract optional headers configuration
		headers := make(map[string]string)
		if h, ok := b.config.ReporterConfig["headers"].(map[string]interface{}); ok {
			for k, v := range h {
				if s, ok := v.(string); ok {
					headers[k] = s
				}
			}
		}

		// Extract optional batch size (default to 100)
		batchSize := 100
		if bs, ok := b.config.ReporterConfig["batch_size"].(int); ok {
			batchSize = bs
		}

		return NewHTTPReporter(endpoint, headers, batchSize), nil
	case "zipkin":
		// Zipkin reporter requires endpoint configuration
		endpoint, ok := b.config.ReporterConfig["endpoint"].(string)
		if !ok {
			return nil, fmt.Errorf("missing endpoint for Zipkin reporter")
		}

		// Extract optional batch size (default to 100)
		batchSize := 100
		if bs, ok := b.config.ReporterConfig["batch_size"].(int); ok {
			batchSize = bs
		}

		return NewZipkinReporter(endpoint, b.config.ServiceName, batchSize), nil
	default:
		// Handle unsupported reporter types
		return nil, fmt.Errorf("unsupported reporter type: %s", b.config.ReporterType)
	}
}
