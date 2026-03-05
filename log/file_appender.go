package log

import (
	"bytes"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// _asyncByteSizePerIOWrite 定义异步写入的最大批量大小 (10MB)
	// 这可以防止大容量日志记录场景中的内存压力。
	_asyncByteSizePerIOWrite = 10 << 20
)

// FileAppender 将日志写入文件，支持同步与异步模式。
// 并支持按大小和时间轮转日志文件。
type FileAppender struct {
	logger            Logger             // 引用父日志器，用于配置和回退处理。
	fileName          string             // 日志文件的路径。
	fileSplitMB       int                // 轮转前的最大文件大小（以 MB 为单位）
	fileSplitHour     int                // 轮转前的时间间隔（以小时为单位）
	isAsync           bool               // 指示是否启用异步模式的标志。
	asyncWriteMillSec int                // 异步批量写入的时间间隔（以毫秒为单位）
	fileFd            *os.File           // 当前日志文件的文件描述符。
	fileCreateTime    time.Time          // 当前日志文件的创建时间。
	fileSize          int64              // 当前日志文件已写入大小（字节）。
	lock              sync.Mutex         // 用于线程安全文件操作的互斥锁。
	bufChan           chan *bytes.Buffer // 异步日志缓冲区的通道。
	ntfChan           chan chan struct{} // 通知和刷新请求的通道。
	stopChan          chan struct{}      // 异步 goroutine 关闭信号的通道。
	asyncDoneChan     chan struct{}      // 当异步 goroutine 退出时通道关闭。
	asyncSendBuf      *bytes.Buffer      // 用于累积异步写入的缓冲区。
	bufferPool        sync.Pool          // 可重用缓冲区的池以避免分配。
	closed            atomic.Bool        // 指示 Appender 是否已关闭。
	closeOnce         sync.Once          // 确保幂等关闭。
	currentConfig     *LogCfg            // 热重载支持的当前配置。
	configMutex       sync.RWMutex       // 配置读写锁。
}

// NewFileAppender 创建文件 Appender，初始化失败时会 panic。
func NewFileAppender(cfg *LogCfg, l Logger) *FileAppender {
	a := &FileAppender{
		logger: l,
	}
	if err := a.init(cfg); err != nil {
		panic(err)
	}
	return a
}

// GetCurrentConfig 返回当前配置。
func (a *FileAppender) GetCurrentConfig() *LogCfg {
	a.configMutex.RLock()
	defer a.configMutex.RUnlock()
	return a.currentConfig
}

// init 初始化轮转参数、异步通道和缓冲池。
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
		// 初始化实例级缓冲池以实现零分配性能。
		// 预分配1KB缓冲区，减少高频日志记录时的GC压力。
		a.bufferPool = sync.Pool{
			New: func() interface{} {
				return &bytes.Buffer{}
			},
		}

		a.asyncSendBuf = bytes.NewBuffer(make([]byte, 0, _asyncByteSizePerIOWrite))

		a.bufChan = make(chan *bytes.Buffer, cfg.AsyncCacheSize)
		a.ntfChan = make(chan chan struct{})
		a.stopChan = make(chan struct{})
		a.asyncDoneChan = make(chan struct{})
		go a.asyncWriteLoop()
	}

	return nil
}

// CheckCfgValid 校验并补齐 FileAppender 相关默认值。
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

// Write 根据配置选择同步或异步写入。
func (a *FileAppender) Write(buf []byte) (n int, err error) {
	if a.closed.Load() {
		return 0, errors.New("file appender is closed")
	}

	if a.isAsync {
		a.writeAsync(buf)
		return len(buf), nil
	}

	return a.writeSync(buf)
}

// Refresh 强制将异步缓冲日志立即落盘。
func (a *FileAppender) Refresh() error {
	if !a.isAsync || a.closed.Load() {
		return nil
	}
	// ntf 并等待结果。
	doneChan := make(chan struct{})
	select {
	case a.ntfChan <- doneChan:
		<-doneChan
	case <-a.stopChan:
	}
	return nil
}

// Close 关闭文件 Appender 并释放资源。
func (a *FileAppender) Close() error {
	var closeErr error
	a.closeOnce.Do(func() {
		a.closed.Store(true)

		// 停止异步写入器并等待最终刷新完成。
		if a.isAsync {
			close(a.stopChan)
			<-a.asyncDoneChan
		}

		a.lock.Lock()
		defer a.lock.Unlock()

		// 关闭文件描述符。
		if a.fileFd != nil {
			closeErr = a.fileFd.Close()
			a.fileFd = nil
		}
	})

	return closeErr
}

// writeSync 执行同步写入，并在写入前检查是否需要轮转。
func (a *FileAppender) writeSync(buf []byte) (n int, err error) {
	a.lock.Lock()
	defer a.lock.Unlock()

	// 热路径快速判断：句柄存在且未触发轮转时，直接写入避免每次 os.Stat 分配与系统调用。
	needRotate := a.fileFd == nil
	if !needRotate {
		if a.fileSplitHour > 0 && shouldRotateByTime(a.fileCreateTime, time.Now(), a.fileSplitHour) {
			needRotate = true
		}
		if !needRotate && a.fileSplitMB > 0 {
			limit := int64(a.fileSplitMB) << 20
			if a.fileSize+int64(len(buf)) >= limit {
				needRotate = true
			}
		}
	}

	if needRotate {
		oldFd := a.fileFd
		newFd, newFileCreateTime, rotateErr := UpdateFileFd(a.fileName,
			a.fileSplitHour,
			a.fileSplitMB,
			a.fileFd, a.fileCreateTime)
		if rotateErr != nil {
			return 0, rotateErr
		}
		if newFd == nil {
			return 0, errors.New("writeSync newFd err")
		}
		a.fileFd = newFd
		a.fileCreateTime = newFileCreateTime

		// 首次打开或轮转后更新文件大小基线。
		if newFd != oldFd {
			stat, statErr := newFd.Stat()
			if statErr != nil {
				return 0, statErr
			}
			a.fileSize = stat.Size()
		}
	}

	n, err = a.fileFd.Write(buf)
	if n > 0 {
		a.fileSize += int64(n)
	}
	return n, err
}

// writeAsync 将日志投递到异步队列，队列满时采用非阻塞退让策略。
func (a *FileAppender) writeAsync(buf []byte) {
	if a.closed.Load() {
		return
	}

	// 使用实例级缓冲池获取可复用的 bytes.Buffer。
	buffer := a.bufferPool.Get().(*bytes.Buffer)

	// 重置缓冲区并写入数据。
	buffer.Reset()
	buffer.Write(buf)

	select {
	case a.bufChan <- buffer:
	case <-a.stopChan:
		buffer.Reset()
		a.bufferPool.Put(buffer)
	default:
		// 当通道满时使用非阻塞策略。
		select {
		case a.bufChan <- buffer:
		case <-a.stopChan:
			buffer.Reset()
			a.bufferPool.Put(buffer)
		case a.ntfChan <- nil:
			// 通知立即写入并重试。
			select {
			case a.bufChan <- buffer:
			case <-a.stopChan:
				buffer.Reset()
				a.bufferPool.Put(buffer)
			}
		}
	}
}

// writeAll 批量消费异步队列并写入文件。
func (a *FileAppender) writeAll() {
	for {
		select {
		case buffer := <-a.bufChan:
			// 每批写入有限字节的日志以避免缓冲区重新分配。
			if a.asyncSendBuf.Len()+buffer.Len() > _asyncByteSizePerIOWrite {
				a.writeSync(a.asyncSendBuf.Bytes())
				a.asyncSendBuf.Reset()
			}
			a.asyncSendBuf.Write(buffer.Bytes())

			// 重置缓冲区并返回实例对象池。
			buffer.Reset()
			a.bufferPool.Put(buffer) // 将指针放回池中。
		default:
			if a.asyncSendBuf.Len() > 0 {
				a.writeSync(a.asyncSendBuf.Bytes())
				a.asyncSendBuf.Reset()
			}
			return
		}
	}
}

// asyncWriteLoop 负责异步写入协程的生命周期管理。
func (a *FileAppender) asyncWriteLoop() {
	defer close(a.asyncDoneChan)

	tickTimer := time.NewTicker(time.Duration(a.asyncWriteMillSec) * time.Millisecond)
	defer tickTimer.Stop()
	for {
		select {
		case <-a.stopChan:
			// 异步写协程正常退出前做最后一次 flush。
			a.writeAll()
			return
		case doneChan := <-a.ntfChan:
			// 外部 Refresh() 触发的立即刷新。
			a.writeAll()
			if doneChan != nil {
				// 确保在明确请求时将数据刷新到磁盘。
				if a.fileFd != nil {
					_ = a.fileFd.Sync()
				}
				doneChan <- struct{}{}
			}
		case <-tickTimer.C:
			// 定时写回文件。
			a.writeAll()
		}
	}
}
