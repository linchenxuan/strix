package log

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// GameLogger 是线程安全的核心日志器。
// 它负责日志级别过滤、事件对象复用以及多 Appender 分发。
type GameLogger struct {
	appenders         []LogAppender // 负责日志输出的 Appender 集合。
	minLevel          Level         // 将处理的最低日志级别。
	callerSkip        int           // 捕获调用者信息时要跳过的堆栈帧数。
	eventPool         *sync.Pool    // LogEvent 对象池，用于降低 GC 压力。
	levelChange       *levelChange  // 每个文件/每行日志级别覆盖的配置。
	callerCache       sync.Map      // 缓存调用者信息，避免冗余计算。
	enabledCallerInfo bool          // 是否写入 caller 字段。

	configMutex   sync.RWMutex // 配置读写锁。
	currentConfig *LogCfg      // 当前配置快照。
}

// NewLogger 创建一个 GameLogger。
// 当 cfg 为 nil 时使用默认配置。
func NewLogger(cfg *LogCfg) *GameLogger {
	if cfg == nil {
		cfg = getDefaultCfg()
	}

	logger := &GameLogger{
		minLevel:          cfg.LogLevel,
		callerSkip:        cfg.CallerSkip,
		levelChange:       newLevelChange(cfg.LevelChange),
		enabledCallerInfo: cfg.EnabledCallerInfo,
		currentConfig:     cfg,
	}

	// 初始化事件对象池。
	logger.eventPool = &sync.Pool{
		New: func() any {
			return newEvent(logger)
		},
	}

	// 按配置安装 Appender。
	if cfg.FileAppender {
		logger.AddAppender(NewFileAppender(cfg, logger))
	}

	if cfg.ConsoleAppender {
		logger.AddAppender(NewConsoleAppender())
	}

	return logger
}

// GetCurrentConfig 返回当前配置。
func (x *GameLogger) GetCurrentConfig() *LogCfg {
	x.configMutex.RLock()
	defer x.configMutex.RUnlock()
	return x.currentConfig
}

// checkLevel 判断给定级别是否满足最小级别要求。
func (x *GameLogger) checkLevel(level Level) bool {
	currentLevel := Level(atomic.LoadUint32((*uint32)(unsafe.Pointer(&x.minLevel))))
	return currentLevel <= level
}

// AddAppender 添加一个日志 Appender。
func (x *GameLogger) AddAppender(appender LogAppender) {
	x.appenders = append(x.appenders, appender)
}

// GetAppender 返回当前已注册的 Appender 列表。
func (x *GameLogger) GetAppender() []LogAppender {
	return x.appenders
}

// Refresh 刷新所有已注册 Appender。
func (x *GameLogger) Refresh() {
	for _, appender := range x.appenders {
		appender.Refresh()
	}
}

// Close 关闭所有已注册 Appender。
func (x *GameLogger) Close() {
	for _, appender := range x.appenders {
		appender.Close()
	}
}

// IgnoreCheckLevel 返回是否跳过级别检查。
// GameLogger 固定返回 false。
func (x *GameLogger) IgnoreCheckLevel() bool {
	return false
}

// newEvent 从对象池获取并重置一个 LogEvent。
func (x *GameLogger) newEvent() *LogEvent {
	e := x.eventPool.Get().(*LogEvent)
	e.Reset()
	return e
}

// OnEventEnd 结束事件生命周期并回收对象。
// 当级别为 Fatal 时触发 panic。
func (x *GameLogger) OnEventEnd(e *LogEvent) {
	// 写入所有已配置的 Appender（控制台、文件等）。
	for _, appender := range x.appenders {
		appender.Write(e.buf.Bytes())
	}

	if e.level == FatalLevel {
		panic("")
	}

	x.eventPool.Put(e)
}

// Debug 创建调试级日志事件。
func (x *GameLogger) Debug() *LogEvent {
	return x.log(DebugLevel)
}

// Info 创建信息级日志事件。
func (x *GameLogger) Info() *LogEvent {
	return x.log(InfoLevel)
}

// Warn 创建警告级日志事件。
func (x *GameLogger) Warn() *LogEvent {
	return x.log(WarnLevel)
}

// Error 创建错误级日志事件。
func (x *GameLogger) Error() *LogEvent {
	return x.log(ErrorLevel)
}

// Fatal 创建致命级日志事件。
func (x *GameLogger) Fatal() *LogEvent {
	return x.log(FatalLevel)
}

// getCallerInfo 返回调用方的文件、函数和行号信息。
func (x *GameLogger) getCallerInfo() *callerInfo {
	// runtime.Callers 仅采集 PC，可避免 runtime.Caller 返回 file string 的常态分配。
	var pcs [1]uintptr
	// +4: runtime.Callers + getCallerInfo + log + level wrapper(Debug/Info/...)
	if runtime.Callers(4+x.callerSkip, pcs[:]) == 0 {
		return _UnknownCallerInfo
	}
	pc := pcs[0] - 1

	// 优先读取缓存，避免重复解析。
	if cached, found := x.callerCache.Load(pc); found {
		return cached.(*callerInfo)
	}

	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return _UnknownCallerInfo
	}

	// 提取函数名。
	funcName := fn.Name()
	var function string
	if dotIdx := strings.LastIndexByte(funcName, '.'); dotIdx != -1 {
		function = funcName[dotIdx+1:]
	} else {
		function = funcName
	}

	file, line := fn.FileLine(pc)

	// 仅保留末两级路径，减少日志长度。
	if len(file) > 0 {
		lastSlash := strings.LastIndexByte(file, '/')
		if lastSlash > 0 {
			// 找到倒数第二个目录分隔符。
			secondLastSlash := strings.LastIndexByte(file[:lastSlash], '/')
			if secondLastSlash >= 0 {
				file = file[secondLastSlash+1:]
			}
		}
	}

	c := newCallerInfo(file, function, line)

	// 缓存解析结果，提升后续命中性能。
	x.callerCache.Store(pc, c)

	return c
}

// log 构建一个带公共字段的日志事件。
// 若级别未通过过滤则返回 nil。
func (x *GameLogger) log(level Level) *LogEvent {
	var info *callerInfo
	// 检查是否需要做级别过滤。
	if !x.IgnoreCheckLevel() {
		// 当前级别不满足最小阈值时，尝试按文件/行号覆盖。
		if !x.checkLevel(level) {
			if x.levelChange.Empty() {
				return nil
			}
			// 读取调用方信息并计算覆盖后的级别。
			info = x.getCallerInfo()
			lv := x.levelChange.GetLevel(info.file, info.line, level)
			level = lv
		}
	}

	// 覆盖后做最终级别检查。
	if !x.checkLevel(level) {
		return nil
	}

	// 从对象池中获取事件对象。
	e := x.newEvent()

	// 写入公共字段：时间戳、级别。
	t := time.Now()
	e.Time("time", &t)
	e.Str("level", level.String())

	// 按配置写入 caller 字段。
	if x.enabledCallerInfo {
		if info == nil {
			info = x.getCallerInfo()
		}
		e.Str("caller", info.String())
	}

	return e
}
