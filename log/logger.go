package log

// Logger 定义日志器接口，提供各级别结构化日志方法。
type Logger interface {
	Debug() *LogEvent
	Info() *LogEvent
	Warn() *LogEvent
	Error() *LogEvent
	Fatal() *LogEvent
	IgnoreCheckLevel() bool
	GetAppender() []LogAppender
	AddAppender(appender LogAppender)
	OnEventEnd(e *LogEvent)
}

var _defaultLogger *GameLogger

func init() {
	// 包初始化时先使用默认配置创建日志器。
	_defaultLogger = NewLogger(getDefaultCfg())
}

// Initialize 使用给定配置初始化默认日志器。
// 如果 cfg 为 nil，则使用默认配置。
func Initialize(cfg *LogCfg) error {
	if cfg == nil {
		cfg = getDefaultCfg()
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	SetDefaultLogger(NewLogger(cfg))
	return nil
}

// AddAppender 向默认日志器添加 Appender。
func AddAppender(appender LogAppender) {
	_defaultLogger.AddAppender(appender)
}

// Refresh 触发默认日志器所有 Appender 的刷新操作。
func Refresh() {
	_defaultLogger.Refresh()
}

// Close 刷新并关闭默认日志器及其 Appender。
func Close() {
	_defaultLogger.Close()
}

// SetDefaultLogger 用自定义实例替换默认日志器。
func SetDefaultLogger(logger *GameLogger) {
	_defaultLogger = logger
}

// Debug 使用默认日志器创建调试级日志事件。
func Debug() *LogEvent {
	return _defaultLogger.Debug()
}

// Info 使用默认日志器创建信息级日志事件。
func Info() *LogEvent {
	return _defaultLogger.Info()
}

// Warn 使用默认日志器创建警告级日志事件。
func Warn() *LogEvent {
	return _defaultLogger.Warn()
}

// Error 使用默认日志器创建错误级日志事件。
func Error() *LogEvent {
	return _defaultLogger.Error()
}

// Fatal 使用默认日志器创建致命级日志事件。
func Fatal() *LogEvent {
	return _defaultLogger.Fatal()
}
