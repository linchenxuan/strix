package tracing

import (
	"fmt"
	"sync"
	"time"
)

// span is the default implementation of the Span interface
// It provides thread-safe operations for capturing timing information,
// metadata (tags), structured logs, and baggage items for a unit of work
type span struct {
	mu         sync.RWMutex           // Mutex for thread-safe access to span fields
	tracer     *tracer                // Tracer that created this span
	context    SpanContext            // Span context containing trace identifiers
	operation  string                 // Name of the operation being tracked
	startTime  time.Time              // Time when the span was started
	finishTime time.Time              // Time when the span was finished
	tags       map[string]interface{} // Metadata tags associated with the span
	logs       []LogData              // Structured log entries recorded during the span's lifetime
	baggage    map[string]string      // Baggage items propagated across process boundaries
	finished   bool                   // Flag indicating if the span has been finished
}

// LogData represents a structured log entry recorded in a span
// Contains a timestamp and a collection of typed fields
type LogData struct {
	Timestamp time.Time  // When the log entry was recorded
	Fields    []LogField // Collection of key-value pairs with type information
}

// End completes the span and marks it as finished
// It records the finish time and prevents further modifications to the span
// Optional FinishOptions can specify a custom finish time
func (s *span) End(options ...FinishOption) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.finished {
		return
	}

	// Add default finishTime option
	defaultFinishOption := finishOptionFunc(func(opts *finishOptions) {
		if opts.finishTime.IsZero() {
			opts.finishTime = time.Now()
		}
	})
	options = append(options, defaultFinishOption)

	opts := &finishOptions{}
	for _, option := range options {
		option.apply(opts)
	}

	s.finishTime = opts.finishTime
	s.finished = true
}

// SetTag adds a key-value pair of metadata to the span
// Tags are used for filtering and grouping spans in the tracing backend
// Has no effect if the span has already been finished
func (s *span) SetTag(key string, value interface{}) Span {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.finished {
		return s
	}

	if s.tags == nil {
		s.tags = make(map[string]interface{})
	}
	s.tags[key] = value
	return s
}

// SetTags adds multiple metadata tags to the span
// This is a convenience method for adding several tags at once
// Has no effect if the span has already been finished
func (s *span) SetTags(tags map[string]interface{}) Span {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.finished {
		return s
	}

	if s.tags == nil {
		s.tags = make(map[string]interface{})
	}

	for k, v := range tags {
		s.tags[k] = v
	}
	return s
}

// LogFields records structured log data in the span
// Each LogField represents a typed key-value pair
// Has no effect if the span has already been finished
func (s *span) LogFields(fields ...LogField) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.finished {
		return
	}

	logData := LogData{
		Timestamp: time.Now(),
		Fields:    fields,
	}

	s.logs = append(s.logs, logData)
}

// LogKV records key-value pairs as structured log data
// This is a convenience method that automatically creates LogFields
// If there's an odd number of arguments, a placeholder value is added
func (s *span) LogKV(keyValues ...interface{}) Span {
	if len(keyValues)%2 != 0 {
		keyValues = append(keyValues, "<missing-value>")
	}

	fields := make([]LogField, 0, len(keyValues)/2)
	for i := 0; i < len(keyValues); i += 2 {
		key, ok := keyValues[i].(string)
		if !ok {
			key = fmt.Sprintf("%v", keyValues[i])
		}
		fields = append(fields, LogField{Key: key, Value: keyValues[i+1]})
	}

	s.LogFields(fields...)
	return s
}

// SetOperationName updates the name of the operation being tracked
// Can be used to rename a span after it's been created
// Has no effect if the span has already been finished
func (s *span) SetOperationName(name string) Span {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.finished {
		return s
	}

	s.operation = name
	return s
}

// Tracer returns the tracer that created this span
// This allows spans to create child spans using the same tracer
func (s *span) Tracer() Tracer {
	return s.tracer
}

// Context returns the span's context, which contains trace identifiers
// The returned context is safe to propagate across process boundaries
func (s *span) Context() SpanContext {
	return s.context
}

// SetBaggageItem stores a key-value pair in the span's baggage
// Baggage items are propagated across process boundaries with the trace context
// Has no effect if the span has already been finished
func (s *span) SetBaggageItem(key, value string) Span {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.finished {
		return s
	}

	// Set baggage for the span itself
	if s.baggage == nil {
		s.baggage = make(map[string]string)
	}
	s.baggage[key] = value

	// Also set baggage in span.context to ensure baggage items are properly propagated
	s.context.SetBaggageItem(key, value)
	return s
}

// BaggageItem retrieves a value from the span's baggage by key
// Returns empty string if the key is not found
func (s *span) BaggageItem(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.baggage[key]
}

// isFinished returns true if the span has been marked as finished
// This is an internal method used by the tracer implementation
func (s *span) isFinished() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.finished
}

// GetDuration returns the duration of the span
// Returns 0 if the span has not been finished
func (s *span) GetDuration() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.finished {
		return 0
	}

	return s.finishTime.Sub(s.startTime)
}

// GetTags returns a copy of the span's metadata tags
// This ensures the original tags map cannot be modified externally
func (s *span) GetTags() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tags := make(map[string]interface{})
	for k, v := range s.tags {
		tags[k] = v
	}
	return tags
}

// GetLogs returns a deep copy of the span's structured log entries
// This ensures the original log data cannot be modified externally
func (s *span) GetLogs() []LogData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	logs := make([]LogData, len(s.logs))
	for i, log := range s.logs {
		// Create a deep copy of LogData
		logData := LogData{
			Timestamp: log.Timestamp,
			Fields:    make([]LogField, len(log.Fields)),
		}
		// Copy Fields
		copy(logData.Fields, log.Fields)
		logs[i] = logData
	}
	return logs
}

// GetBaggage returns a copy of the span's baggage items
// This ensures the original baggage map cannot be modified externally
func (s *span) GetBaggage() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	baggage := make(map[string]string)
	for k, v := range s.baggage {
		baggage[k] = v
	}
	return baggage
}

// LogEvent records a named event with an optional payload
// This is a convenience method for recording significant moments in the span's lifecycle
// Has no effect if the span has already been finished
func (s *span) LogEvent(event string, payload ...interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.finished {
		return
	}

	fields := []LogField{LogField{Key: "event", Value: event}}
	if len(payload) > 0 {
		fields = append(fields, LogField{Key: "payload", Value: payload})
	}

	logData := LogData{
		Timestamp: time.Now(),
		Fields:    fields,
	}
	s.logs = append(s.logs, logData)
}
