package tracing

import (
	"log"
	"sync"
	"time"
)

// ConsoleReporter implements the Reporter interface to output span data to the console
// This reporter is primarily useful for development, debugging, and testing purposes
// It provides human-readable formatting of span details including trace IDs, timestamps, tags, and logs
type ConsoleReporter struct {
	mu     sync.Mutex  // Mutex to ensure thread-safe logging
	logger *log.Logger // Logger used for outputting span information
}

// NewConsoleReporter creates a new console reporter
// If no logger is provided, it uses a default logger with the "[TRACING]" prefix
func NewConsoleReporter(logger *log.Logger) Reporter {
	if logger == nil {
		logger = log.New(log.Writer(), "[TRACING] ", log.LstdFlags)
	}

	return &ConsoleReporter{
		logger: logger,
	}
}

// Report implements the Reporter interface
// It formats and logs span data in a human-readable format
// All operations are thread-safe due to mutex protection
func (r *ConsoleReporter) Report(span SpanData) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger.Printf("=== Span Report ===")
	r.logger.Printf("TraceID: %s", span.TraceID)
	r.logger.Printf("SpanID: %s", span.SpanID)
	if span.ParentSpanID != "" {
		r.logger.Printf("ParentSpanID: %s", span.ParentSpanID)
	}
	r.logger.Printf("Operation: %s", span.Operation)
	r.logger.Printf("StartTime: %s", span.StartTime.Format(time.RFC3339))
	r.logger.Printf("Duration: %v", span.Duration)

	if len(span.Tags) > 0 {
		r.logger.Printf("Tags:")
		for k, v := range span.Tags {
			r.logger.Printf("  %s: %v", k, v)
		}
	}

	if len(span.Logs) > 0 {
		r.logger.Printf("Logs:")
		for _, log := range span.Logs {
			r.logger.Printf("  [%s]", log.Timestamp.Format(time.RFC3339))
			for _, field := range log.Fields {
				r.logger.Printf("    %s: %v", field.Key, field.Value)
			}
		}
	}

	if len(span.Baggage) > 0 {
		r.logger.Printf("Baggage:")
		for k, v := range span.Baggage {
			r.logger.Printf("  %s: %s", k, v)
		}
	}

	r.logger.Printf("==================")

	return nil
}

// Close implements the Reporter interface
// No specific cleanup needed for the console reporter
func (r *ConsoleReporter) Close() error {
	return nil
}

// InMemoryReporter implements the Reporter interface to store spans in memory
// This reporter is primarily useful for testing and scenarios where spans need to be programmatically accessed
// It provides methods to retrieve, count, and clear stored spans
type InMemoryReporter struct {
	mu    sync.RWMutex // Read-write mutex for thread-safe access
	spans []SpanData   // Collection of stored spans
}

// NewInMemoryReporter creates a new in-memory reporter with an empty span collection
func NewInMemoryReporter() *InMemoryReporter {
	return &InMemoryReporter{
		spans: make([]SpanData, 0),
	}
}

// Report implements the Reporter interface
// It adds the span to the in-memory collection
// All operations are thread-safe due to mutex protection
func (r *InMemoryReporter) Report(span SpanData) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.spans = append(r.spans, span)
	return nil
}

// Close implements the Reporter interface
// No specific cleanup needed for the in-memory reporter
func (r *InMemoryReporter) Close() error {
	return nil
}

// GetSpans returns a copy of all stored spans
// This method is thread-safe and returns a deep copy to prevent external modification
func (r *InMemoryReporter) GetSpans() []SpanData {
	r.mu.RLock()
	defer r.mu.RUnlock()

	spans := make([]SpanData, len(r.spans))
	copy(spans, r.spans)
	return spans
}

// Clear removes all stored spans from memory
// This method is thread-safe
func (r *InMemoryReporter) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.spans = r.spans[:0] // Reset slice length while preserving capacity
}

// GetSpanCount returns the number of stored spans
// This method is thread-safe and uses read-only access for efficiency
func (r *InMemoryReporter) GetSpanCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.spans)
}

// BatchReporter wraps another reporter to provide batching functionality
// It accumulates spans and sends them in batches to improve performance
// Features include configurable batch size and automatic flushing after a timeout
type BatchReporter struct {
	reporter      Reporter       // Underlying reporter that actually processes the spans
	maxBatchSize  int            // Maximum number of spans per batch
	flushInterval time.Duration  // Time after which a partial batch is automatically flushed
	spans         []SpanData     // Buffer for accumulating spans
	mu            sync.Mutex     // Mutex for thread-safe operations
	flushTimer    *time.Timer    // Timer for automatic flushing
	stopCh        chan struct{}  // Channel for signaling shutdown
	wg            sync.WaitGroup // WaitGroup for managing background goroutine
}

// NewBatchReporter creates a new batch reporter
// reporter: The underlying reporter to use
// maxBatchSize: Maximum number of spans per batch (default 100)
// flushInterval: Time after which to flush a partial batch (default 5 seconds)
func NewBatchReporter(reporter Reporter, maxBatchSize int, flushInterval time.Duration) *BatchReporter {
	if maxBatchSize <= 0 {
		maxBatchSize = 100
	}
	if flushInterval <= 0 {
		flushInterval = 5 * time.Second
	}

	br := &BatchReporter{
		reporter:      reporter,
		maxBatchSize:  maxBatchSize,
		flushInterval: flushInterval,
		spans:         make([]SpanData, 0, maxBatchSize), // Pre-allocate capacity
		stopCh:        make(chan struct{}),
	}

	// Start background flushing loop
	br.wg.Add(1)
	go br.flushLoop()

	return br
}

// Report implements the Reporter interface
// It adds the span to the batch and triggers flushing when necessary
// If the batch reaches maximum size, it flushes immediately
// Otherwise, it starts a timer for automatic flushing
func (br *BatchReporter) Report(span SpanData) error {
	br.mu.Lock()
	defer br.mu.Unlock()

	br.spans = append(br.spans, span)

	// Start the timer if not already started
	if br.flushTimer == nil {
		br.flushTimer = time.AfterFunc(br.flushInterval, func() {
			br.mu.Lock()
			br.flush()
			br.mu.Unlock()
		})
	}

	// If batch is full, flush immediately
	if len(br.spans) >= br.maxBatchSize {
		br.flush()
	}

	return nil
}

// Close shuts down the batch reporter gracefully
// It signals the flushLoop to stop, waits for it to finish, and flushes any remaining spans
func (br *BatchReporter) Close() error {
	close(br.stopCh) // Signal flushLoop to exit
	br.wg.Wait()     // Wait for flushLoop to finish

	br.mu.Lock()
	defer br.mu.Unlock()

	// Flush any remaining spans
	br.flush()

	// Close the underlying reporter
	return br.reporter.Close()
}

// flush sends accumulated spans to the underlying reporter
// It cancels any pending timer and resets the span buffer
func (br *BatchReporter) flush() {
	if len(br.spans) == 0 {
		return
	}

	// Reset timer
	if br.flushTimer != nil {
		br.flushTimer.Stop()
		br.flushTimer = nil
	}

	// Report all spans
	for _, span := range br.spans {
		br.reporter.Report(span)
	}

	// Clear batch
	br.spans = br.spans[:0]
}

func (br *BatchReporter) flushLoop() {
	defer br.wg.Done()

	ticker := time.NewTicker(br.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			br.mu.Lock()
			br.flush()
			br.mu.Unlock()
		case <-br.stopCh:
			return
		}
	}
}

// AsyncReporter reports spans asynchronously
type AsyncReporter struct {
	reporter Reporter
}

// NewAsyncReporter creates a new async reporter
func NewAsyncReporter(reporter Reporter, queueSize int) *AsyncReporter {
	// Ignore queueSize since we now directly call reporter.Report
	return &AsyncReporter{
		reporter: reporter,
	}
}

func (ar *AsyncReporter) Report(span SpanData) error {
	// Async reporter directly calls reporter.Report
	return ar.reporter.Report(span)
}

func (ar *AsyncReporter) Close() error {
	return ar.reporter.Close()
}

// Since we simplified the AsyncReporter implementation, the reportLoop method is no longer needed
// The reportLoop method has been removed because AsyncReporter now directly calls reporter.Report
