package log

import (
	"fmt"
	"path/filepath"
)

// LogCfg 定义日志组件配置。
// 包含日志级别、输出端、轮转和异步写入等选项。
type LogCfg struct {
	// LogPath 指定基于文件的日志记录的目标日志文件路径。
	// 支持相对和绝对路径以及自动目录创建。
	LogPath string `mapstructure:"path"`

	// LogLevel 定义过滤日志条目的最低日志级别。
	// 支持热重载，无需重启服务，实现动态日志级别调整。
	// 有效级别：跟踪、调试、信息、警告、错误、致命。
	LogLevel Level `mapstructure:"level"`

	// FileSplitMB 确定文件循环阈值（以兆字节为单位）。
	// 当日志文件超过此大小时，自动轮换会创建新文件。
	// 支持热重载，用于运行时调整轮换策略。
	FileSplitMB int `mapstructure:"splitMB"`

	// FileSplitHour 指定按时间轮转的触发小时（0-23）。
	FileSplitHour int `mapstructure:"splitHour"`

	// IsAsync 启用异步日志写入以防止 I/O 阻塞。
	// 建议用于高吞吐量游戏服务器以保持低延迟。
	IsAsync bool `mapstructure:"isAsync"`

	// AsyncCacheSize 限制异步模式下的最大缓冲日志条目。
	// 防止流量高峰或 I/O 减速期间内存溢出。
	// 默认值：启用异步模式时为 1024 个条目。
	AsyncCacheSize int `mapstructure:"asyncCacheSize"`

	// AsyncWriteMillSec 定义异步写入间隔（以毫秒为单位）。
	// 平衡写入延迟和批处理效率以获得最佳性能。
	// 默认值：200ms，用于在响应能力和吞吐量之间进行合理权衡。
	AsyncWriteMillSec int `mapstructure:"asyncWriteMillSec"`

	// LevelChangeMin 启用动态最小日志级别调整。
	// 允许更改运行时日志级别以进行调试或性能调整。
	LevelChangeMin int `mapstructure:"levelChangeMin"`

	// CallerSkip 指定采集调用方信息时要跳过的栈帧数。
	CallerSkip int `mapstructure:"callerSkip"`

	// FileAppender 支持基于文件的日志记录输出。
	// 用于持久存储和分析的主要日志记录目的地。
	FileAppender bool `mapstructure:"fileAppender"`

	// ConsoleAppender 启用控制台（stdout）日志输出。
	// 方便开发和容器化环境。
	ConsoleAppender bool `mapstructure:"consoleAppender"`

	// LevelChange 可以对特定代码位置进行细粒度的日志级别控制。
	// 允许在运行时调整日志记录详细程度，而无需重新启动服务。
	// 每个条目将文件路径和行号映射到特定的日志级别。
	// 专为调试生产中的关键游戏服务器组件而设计。
	LevelChange []LevelChangeEntry `mapstructure:"levelChange"`

	// ActorWhiteList 定义绕过日志级别过滤的 Actor ID 列表。
	// 为特定的高优先级玩家或测试帐户启用有针对性的调试。
	// 支持热重载以动态添加/删除调试目标。
	// 示例：[123456789、987654321、555666777]。
	ActorWhiteList []uint64 `mapstructure:"actorWhiteList"`

	// actorWhiteListSet 是用于 O(1) 白名单查找的内部缓存。
	// 在配置初始化期间从 ActorWhiteList 自动填充。
	// 不适合直接配置 - 请改用 ActorWhiteList。
	actorWhiteListSet map[uint64]struct{} `mapstructure:"-"`

	// ActorFileLog 允许记录到特定于Actor的日志文件。
	// 启用后，ActorLogger 将输出到原始日志文件和特定于Actor的文件。
	// 禁用时，ActorLogger 将仅输出到原始日志文件。
	ActorFileLog bool `mapstructure:"actorFileLog"`

	EnabledCallerInfo bool `mapstructure:"enabledCallerInfo"`
}

// Validate 校验日志配置的正确性与一致性。
func (cfg *LogCfg) Validate() error {
	// 验证日志级别。
	if cfg.LogLevel < TraceLevel || cfg.LogLevel > FatalLevel {
		return fmt.Errorf("invalid log level: %d, must be between %d (Trace) and %d (Fatal)",
			cfg.LogLevel, TraceLevel, FatalLevel)
	}

	// 验证文件分割大小。
	if cfg.FileSplitMB < 1 || cfg.FileSplitMB > 1024 {
		return fmt.Errorf("file split size must be between 1MB and 1024MB, got %dMB", cfg.FileSplitMB)
	}

	// 验证文件分割时间。
	if cfg.FileSplitHour < 0 || cfg.FileSplitHour > 23 {
		return fmt.Errorf("file split hour must be between 0 and 23, got %d", cfg.FileSplitHour)
	}

	// 验证异步缓存大小。
	if cfg.IsAsync && cfg.AsyncCacheSize < 1 {
		return fmt.Errorf("async cache size must be at least 1 when async mode is enabled, got %d", cfg.AsyncCacheSize)
	}

	// 验证异步写入间隔。
	if cfg.IsAsync && cfg.AsyncWriteMillSec < 10 {
		return fmt.Errorf("async write interval must be at least 10ms, got %dms", cfg.AsyncWriteMillSec)
	}

	// 校验 callerSkip。
	if cfg.CallerSkip < 0 {
		return fmt.Errorf("caller skip must be non-negative, got %d", cfg.CallerSkip)
	}

	// 校验文件 Appender 的日志路径。
	if cfg.FileAppender && cfg.LogPath == "" {
		return fmt.Errorf("log path cannot be empty when file appender is enabled")
	}

	// 验证日志路径格式。
	if cfg.FileAppender && cfg.LogPath != "" {
		if !filepath.IsAbs(cfg.LogPath) && filepath.IsAbs(filepath.Clean(cfg.LogPath)) {
			// 这是相对路径，清理后将变为绝对路径。
			// 我们要确保使用干净的路径。
			cfg.LogPath = filepath.Clean(cfg.LogPath)
		}
	}

	// 至少启用一个输出端。
	if !cfg.FileAppender && !cfg.ConsoleAppender {
		return fmt.Errorf("at least one appender (file or console) must be enabled")
	}

	// 重建白名单集以保持一致性。
	if len(cfg.ActorWhiteList) > 0 {
		cfg.actorWhiteListSet = make(map[uint64]struct{}, len(cfg.ActorWhiteList))
		for _, id := range cfg.ActorWhiteList {
			cfg.actorWhiteListSet[id] = struct{}{}
		}
	} else {
		cfg.actorWhiteListSet = nil
	}

	return nil
}

// IsInWhiteList 判断 Actor 是否在白名单中，复杂度 O(1)。
func (cfg *LogCfg) IsInWhiteList(actorID uint64) bool {
	if len(cfg.actorWhiteListSet) == 0 && len(cfg.ActorWhiteList) != 0 {
		cfg.actorWhiteListSet = make(map[uint64]struct{}, len(cfg.ActorWhiteList))
		for _, id := range cfg.ActorWhiteList {
			cfg.actorWhiteListSet[id] = struct{}{}
		}
	}

	_, exists := cfg.actorWhiteListSet[actorID]
	return exists
}

var _defaultCfg = &LogCfg{
	LogPath:           "./strix.log",
	LogLevel:          DebugLevel, // 默认日志级别。
	FileSplitMB:       50,
	FileSplitHour:     0,
	IsAsync:           true,
	CallerSkip:        1,
	FileAppender:      true,
	ConsoleAppender:   true,
	EnabledCallerInfo: true,
}

func getDefaultCfg() *LogCfg {
	return _defaultCfg
}
