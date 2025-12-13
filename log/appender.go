package log

// LogAppender defines the interface for log appenders in the Strix logging framework.
// It provides the core abstraction for different log output destinations (console, file, network, etc.)
// ensuring high-performance and extensible logging capabilities for distributed game servers.
//
// Implementations should be goroutine-safe as appenders are typically accessed concurrently
// by multiple goroutines in high-throughput game server scenarios.
type LogAppender interface {
	// Write outputs the formatted log data to the destination.
	// Implementations should be optimized for high-throughput scenarios typical
	// in game server environments with thousands of concurrent connections.
	//
	// The method must be thread-safe and should handle backpressure gracefully
	// to prevent memory exhaustion under extreme load conditions.
	Write(buf []byte) (n int, err error)

	// Refresh forces any buffered log data to be written immediately.
	// This is particularly important for asynchronous appenders to ensure
	// log persistence before critical operations (e.g., server shutdown, player data persistence).
	//
	// The method should block until all pending logs are flushed to the underlying storage.
	Refresh() error

	// Close flushes any buffered logs and releases underlying resources.
	// It should be called at application shutdown to prevent data loss.
	Close() error
}
