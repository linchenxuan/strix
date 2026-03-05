package log

import (
	"os"
	"testing"
)

// BenchmarkSyncLogging 测量同步日志性能。
func BenchmarkSyncLogging(b *testing.B) {
	cfg := &LogCfg{
		LogPath:           "./strix-bench.log",
		LogLevel:          DebugLevel,
		FileSplitMB:       50,
		FileSplitHour:     0,
		IsAsync:           false,
		FileAppender:      true, // 输出重定向到 os.DevNull。
		ConsoleAppender:   false,
		EnabledCallerInfo: true,
	}
	// 基准期间关闭真实 I/O 输出。
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

// BenchmarkAsyncLogging 测量异步日志性能。
func BenchmarkAsyncLogging(b *testing.B) {
	cfg := &LogCfg{
		LogPath:           "./strix-bench.log",
		LogLevel:          DebugLevel,
		FileSplitMB:       50,
		FileSplitHour:     0,
		IsAsync:           true,
		FileAppender:      true, // 输出重定向到 os.DevNull。
		ConsoleAppender:   false,
		AsyncCacheSize:    b.N, // 缓冲区设为 b.N，避免阻塞。
		AsyncWriteMillSec: 200,
		EnabledCallerInfo: true,
	}
	// 基准期间关闭真实 I/O 输出。
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

// redirectAppenderOutputToDiscard 初始化日志器并将文件输出重定向到 os.DevNull。
func redirectAppenderOutputToDiscard(cfg *LogCfg) error {
	// 初始化日志器并创建 Appender。
	if err := Initialize(cfg); err != nil {
		return err
	}

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}

	// 找到文件 Appender 并替换其文件描述符。
	for _, appender := range _defaultLogger.GetAppender() {
		if fa, ok := appender.(*FileAppender); ok {
			fa.fileFd = devNull
		}
	}
	return nil
}
