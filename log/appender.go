package log

// LogAppender 定义日志输出端接口（如控制台、文件等）。
// 实现必须支持并发调用。
type LogAppender interface {
	// Write 输出格式化后的日志数据。
	Write(buf []byte) (n int, err error)

	// Refresh 强制刷新缓冲日志，通常用于异步写入场景。
	Refresh() error

	// Close 关闭输出端并释放底层资源。
	Close() error
}
