package log

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func TestFileLogging(t *testing.T) {
	// 1. 创建临时日志文件。
	tmpfile, err := ioutil.TempFile("", "test_log_*.log")
	if err != nil {
		t.Fatal(err)
	}
	logPath := tmpfile.Name()
	// 关闭临时文件，便于后续由日志器重新打开。
	tmpfile.Close()
	defer os.Remove(logPath) // 测试结束后清理。

	// 2. 初始化日志器。
	cfg := &LogCfg{
		LogPath:           logPath,
		LogLevel:          DebugLevel,
		FileSplitMB:       10,
		FileSplitHour:     0,     // 测试期间不按时间轮转。
		IsAsync:           false, // 使用同步模式，结果更稳定。
		FileAppender:      true,
		ConsoleAppender:   false, // 禁用控制台输出，避免污染测试日志。
		EnabledCallerInfo: true,
	}
	err = Initialize(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// 3. 写入测试消息。
	testMessage := "this is a test message"
	Info().Msg(testMessage)

	// 4. 刷新并关闭，确保日志落盘。
	Refresh()
	Close() // 关闭Appender以完成写入。

	// 5. 恢复默认日志器，避免影响其他测试。
	Initialize(nil)

	// 6. 读取日志文件并校验内容。
	content, err := ioutil.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logOutput := string(content)
	if !strings.Contains(logOutput, testMessage) {
		t.Errorf("Log file does not contain the test message.\nExpected to find: '%s'\nGot: %s", testMessage, logOutput)
	}
	if !strings.Contains(logOutput, "INFO") {
		t.Errorf("Log file does not contain the log level 'INFO'.\nGot: %s", logOutput)
	}
	t.Logf("Log content: %s", logOutput)
}
