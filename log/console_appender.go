package log

import (
	"os"
)

// ConsoleAppender implements a simple console output appender for logging.
// It writes log entries directly to standard output (stdout) without any buffering,
// making it suitable for development environments, containerized applications,
// and scenarios requiring immediate log visibility.
// This appender provides zero-overhead implementation with no internal state management.
type ConsoleAppender struct {
}

// NewConsoleAppender creates and returns a new ConsoleAppender instance.
// This constructor provides a clean, stateless console appender ready for immediate use.
// The returned instance is safe for concurrent use across multiple goroutines
// as it contains no shared mutable state.
func NewConsoleAppender() *ConsoleAppender {
	return &ConsoleAppender{}
}

// Write writes the provided log buffer to standard output (stdout).
// This method implements the Appender interface and provides direct,
// unbuffered console output suitable for real-time log streaming.
// Returns the number of bytes written and any error encountered during writing.
// Thread-safe: safe for concurrent invocation from multiple goroutines.
func (ca *ConsoleAppender) Write(buf []byte) (int, error) {
	return os.Stdout.Write(buf)
}

// Refresh forces an immediate flush of any buffered data.
// Since ConsoleAppender performs unbuffered writes directly to stdout,
// this method always returns nil immediately with no actual flush operation.
// This satisfies the Appender interface requirement for explicit buffer flushing.
func (ca *ConsoleAppender) Refresh() error {
	return nil
}

// Close is a no-op for ConsoleAppender as there are no resources to release.
// It satisfies the LogAppender interface.
func (ca *ConsoleAppender) Close() error {
	return nil
}
