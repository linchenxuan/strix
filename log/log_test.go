package log

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func TestFileLogging(t *testing.T) {
	// 1. Setup a temporary log file
	tmpfile, err := ioutil.TempFile("", "test_log_*.log")
	if err != nil {
		t.Fatal(err)
	}
	logPath := tmpfile.Name()
	// Close the file so the logger can open it
	tmpfile.Close()
	defer os.Remove(logPath) // Clean up the file afterwards

	// 2. Configure and initialize the logger
	cfg := &LogCfg{
		LogPath:         logPath,
		LogLevel:        DebugLevel,
		FileSplitMB:     10,
		FileSplitHour:   0, // No time-based splitting during test
		IsAsync:         false, // Use synchronous mode for predictable testing
		FileAppender:    true,
		ConsoleAppender: false, // Disable console to not pollute test output
		EnabledCallerInfo: true,
	}
	err = Initialize(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// 3. Log a message
	testMessage := "this is a test message"
	Info().Msg(testMessage)

	// 4. Refresh to ensure the log is written (especially important for async)
	Refresh()
	Close() // Close the appender to finalize writes

	// 5. Re-initialize with a default logger to avoid side-effects
	Initialize(nil)

	// 6. Read the file and verify content
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
