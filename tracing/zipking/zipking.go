package zipking

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/linchenxuan/strix/tracing"
)

type ZipkinReporterConfig struct {
	Endpoint       string `mapmapstructure:"endpoint"`    // Zipkin server endpoint URL
	ServiceName    string `mapmapstructure:"serviceName"` // Name of the service being traced
	BatchSize      int    `mapmapstructure:"batchSize"`   // Maximum number of spans to batch before flushing
	RequestTimeout int    `mapmapstructure:"requestTimeout"`
}

// ZipkinReporter reports spans to Zipkin distributed tracing system
// It implements the Reporter interface and provides batch processing and async reporting capabilities
// to minimize performance impact on the main application thread
type ZipkinReporter struct {
	cfg    *ZipkinReporterConfig
	client *http.Client   // HTTP client for sending span data
	spans  []ZipkinSpan   // Buffer for spans waiting to be sent
	mu     sync.Mutex     // Mutex to protect concurrent access to spans
	stopCh chan struct{}  // Channel to signal the reporter to stop
	wg     sync.WaitGroup // WaitGroup to wait for the reportLoop goroutine to finish
}

// ZipkinSpan represents a span in Zipkin format
// This struct is used to convert internal SpanData to the format expected by Zipkin
type ZipkinSpan struct {
	TraceID       string            `json:"traceId"`            // Unique identifier for the trace
	Name          string            `json:"name"`               // Name of the operation being traced
	ID            string            `json:"id"`                 // Unique identifier for this span
	ParentID      string            `json:"parentId,omitempty"` // ID of the parent span (optional)
	Timestamp     int64             `json:"timestamp"`          // Start time in microseconds since epoch
	Duration      int64             `json:"duration"`           // Duration in microseconds
	LocalEndpoint ZipkinEndpoint    `json:"localEndpoint"`      // Information about the local service
	Tags          map[string]string `json:"tags,omitempty"`     // Optional key-value pairs with additional information
}

// ZipkinEndpoint represents an endpoint in Zipkin format
// Contains information about the service reporting the span
type ZipkinEndpoint struct {
	ServiceName string `json:"serviceName"` // Name of the service
}

// newZipkinReporter creates a new Zipkin reporter
func newZipkinReporter(cfg *ZipkinReporterConfig) *ZipkinReporter {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}

	zr := &ZipkinReporter{
		cfg:    cfg,
		client: &http.Client{Timeout: time.Duration(cfg.RequestTimeout)},
		spans:  make([]ZipkinSpan, 0, cfg.BatchSize),
		stopCh: make(chan struct{}),
	}
	zr.wg.Add(1)
	go zr.start()
	return zr
}

// Report implements the Reporter interface
// It converts the given SpanData to Zipkin format and adds it to the batch
// If the batch size is reached, it automatically flushes the batch
func (zr *ZipkinReporter) Report(span tracing.SpanData) error {
	zipkinSpan := zr.convertToZipkin(span)

	zr.mu.Lock()
	defer zr.mu.Unlock()

	zr.spans = append(zr.spans, zipkinSpan)

	// Flush if batch size is reached
	if len(zr.spans) >= zr.cfg.BatchSize {
		return zr.flush()
	}

	return nil
}

// Close closes the reporter and flushes any remaining spans
// It signals the reportLoop to stop and waits for it to finish
// Then it locks and flushes any remaining spans in the buffer
func (zr *ZipkinReporter) stop() error {
	// Check if the channel is already closed
	select {
	case <-zr.stopCh:
		// Channel already closed, return immediately
		return nil
	default:
		// Channel not closed, close it and continue
		close(zr.stopCh)
	}
	// Wait for the reportLoop goroutine to finish
	zr.wg.Wait()

	// Lock and flush any remaining spans
	zr.mu.Lock()
	defer zr.mu.Unlock()

	// Flush remaining spans
	return zr.flush()
}

// convertToZipkin converts SpanData to ZipkinSpan format
// It transforms internal tracing data to the format expected by Zipkin
// Converts timestamps from time.Time to microseconds and tags to string values
func (zr *ZipkinReporter) convertToZipkin(data tracing.SpanData) ZipkinSpan {
	// Convert tags to string values as required by Zipkin
	tags := make(map[string]string)
	for k, v := range data.Tags {
		tags[k] = fmt.Sprintf("%v", v)
	}

	return ZipkinSpan{
		TraceID:   data.TraceID,
		Name:      data.Operation,
		ID:        data.SpanID,
		ParentID:  data.ParentSpanID,
		Timestamp: data.StartTime.UnixNano() / 1000, // Convert to microseconds
		Duration:  int64(data.Duration) / 1000,      // Convert to microseconds
		LocalEndpoint: ZipkinEndpoint{
			ServiceName: zr.cfg.ServiceName,
		},
		Tags: tags,
	}
}

// flush sends the current batch of spans to Zipkin
// It marshals the spans to JSON, creates an HTTP request, and sends it to the Zipkin server
// If successful, it clears the batch buffer
func (zr *ZipkinReporter) flush() error {
	if len(zr.spans) == 0 {
		return nil
	}

	// Create request body by marshaling spans to JSON
	body, err := json.Marshal(zr.spans)
	if err != nil {
		return fmt.Errorf("failed to marshal spans: %v", err)
	}

	// Create HTTP request to Zipkin API
	req, err := http.NewRequest("POST", zr.cfg.Endpoint+"/api/v2/spans", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Set request body
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))

	// Send request to Zipkin server
	resp, err := zr.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned error: %d %s", resp.StatusCode, string(body))
	}

	// Clear the batch buffer for future spans
	zr.spans = zr.spans[:0]

	return nil
}

// start is a background goroutine that periodically flushes spans
// It runs a ticker that triggers a flush every 5 seconds
// It also listens for the stop signal to exit
func (zr *ZipkinReporter) start() {
	defer zr.wg.Done()

	// Create a ticker to trigger periodic flushing (every 5 seconds)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Periodic flush to ensure spans are sent even if batch size isn't reached
			zr.mu.Lock()
			zr.flush()
			zr.mu.Unlock()
		case <-zr.stopCh:
			// Exit the loop when stop signal is received
			return
		}
	}
}
