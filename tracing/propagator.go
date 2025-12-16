package tracing

import (
	"encoding/base64"
	"strings"
)

// TextMapPropagator implements text map format context propagation
// Responsible for passing SpanContext information between different processes
type TextMapPropagator struct{}

// NewTextMapPropagator creates a new text map propagator
// Used for propagating SpanContext in text format carriers
func NewTextMapPropagator() Propagator {
	return &TextMapPropagator{}
}

// Inject injects SpanContext into text map carrier
// Sets key-value pairs according to OpenTracing standard format
func (p *TextMapPropagator) Inject(ctx SpanContext, carrier Carrier) error {
	if !ctx.IsValid() || carrier == nil {
		return nil
	}

	// Set TraceID and SpanID
	carrier.Set("trace-id", ctx.TraceID())
	carrier.Set("span-id", ctx.SpanID())

	// Inject baggage items
	ctx.ForeachBaggageItem(func(k, v string) bool {
		encodedKey := "baggage-" + k
		carrier.Set(encodedKey, v)
		return true
	})

	return nil
}

// Extract extracts SpanContext from text map carrier
// Parses key-value pairs in OpenTracing standard format
func (p *TextMapPropagator) Extract(carrier Carrier) (SpanContext, error) {
	if carrier == nil {
		return EmptySpanContext(), nil
	}

	// Extract TraceID and SpanID
	traceID := carrier.Get("trace-id")
	spanID := carrier.Get("span-id")

	// Return empty context if no valid TraceID and SpanID found
	if traceID == "" || spanID == "" {
		return EmptySpanContext(), nil
	}

	// Create span context and initialize baggage map
	ctx := &spanContext{
		traceID: traceID,
		spanID:  spanID,
		baggage: make(map[string]string), // Explicitly initialize baggage map
	}

	// Extract baggage items
	// Note: Since Carrier interface doesn't have Keys() method, we need to check common baggage keys
	commonBaggageKeys := []string{"baggage-userid", "baggage-requestid", "baggage-sessionid"}
	for _, key := range commonBaggageKeys {
		if value := carrier.Get(key); value != "" {
			// Remove prefix to get original key name
			baggageKey := key[len("baggage-"):]
			ctx.SetBaggageItem(baggageKey, value)
		}
	}

	return ctx, nil
}

// HTTPHeadersPropagator implements HTTP header format context propagation
// Optimized propagator implementation for HTTP environments
type HTTPHeadersPropagator struct{}

// NewHTTPHeadersPropagator creates a new HTTP header propagator
// Used for propagating SpanContext in HTTP request headers
func NewHTTPHeadersPropagator() Propagator {
	return &HTTPHeadersPropagator{}
}

// Inject injects SpanContext into HTTP header carrier
// Uses HTTP standard PascalCase naming format
func (p *HTTPHeadersPropagator) Inject(ctx SpanContext, carrier Carrier) error {
	if !ctx.IsValid() || carrier == nil {
		return nil
	}

	// Set standard HTTP header format
	carrier.Set("Trace-Id", ctx.TraceID())
	carrier.Set("Span-Id", ctx.SpanID())

	// Inject baggage items using Baggage- prefix
	ctx.ForeachBaggageItem(func(k, v string) bool {
		// Encode key name to ensure compliance with HTTP header specifications
		encodedKey := "Baggage-" + sanitizeHTTPHeaderKey(k)
		carrier.Set(encodedKey, v)
		return true
	})

	return nil
}

// Extract extracts SpanContext from HTTP header carrier
// Supports multiple common Trace ID HTTP header formats
func (p *HTTPHeadersPropagator) Extract(carrier Carrier) (SpanContext, error) {
	if carrier == nil {
		return EmptySpanContext(), nil
	}

	// Try to extract TraceID and SpanID from multiple common HTTP header formats
	traceID := ""
	spanID := ""

	// Check standard header format
	if t := carrier.Get("Trace-Id"); t != "" {
		traceID = t
	}
	if s := carrier.Get("Span-Id"); s != "" {
		spanID = s
	}

	// Check alternative formats
	if traceID == "" {
		traceID = carrier.Get("trace-id")
	}
	if spanID == "" {
		spanID = carrier.Get("span-id")
	}

	// Return empty context if no valid TraceID and SpanID found
	if traceID == "" || spanID == "" {
		return EmptySpanContext(), nil
	}

	// Create span context and initialize baggage map
	ctx := &spanContext{
		traceID: traceID,
		spanID:  spanID,
		baggage: make(map[string]string),
	}

	// Extract baggage items
	// Note: Since Carrier interface doesn't have Keys() method, we need to check common baggage keys
	commonBaggageKeys := []string{"baggage-userid", "baggage-requestid", "baggage-sessionid"}
	for _, key := range commonBaggageKeys {
		if value := carrier.Get(key); value != "" {
			// Remove prefix to get original key name
			baggageKey := key[len("baggage-"):]
			ctx.SetBaggageItem(baggageKey, value)
		}
	}

	// Also check uppercase versions for HTTP headers
	upperBaggageKeys := []string{"Baggage-Userid", "Baggage-Requestid", "Baggage-Sessionid"}
	for _, key := range upperBaggageKeys {
		if value := carrier.Get(key); value != "" {
			// Remove prefix to get original key name
			baggageKey := key[len("Baggage-"):]
			ctx.SetBaggageItem(baggageKey, value)
		}
	}

	return ctx, nil
}

// sanitizeHTTPHeaderKey sanitizes HTTP header key names to ensure compliance with specifications
// Removes or replaces characters that don't conform to HTTP header specifications
func sanitizeHTTPHeaderKey(key string) string {
	// Simple implementation: replace non-alphanumeric and non-hyphen characters with hyphens
	var result strings.Builder
	for _, c := range key {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
			result.WriteRune(c)
		} else {
			result.WriteRune('-')
		}
	}
	return result.String()
}

// BinaryPropagator implements binary format context propagation
// Suitable for scenarios requiring efficient binary transmission
type BinaryPropagator struct{}

// NewBinaryPropagator creates a new binary propagator
// Used for propagating SpanContext in binary format carriers
func NewBinaryPropagator() Propagator {
	return &BinaryPropagator{}
}

// Inject injects SpanContext into binary carrier
// Uses base64 encoding for binary transmission
func (p *BinaryPropagator) Inject(ctx SpanContext, carrier Carrier) error {
	// Binary propagator implementation example
	// In practical applications, more efficient binary encoding schemes should be used

	if !ctx.IsValid() || carrier == nil {
		return nil
	}

	// Simple implementation: use base64 encoding for text format
	// In practice, more efficient binary serialization should be used
	traceID := ctx.TraceID()
	spanID := ctx.SpanID()

	// Create a simple binary representation
	binaryData := []byte(traceID + ":" + spanID)
	encodedData := base64.StdEncoding.EncodeToString(binaryData)

	carrier.Set("binary-trace-context", encodedData)

	return nil
}

// Extract extracts SpanContext from binary carrier
// Parses base64 encoded binary data
func (p *BinaryPropagator) Extract(carrier Carrier) (SpanContext, error) {
	if carrier == nil {
		return EmptySpanContext(), nil
	}

	// Get and decode binary data
	encodedData := carrier.Get("binary-trace-context")
	if encodedData == "" {
		return EmptySpanContext(), nil
	}

	// Decode base64 data
	binaryData, err := base64.StdEncoding.DecodeString(encodedData)
	if err != nil {
		return EmptySpanContext(), err
	}

	// Parse format: traceID:spanID
	parts := strings.Split(string(binaryData), ":")
	if len(parts) < 2 {
		return EmptySpanContext(), nil
	}

	// Create and return SpanContext
	return &spanContext{
		traceID: parts[0],
		spanID:  parts[1],
	}, nil
}
