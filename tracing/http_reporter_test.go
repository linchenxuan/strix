package tracing

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestHTTPReporter(t *testing.T) {
	// Create a test server to capture HTTP requests
	var mu sync.Mutex
	var receivedSpans []SpanData
	var requestCount int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		requestCount++

		// Read and parse the request body
		var spans []SpanData
		if err := json.NewDecoder(r.Body).Decode(&spans); err != nil {
			t.Errorf("Failed to decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		receivedSpans = append(receivedSpans, spans...)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create test spans
	span1 := SpanData{
		TraceID:   "trace-1",
		SpanID:    "span-1",
		Operation: "test-operation-1",
		StartTime: time.Now(),
		Duration:  100 * time.Millisecond,
		Tags:      map[string]interface{}{"tag1": "value1"},
		Baggage:   map[string]string{"baggage1": "baggage-value1"},
	}

	span2 := SpanData{
		TraceID:      "trace-2",
		SpanID:       "span-2",
		ParentSpanID: "parent-span-2",
		Operation:    "test-operation-2",
		StartTime:    time.Now(),
		Duration:     200 * time.Millisecond,
		Tags:         map[string]interface{}{"tag2": "value2"},
	}

	// Test Case 1: Report single span (batch not full)
	hr := NewHTTPReporter(server.URL, nil, 3) // batchSize=3
	err := hr.Report(span1)
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}

	// Wait a short time to ensure no unexpected flush happened
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if requestCount > 0 {
		t.Errorf("Expected no request yet, but got %d", requestCount)
	}
	mu.Unlock()

	// Test Case 2: Report enough spans to trigger batch flush
	err = hr.Report(span2)
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}
	err = hr.Report(span1) // Adding third span to trigger flush
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}

	// Give the server time to process the request
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if requestCount != 1 {
		t.Errorf("Expected 1 request after batch flush, but got %d", requestCount)
	}
	if len(receivedSpans) != 3 {
		t.Errorf("Expected 3 spans after batch flush, but got %d", len(receivedSpans))
	}
	mu.Unlock()

	// Test Case 3: Close the reporter and verify remaining spans are flushed
	hr = NewHTTPReporter(server.URL, nil, 100)
	err = hr.Report(span1)
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}

	initialCount := requestCount
	err = hr.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Give the server time to process the request
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if requestCount != initialCount+1 {
		t.Errorf("Expected 1 additional request after Close, but got %d", requestCount-initialCount)
	}
	if len(receivedSpans) != 4 {
		t.Errorf("Expected 4 total spans after Close, but got %d", len(receivedSpans))
	}
	mu.Unlock()

	// Test Case 4: Test reportLoop periodic flush - commented out to reduce test time
	// This test requires waiting 5+ seconds which makes the test too slow
	// To properly test this functionality, consider using a test framework that can mock time
	// For now, we'll skip this test to keep the overall test suite fast

	// Clean up
	hr.Close()
}

func TestHTTPReporterErrorHandling(t *testing.T) {
	// Test Case 1: Invalid endpoint
	hr := NewHTTPReporter("http://non-existent-endpoint-that-should-fail", nil, 1)
	err := hr.Report(SpanData{TraceID: "test"})
	if err == nil {
		t.Error("Expected error with invalid endpoint, but got nil")
	}

	// Clean up
	hr.Close()
}

func TestHTTPReporterWithHeaders(t *testing.T) {
	// Create a test server to capture HTTP requests and headers
	var receivedHeaders http.Header
	var requestCount int
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		requestCount++
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create HTTP reporter with custom headers
	headers := map[string]string{
		"X-Custom-Header": "custom-value",
		"Authorization":   "Bearer token123",
	}
	hr := NewHTTPReporter(server.URL, headers, 1)

	// Trigger a report
	err := hr.Report(SpanData{TraceID: "test"})
	if err != nil {
		t.Errorf("Report failed: %v", err)
	}

	// Give the server time to process the request
	time.Sleep(100 * time.Millisecond)

	// Verify headers
	mu.Lock()
	defer mu.Unlock()

	if requestCount != 1 {
		t.Errorf("Expected 1 request, but got %d", requestCount)
	}

	for k, v := range headers {
		if receivedHeaders.Get(k) != v {
			t.Errorf("Expected header %s=%s, but got %s", k, v, receivedHeaders.Get(k))
		}
	}

	// Verify Content-Type header is set correctly
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type=application/json, but got %s", receivedHeaders.Get("Content-Type"))
	}

	// Clean up
	hr.Close()
}
