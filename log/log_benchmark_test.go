package log

import (
	"os"
	"testing"
)

// BenchmarkSyncLogging measures the performance of synchronous logging.
func BenchmarkSyncLogging(b *testing.B) {
	cfg := &LogCfg{
		IsAsync:         false,
		FileAppender:    true, // Will be redirected to Discard
		ConsoleAppender: false,
	}
	// Suppress output during benchmark
	err := redirectAppenderOutputToDiscard(cfg)
	if err != nil {
		b.Fatalf("Failed to redirect output: %v", err)
	}
	defer Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Info().Msg("benchmark message")
		}
	})
}

// BenchmarkAsyncLogging measures the performance of asynchronous logging.
func BenchmarkAsyncLogging(b *testing.B) {
	cfg := &LogCfg{
		IsAsync:         true,
		FileAppender:    true, // Will be redirected to Discard
		ConsoleAppender: false,
		AsyncCacheSize:  b.N, // Set cache size to avoid blocking
	}
	// Suppress output during benchmark
	err := redirectAppenderOutputToDiscard(cfg)
	if err != nil {
		b.Fatalf("Failed to redirect output: %v", err)
	}
	defer Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Info().Msg("benchmark message")
		}
	})
}

// redirectAppenderOutputToDiscard is a helper to initialize the logger
// but force its output to os.DevNull to measure pure logging overhead
// without I/O interference.
func redirectAppenderOutputToDiscard(cfg *LogCfg) error {
	// Initialize logger which will create appenders
	Initialize(cfg)

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}

	// Find the file appender and replace its file descriptor
	for _, appender := range _defaultLogger.GetAppender() {
		if fa, ok := appender.(*FileAppender); ok {
			fa.fileFd = devNull
		}
	}
	return nil
}