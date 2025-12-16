// Package tracing provides distributed tracing capabilities for applications
// It includes core interfaces for spans, tracers, and reporters, as well as
// implementations for HTTP-based reporting
package tracing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// HTTPReporter implements the Reporter interface to send span data to a remote
// HTTP endpoint. It supports batching of spans to minimize network overhead
// and provides both batch size and time-based flushing strategies.
// This reporter is thread-safe and can be used concurrently from multiple goroutines.
type HTTPReporter struct {
	endpoint  string            // The remote HTTP endpoint URL to receive span data
	headers   map[string]string // Custom HTTP headers to include in all requests
	client    *http.Client      // HTTP client for sending span data requests
	batchSize int               // Maximum number of spans to include in a single batch
	spans     []SpanData        // Internal buffer for spans waiting to be reported
	mu        sync.Mutex        // Mutex to protect concurrent access to spans and state
	stopCh    chan struct{}     // Channel to signal the report loop goroutine to stop
	wg        sync.WaitGroup    // WaitGroup to track and manage the report loop goroutine
	closed    bool              // Flag indicating whether the reporter has been closed
}

// NewHTTPReporter creates a new HTTP reporter with the specified configuration.
// It starts a background goroutine to periodically flush spans to the remote endpoint.
//
// Parameters:
//
//	endpoint: The URL of the remote HTTP endpoint that will receive span data
//	headers: Optional custom HTTP headers to include in each request
//	batchSize: Maximum number of spans to include in a single batch (default: 100)
//
// Returns:
//
//	A pointer to the newly created HTTPReporter instance
func NewHTTPReporter(endpoint string, headers map[string]string, batchSize int) *HTTPReporter {
	// Set default batch size if not provided or invalid
	if batchSize <= 0 {
		batchSize = 100
	}

	// Initialize headers map if nil
	if headers == nil {
		headers = make(map[string]string)
	}

	// Create and initialize the reporter instance
	hr := &HTTPReporter{
		endpoint:  endpoint,
		headers:   headers,
		client:    &http.Client{Timeout: 10 * time.Second}, // Configure with 10s timeout
		batchSize: batchSize,
		spans:     make([]SpanData, 0, batchSize), // Pre-allocate buffer capacity
		stopCh:    make(chan struct{}),
	}

	// Start the background report loop
	hr.wg.Add(1)
	go hr.reportLoop()

	return hr
}

// Report adds a span to the batch. If the batch reaches the configured size,
// it immediately flushes the batch to the remote endpoint.
// This method is thread-safe and can be called concurrently from multiple goroutines.
//
// Parameters:
//
//	span: The span data to be reported
//
// Returns:
//
//	An error if flushing the batch fails, otherwise nil
func (hr *HTTPReporter) Report(span SpanData) error {
	// Acquire lock to ensure thread-safe access to the span buffer
	hr.mu.Lock()
	defer hr.mu.Unlock()

	// Add the span to the internal buffer
	hr.spans = append(hr.spans, span)

	// If the buffer has reached maximum capacity, trigger a flush
	if len(hr.spans) >= hr.batchSize {
		return hr.flush()
	}

	return nil
}

// Close gracefully shuts down the reporter. It stops the background report loop,
// waits for any in-flight operations to complete, and flushes any remaining spans.
// This method is idempotent and can be called multiple times without causing errors.
//
// Returns:
//
//	An error if flushing the remaining spans fails, otherwise nil
func (hr *HTTPReporter) Close() error {
	// Check if the reporter is already closed
	hr.mu.Lock()
	if hr.closed {
		hr.mu.Unlock()
		return nil // Already closed, nothing to do
	}

	// Mark as closed to prevent concurrent access during shutdown
	hr.closed = true
	hr.mu.Unlock()

	// Signal the report loop to stop and wait for it to complete
	close(hr.stopCh)
	hr.wg.Wait()

	// Lock and flush any remaining spans in the buffer
	hr.mu.Lock()
	defer hr.mu.Unlock()

	return hr.flush()
}

// flush sends the current batch of spans to the remote HTTP endpoint. It marshals
// the spans to JSON, creates an HTTP POST request, sends it to the configured endpoint,
// and handles any errors that occur during this process.
// This method assumes the caller holds the lock.
//
// Returns:
//
//	An error if marshaling or sending the request fails, otherwise nil
func (hr *HTTPReporter) flush() error {
	// Early return if there are no spans to report
	if len(hr.spans) == 0 {
		return nil
	}

	// Marshal spans to JSON format for the request body
	body, err := json.Marshal(hr.spans)
	if err != nil {
		return fmt.Errorf("failed to marshal spans: %v", err)
	}

	// Create a new HTTP POST request to the configured endpoint
	req, err := http.NewRequest("POST", hr.endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// Set required Content-Type header and any custom headers
	req.Header.Set("Content-Type", "application/json")
	for key, value := range hr.headers {
		req.Header.Set(key, value)
	}

	// Set the request body and content length
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))

	// Send the request using the configured HTTP client
	resp, err := hr.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Check for HTTP error status codes
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned error: %d %s", resp.StatusCode, string(body))
	}

	// Clear the span buffer for future use
	hr.spans = hr.spans[:0]

	return nil
}

// reportLoop runs in a background goroutine and periodically flushes the span batch
// to the remote endpoint, regardless of whether the batch has reached its maximum size.
// It uses a ticker set to 5 seconds and stops when the stopCh is closed.
// This ensures that spans are eventually reported even if the batch never fills up.
func (hr *HTTPReporter) reportLoop() {
	defer hr.wg.Done()

	// Create a ticker that triggers every 5 seconds
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop() // Ensure ticker is stopped when goroutine exits

	// Main loop that either flushes spans or exits based on triggers
	for {
		select {
		case <-ticker.C:
			// Periodic flush triggered by ticker
			hr.mu.Lock()
			hr.flush()
			hr.mu.Unlock()
		case <-hr.stopCh:
			// Exit loop when stop signal is received
			return
		}
	}
}
