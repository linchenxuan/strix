package log

import (
	"fmt"
	"path/filepath"
)

// LogCfg represents comprehensive logging configuration for high-performance game servers.
// It provides flexible configuration options for both synchronous and asynchronous logging,
// file rotation strategies, and output destinations suitable for production environments.
type LogCfg struct {
	// LogPath specifies the target log file path for file-based logging.
	// Supports relative and absolute paths with automatic directory creation.
	LogPath string `mapstructure:"path"`

	// LogLevel defines the minimum log level for filtering log entries.
	// Supports hot-reload without service restart for dynamic log level adjustment.
	// Valid levels: Trace, Debug, Info, Warn, Error, Fatal.
	LogLevel Level `mapstructure:"level"`

	// FileSplitMB determines the file rotation threshold in megabytes.
	// When log file exceeds this size, automatic rotation creates new files.
	// Supports hot-reload for runtime adjustment of rotation strategy.
	FileSplitMB int `mapstructure:"splitMB"`

	// FileSplitHour specifies the hour of day (0-23) for time-based file rotation.
	// Enables daily log rotation at specific times for operational convenience.
	FileSplitHour int `mapstructure:"splitHour"`

	// IsAsync enables asynchronous log writing to prevent I/O blocking.
	// Recommended for high-throughput game servers to maintain low latency.
	IsAsync bool `mapstructure:"isAsync"`

	// AsyncCacheSize limits the maximum buffered log entries in async mode.
	// Prevents memory overflow during traffic spikes or I/O slowdowns.
	// Default: 1024 entries when async mode is enabled.
	AsyncCacheSize int `mapstructure:"asyncCacheSize"`

	// AsyncWriteMillSec defines the async write interval in milliseconds.
	// Balances between write latency and batch efficiency for optimal performance.
	// Default: 200ms for reasonable trade-off between responsiveness and throughput.
	AsyncWriteMillSec int `mapstructure:"asyncWriteMillSec"`

	// LevelChangeMin enables dynamic minimum log level adjustment.
	// Allows runtime log level changes for debugging or performance tuning.
	LevelChangeMin int `mapstructure:"levelChangeMin"`

	// CallerSkip specifies the number of stack frames to skip for caller information.
	// Useful for wrapper functions or middleware layers in complex applications.
	CallerSkip int `mapstructure:"callerSkip"`

	// FileAppender enables file-based logging output.
	// Primary logging destination for persistent storage and analysis.
	FileAppender bool `mapstructure:"fileAppender"`

	// ConsoleAppender enables console (stdout) logging output.
	// Convenient for development and containerized environments.
	ConsoleAppender bool `mapstructure:"consoleAppender"`

	// LevelChange enables fine-grained log level control for specific code locations.
	// Allows runtime adjustment of logging verbosity without service restart.
	// Each entry maps a file path and line number to a specific log level.
	// Designed for debugging critical game server components in production.
	LevelChange []LevelChangeEntry `mapstructure:"levelChange"`

	// ActorWhiteList defines the list of player/actor IDs that bypass log level filtering.
	// Enables targeted debugging for specific high-priority players or test accounts.
	// Supports hot-reload for dynamic addition/removal of debug targets.
	// Example: [123456789, 987654321, 555666777]
	ActorWhiteList []uint64 `mapstructure:"actorWhiteList"`

	// actorWhiteListSet is an internal cache for O(1) whitelist lookups.
	// Populated automatically from ActorWhiteList during configuration initialization.
	// Not intended for direct configuration - use ActorWhiteList instead.
	actorWhiteListSet map[uint64]struct{} `mapstructure:"-"`

	// ActorFileLog enables logging to actor-specific log files.
	// When enabled, ActorLogger will output to both the original log file and actor-specific files.
	// When disabled, ActorLogger will only output to the original log file.
	ActorFileLog bool `mapstructure:"actorFileLog"`

	EnabledCallerInfo bool `mapstructure:"enabledCallerInfo"`
}

// Validate validates the logging configuration for correctness and consistency.
func (cfg *LogCfg) Validate() error {
	// Validate log level
	if cfg.LogLevel < TraceLevel || cfg.LogLevel > FatalLevel {
		return fmt.Errorf("invalid log level: %d, must be between %d (Trace) and %d (Fatal)",
			cfg.LogLevel, TraceLevel, FatalLevel)
	}

	// Validate file split size
	if cfg.FileSplitMB < 1 || cfg.FileSplitMB > 1024 {
		return fmt.Errorf("file split size must be between 1MB and 1024MB, got %dMB", cfg.FileSplitMB)
	}

	// Validate file split hour
	if cfg.FileSplitHour < 0 || cfg.FileSplitHour > 23 {
		return fmt.Errorf("file split hour must be between 0 and 23, got %d", cfg.FileSplitHour)
	}

	// Validate async cache size
	if cfg.IsAsync && cfg.AsyncCacheSize < 1 {
		return fmt.Errorf("async cache size must be at least 1 when async mode is enabled, got %d", cfg.AsyncCacheSize)
	}

	// Validate async write interval
	if cfg.IsAsync && cfg.AsyncWriteMillSec < 10 {
		return fmt.Errorf("async write interval must be at least 10ms, got %dms", cfg.AsyncWriteMillSec)
	}

	// Validate caller skip
	if cfg.CallerSkip < 0 {
		return fmt.Errorf("caller skip must be non-negative, got %d", cfg.CallerSkip)
	}

	// Validate log path for file appender
	if cfg.FileAppender && cfg.LogPath == "" {
		return fmt.Errorf("log path cannot be empty when file appender is enabled")
	}

	// Validate log path format
	if cfg.FileAppender && cfg.LogPath != "" {
		if !filepath.IsAbs(cfg.LogPath) && filepath.IsAbs(filepath.Clean(cfg.LogPath)) {
			// This is a relative path that would become absolute after cleaning
			// We want to ensure we're using a clean path
			cfg.LogPath = filepath.Clean(cfg.LogPath)
		}
	}

	// Validate that at least one appender is enabled
	if !cfg.FileAppender && !cfg.ConsoleAppender {
		return fmt.Errorf("at least one appender (file or console) must be enabled")
	}

	// Rebuild whitelist set for consistency
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

// IsInWhiteList checks if an actor ID exists in the whitelist with O(1) complexity.
// Returns true if the actor is whitelisted for unrestricted logging.
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
	LogLevel:          DebugLevel, // Default log level
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
