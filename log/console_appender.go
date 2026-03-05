package log

import (
	"os"
)

// ConsoleAppender 将日志直接写入标准输出。
type ConsoleAppender struct {
}

// NewConsoleAppender 创建控制台 Appender。
func NewConsoleAppender() *ConsoleAppender {
	return &ConsoleAppender{}
}

// Write 将日志写入 stdout。
func (ca *ConsoleAppender) Write(buf []byte) (int, error) {
	return os.Stdout.Write(buf)
}

// Refresh 为无操作，满足接口约定。
func (ca *ConsoleAppender) Refresh() error {
	return nil
}

// Close 为无操作，满足接口约定。
func (ca *ConsoleAppender) Close() error {
	return nil
}
