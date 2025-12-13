package log

import "strings"

// Level defines comprehensive logging levels for structured logging in distributed game servers.
// Levels are ordered by severity, with higher values indicating more critical issues.
// This type enables fine-grained log filtering and dynamic level adjustment at runtime.
type Level int8

// Logging level constants for structured severity-based log filtering.
// Higher numeric values indicate more critical levels with stricter output filtering.
const (
	// TraceLevel provides extremely detailed diagnostic information for deep debugging.
	// Suitable for tracing request flows, performance profiling, and detailed system state.
	TraceLevel Level = iota + 1

	// DebugLevel contains debugging information useful during development and troubleshooting.
	// Includes variable states, function entry/exit points, and conditional branches.
	DebugLevel

	// InfoLevel contains general informational messages about normal application operation.
	// Tracks significant business events, service lifecycle, and configuration changes.
	InfoLevel

	// WarnLevel indicates potentially harmful situations that don't prevent operation.
	// Signals deprecated usage, configuration issues, or recoverable errors.
	WarnLevel

	// ErrorLevel indicates serious problems that require immediate attention.
	// Logs unrecoverable errors, failed operations, and system inconsistencies.
	ErrorLevel

	// FatalLevel represents critical errors that force immediate application termination.
	// Used for unrecoverable system failures, data corruption, or security breaches.
	FatalLevel
)

// String returns the human-readable string representation of the log level.
// Provides uppercase level names compatible with industry standards and configuration parsing.
func (l Level) String() string {
	switch l {
	case TraceLevel:
		return "TRACE"
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	case FatalLevel:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel converts string representation to Level enum value with case-insensitive parsing.
// Returns InfoLevel for invalid inputs, ensuring safe defaults in configuration scenarios.
func ParseLevel(levelStr string) Level {
	switch strings.ToUpper(levelStr) {
	case "TRACE":
		return TraceLevel
	case "DEBUG":
		return DebugLevel
	case "INFO":
		return InfoLevel
	case "WARN":
		return WarnLevel
	case "ERROR":
		return ErrorLevel
	case "FATAL":
		return FatalLevel
	}
	return InfoLevel // Default to InfoLevel for unrecognized inputs
}
