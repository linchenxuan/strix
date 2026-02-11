package log

import (
	"bytes"
	"errors"
	"os"
	"sync"
	"time"
)

const (
	// _asyncByteSizePerIOWrite defines the maximum batch size for async writes (10MB)
	// This prevents memory pressure during high-volume logging scenarios
	_asyncByteSizePerIOWrite = 10 << 20
)

// FileAppender handles log output to files with support for both synchronous and asynchronous modes.
// It provides automatic file rotation based on size and time, zero-allocation buffer management,
// and high-performance concurrent log writing capabilities suitable for game server environments.
type FileAppender struct {
	logger            Logger             // Reference to the parent logger for configuration and fallback
	fileName          string             // Path to the log file
	fileSplitMB       int                // Maximum file size in MB before rotation
	fileSplitHour     int                // Time interval in hours before rotation
	isAsync           bool               // Flag indicating if async mode is enabled
	asyncWriteMillSec int                // Interval in milliseconds for async batch writes
	fileFd            *os.File           // File descriptor for the current log file
	fileCreateTime    time.Time          // Creation time of the current log file
	lock              sync.Mutex         // Mutex for thread-safe file operations
	bufChan           chan *bytes.Buffer // Channel for async log buffers
	ntfChan           chan chan struct{} // Channel for notification and flush requests
	asyncSendBuf      *bytes.Buffer      // Buffer for accumulating async writes
	bufferPool        sync.Pool          // Pool for reusable buffers to avoid allocations
	currentConfig     *LogCfg            // Current configuration for hot-reload support
	configMutex       sync.RWMutex       // Mutex for thread-safe configuration updates
}

// NewFileAppender creates a new FileAppender instance with the specified configuration.
// This constructor initializes all necessary components for file-based logging,
// including file rotation settings, buffer management, and async mode configuration.
// Panics if initialization fails, ensuring early detection of configuration errors.
//
// Parameters:
//   - cfg: Log configuration specifying path, rotation rules, and async behavior
//   - l: Logger instance for event routing and fallback handling
//
// Returns: Newly created FileAppender instance ready for logging operations
func NewFileAppender(cfg *LogCfg, l Logger) *FileAppender {
	a := &FileAppender{
		logger: l,
	}
	if err := a.init(cfg); err != nil {
		panic(err)
	}
	return a
}

// GetCurrentConfig returns the current file appender configuration.
// This method provides thread-safe access to the current configuration
// for inspection or debugging purposes.
//
// Returns:
//   - Current file appender configuration
func (a *FileAppender) GetCurrentConfig() *LogCfg {
	a.configMutex.RLock()
	defer a.configMutex.RUnlock()
	return a.currentConfig
}

// init initializes the FileAppender with configuration parameters.
// This method sets up file rotation parameters, async mode configuration, and buffer pools.
// It validates the configuration first, then initializes all necessary components.
// For async mode, it creates buffer pool, channels, and starts the async write goroutine.
//
// Parameters:
//   - cfg: Log configuration specifying path, rotation rules, and async behavior
//
// Returns: nil on success, or error if configuration validation fails
func (a *FileAppender) init(cfg *LogCfg) error {
	if err := CheckCfgValid(cfg); err != nil {
		return err
	}

	a.configMutex.Lock()
	defer a.configMutex.Unlock()

	a.fileName = cfg.LogPath
	a.isAsync = cfg.IsAsync
	a.asyncWriteMillSec = cfg.AsyncWriteMillSec
	a.fileSplitMB = cfg.FileSplitMB
	a.fileSplitHour = cfg.FileSplitHour
	a.currentConfig = cfg

	if cfg.IsAsync {
		// Initialize instance-level buffer pool for zero-allocation performance
		// Pre-allocates 1KB buffers to reduce GC pressure during high-frequency logging
		a.bufferPool = sync.Pool{
			New: func() interface{} {
				return &bytes.Buffer{}
			},
		}

		a.asyncSendBuf = bytes.NewBuffer(make([]byte, 0, _asyncByteSizePerIOWrite))

		a.bufChan = make(chan *bytes.Buffer, cfg.AsyncCacheSize)
		a.ntfChan = make(chan chan struct{})
		go a.asyncWriteLoop()
	}

	return nil
}

// CheckCfgValid validates and normalizes configuration parameters.
// This function ensures all configuration values are within valid ranges and
// applies sensible defaults for missing or invalid parameters. It handles log path,
// log level, file rotation settings, and async mode configuration.
//
// Parameters:
//   - cfg: Log configuration to validate and normalize
//
// Returns: nil on success, as defaults are applied for any missing or invalid values
func CheckCfgValid(cfg *LogCfg) error {
	if len(cfg.LogPath) == 0 {
		cfg.LogPath = "./strix.log"
	}
	if cfg.LogLevel <= 0 {
		cfg.LogLevel = DebugLevel
	}

	if cfg.FileSplitMB <= 0 {
		cfg.FileSplitMB = 50
	}

	if cfg.FileSplitHour < 0 {
		cfg.FileSplitHour = 24
	}

	if cfg.IsAsync {
		if cfg.AsyncCacheSize <= 0 {
			cfg.AsyncCacheSize = 1024
		}
		if cfg.AsyncWriteMillSec <= 0 {
			cfg.AsyncWriteMillSec = 200
		}
	}
	return nil
}

// Write handles log data writing with automatic mode selection based on configuration.
// This method implements the LogAppender interface and serves as a dispatcher between
// synchronous (blocking) and asynchronous (non-blocking) write operations.
// In async mode, it returns immediately after queueing the data, while in sync mode
// it blocks until the write is completed.
//
// Parameters:
//   - buf: Byte slice containing the log data to write
//
// Returns:
//   - n: Number of bytes written (always len(buf) in async mode)
//   - err: Error if writing failed (only possible in sync mode)
func (a *FileAppender) Write(buf []byte) (n int, err error) {
	if a.isAsync {
		a.writeAsync(buf)
		return len(buf), nil
	}

	return a.writeSync(buf)
}

// Refresh forces immediate flush of all buffered logs to disk.
// This method implements the LogAppender interface and is primarily useful in async mode,
// where it ensures that all pending log entries are written to disk before returning.
// In sync mode, it returns immediately as there are no buffers to flush.
//
// Returns: nil on success, or error if flushing failed (unlikely but possible)
func (a *FileAppender) Refresh() error {
	if !a.isAsync {
		return nil
	}
	// ntf and wait result
	doneChan := make(chan struct{})
	a.ntfChan <- doneChan
	<-doneChan
	return nil
}

// Close closes the file appender and releases all resources.
// It flushes all pending logs, closes the notification channel to stop the async goroutine,
// and closes the file descriptor.
func (a *FileAppender) Close() error {
	a.lock.Lock()
	defer a.lock.Unlock()

	// Flush all pending logs
	if a.isAsync {
		// Close notification channel to stop async goroutine
		close(a.ntfChan)
		// Write all remaining buffers
		a.writeAll()
	}

	// Close file descriptor
	if a.fileFd != nil {
		err := a.fileFd.Close()
		a.fileFd = nil
		return err
	}
	return nil
}

// writeSync performs synchronized log writing with file rotation support.
// This method handles file rotation based on size and time thresholds before writing data.
// It is thread-safe through mutex locking to ensure proper file access in concurrent environments.
// Typically used for direct blocking writes or by the async writer goroutine for batch flushing.
//
// Parameters:
//   - buf: Byte slice containing the log data to write
//
// Returns:
//   - n: Number of bytes written
//   - err: Error if writing failed
func (a *FileAppender) writeSync(buf []byte) (n int, err error) {
	a.lock.Lock()
	defer a.lock.Unlock()

	newFd, newFileCreateTime, err := UpdateFileFd(a.fileName,
		a.fileSplitHour,
		a.fileSplitMB,
		a.fileFd, a.fileCreateTime)
	if err != nil {
		return 0, err
	}
	if newFd == nil {
		return 0, errors.New("writeSync newFd err")
	}
	a.fileFd = newFd
	a.fileCreateTime = newFileCreateTime
	return a.fileFd.Write(buf)
}

// writeAsync performs non-blocking asynchronous log writing with zero-allocation optimization.
// This method implements high-performance async logging by utilizing instance-level buffer pool
// to avoid memory allocations and channels for concurrent processing. It implements a two-level
// non-blocking strategy to handle backpressure when the queue is full, ensuring log entries
// are prioritized appropriately.
//
// Parameters:
//   - buf: Byte slice containing the log data to write
func (a *FileAppender) writeAsync(buf []byte) {
	// Use instance-level buffer pool to get a reusable bytes.Buffer
	buffer := a.bufferPool.Get().(*bytes.Buffer)

	// Reset buffer and write data
	buffer.Reset()
	buffer.Write(buf)

	select {
	case a.bufChan <- buffer:
	default:
		// Use non-blocking strategy when channel is full
		select {
		case a.bufChan <- buffer:
		case a.ntfChan <- nil:
			// Notify immediate write and retry
			a.bufChan <- buffer
		}
	}
}

// writeAll processes all pending log buffers from the async queue.
// This method implements efficient batch writing with size limits to optimize I/O performance
// and minimize disk operations. It ensures proper buffer recycling to prevent memory leaks
// and manages buffer growth to maintain consistent performance.
func (a *FileAppender) writeAll() {
	for {
		select {
		case buffer := <-a.bufChan:
			// Write logs with limited bytes per batch to avoid buffer reallocation.
			if a.asyncSendBuf.Len()+buffer.Len() > _asyncByteSizePerIOWrite {
				a.writeSync(a.asyncSendBuf.Bytes())
				a.asyncSendBuf.Reset()
			}
			a.asyncSendBuf.Write(buffer.Bytes())

			// Reset buffer and return to instance object pool
			buffer.Reset()
			a.bufferPool.Put(buffer) // Put pointer back into pool
		default:
			if a.asyncSendBuf.Len() > 0 {
				a.writeSync(a.asyncSendBuf.Bytes())
				a.asyncSendBuf.Reset()
			}
			return
		}
	}
}

// asyncWriteLoop manages the asynchronous log writing goroutine lifecycle.
// This method runs in a dedicated goroutine and coordinates between timer-based batch writes
// and immediate flush requests triggered by Refresh(). It handles graceful shutdown and ensures
// all pending logs are written before exiting. This goroutine is started during initialization
// when async mode is enabled.
func (a *FileAppender) asyncWriteLoop() {
	tickTimer := time.NewTicker(time.Duration(a.asyncWriteMillSec) * time.Millisecond)
	defer tickTimer.Stop()
	for {
		select {
		case doneChan, ok := <-a.ntfChan:
			// Immediate flush triggered by external Refresh() call
			a.writeAll()
			if doneChan != nil {
				// Ensure data is flushed to disk when explicitly requested
				if a.fileFd != nil {
					_ = a.fileFd.Sync()
				}
				doneChan <- struct{}{}
			}
			if !ok {
				// Graceful shutdown of async writer goroutine
				return
			}
		case <-tickTimer.C:
			// Timed write back to file
			a.writeAll()
		}
	}
}
