package tracing

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// spanContext is the default implementation of the SpanContext interface
// It contains trace identifiers and baggage items for propagating context across process boundaries
type spanContext struct {
	traceID      string            // Unique identifier for the entire trace
	spanID       string            // Unique identifier for this specific span
	parentSpanID string            // Identifier of the parent span (empty for root spans)
	baggage      map[string]string // Key-value pairs propagated across process boundaries
	mu           sync.RWMutex      // Mutex for thread-safe access to baggage
}

// NewSpanContext creates a new span context
// If a parent context is provided, it inherits the trace ID and baggage
// Generates a new span ID regardless of whether a parent is provided
func NewSpanContext(parent SpanContext) SpanContext {
	ctx := &spanContext{
		traceID: generateTraceID(),
		spanID:  generateSpanID(),
		baggage: make(map[string]string),
	}

	// Inherit from parent if available
	if parent != nil {
		ctx.traceID = parent.TraceID()
		ctx.parentSpanID = parent.SpanID()

		// Copy baggage
		parent.ForeachBaggageItem(func(key, value string) bool {
			ctx.baggage[key] = value
			return true
		})
	}

	return ctx
}

// TraceID returns the unique identifier for the entire trace
// All spans in the same trace share the same trace ID
func (c *spanContext) TraceID() string {
	return c.traceID
}

// SpanID returns the unique identifier for this specific span
// Each individual span has its own unique span ID
func (c *spanContext) SpanID() string {
	return c.spanID
}

// ParentSpanID returns the identifier of this span's parent span
// Returns empty string if this is a root span
func (c *spanContext) ParentSpanID() string {
	return c.parentSpanID
}

// IsValid checks if the span context contains valid trace information
// A valid context must have both trace ID and span ID
func (c *spanContext) IsValid() bool {
	return c.traceID != "" && c.spanID != ""
}

// ForeachBaggageItem iterates over all baggage items
// Calls the provided handler function for each key-value pair
// The iteration stops if the handler returns false
func (c *spanContext) ForeachBaggageItem(handler func(key, value string) bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for k, v := range c.baggage {
		if !handler(k, v) {
			break
		}
	}
}

// SetBaggageItem stores a key-value pair in the baggage
// Baggage items are propagated across process boundaries with the trace context
func (c *spanContext) SetBaggageItem(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.baggage[key] = value
}

// GetBaggageItem retrieves a value from the baggage by key
// Returns empty string if the key is not found
func (c *spanContext) GetBaggageItem(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.baggage[key]
}

// String returns a string representation of the span context
// Format: "TraceID: {traceID}, SpanID: {spanID}, ParentSpanID: {parentSpanID}"
func (c *spanContext) String() string {
	return fmt.Sprintf("TraceID: %s, SpanID: %s, ParentSpanID: %s", c.traceID, c.spanID, c.parentSpanID)
}

// generateTraceID generates a new trace ID using UUID v4 format
// Falls back to a timestamp-based ID if random generation fails
func generateTraceID() string {
	// Generate 16 random bytes for UUID v4
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a simple ID if random generation fails
		return fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}

	// Set version (4) and variant bits for UUID v4
	bytes[6] = (bytes[6] & 0x0f) | 0x40 // Version 4
	bytes[8] = (bytes[8] & 0x3f) | 0x80 // Variant 10

	// Format as UUID string
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
}

// generateSpanID generates a new span ID using 8 random bytes
// Returns the ID as a hexadecimal string
// Falls back to a timestamp-based ID if random generation fails
func generateSpanID() string {
	// Generate 8 random bytes for span ID
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a simple ID if random generation fails
		return fmt.Sprintf("span-%d", time.Now().UnixNano())
	}

	// Return as hex string
	return hex.EncodeToString(bytes)
}

// parseTraceID parses a trace ID string and validates its format
// Accepts both UUID format (with dashes) and other string formats
func parseTraceID(traceID string) (string, error) {
	if traceID == "" {
		return "", fmt.Errorf("empty trace ID")
	}

	// Validate UUID format if it looks like one
	if strings.Contains(traceID, "-") {
		parts := strings.Split(traceID, "-")
		if len(parts) != 5 {
			return "", fmt.Errorf("invalid UUID format")
		}
	}

	return traceID, nil
}

// parseSpanID parses a span ID string and validates its format
// If the string is 16 characters long, it checks if it's a valid hexadecimal
func parseSpanID(spanID string) (string, error) {
	if spanID == "" {
		return "", fmt.Errorf("empty span ID")
	}

	// Validate hex format if it looks like one
	if len(spanID) == 16 { // 8 bytes in hex
		if _, err := hex.DecodeString(spanID); err != nil {
			return "", fmt.Errorf("invalid span ID format: %v", err)
		}
	}

	return spanID, nil
}

// EmptySpanContext returns a span context with empty identifiers
// Useful for creating root spans or indicating missing context
func EmptySpanContext() SpanContext {
	return &spanContext{
		traceID: "",
		spanID:  "",
		baggage: make(map[string]string),
	}
}

// SpanContextFromString creates a span context from its string representation
// Expects format: "TraceID: {traceID}, SpanID: {spanID}, ParentSpanID: {parentSpanID}"
// Returns an error if the format is invalid or if required fields are missing
func SpanContextFromString(str string) (SpanContext, error) {
	// Parse format: "TraceID: {traceID}, SpanID: {spanID}, ParentSpanID: {parentSpanID}"
	parts := strings.Split(str, ", ")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid span context format")
	}

	ctx := &spanContext{
		baggage: make(map[string]string),
	}

	for _, part := range parts {
		kv := strings.SplitN(part, ": ", 2)
		if len(kv) != 2 {
			continue
		}

		switch kv[0] {
		case "TraceID":
			ctx.traceID = kv[1]
		case "SpanID":
			ctx.spanID = kv[1]
		case "ParentSpanID":
			ctx.parentSpanID = kv[1]
		}
	}

	if !ctx.IsValid() {
		return nil, fmt.Errorf("invalid span context")
	}

	return ctx, nil
}
