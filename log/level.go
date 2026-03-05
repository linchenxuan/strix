package log

import "strings"

// 级别定义了分布式游戏服务器中结构化日志记录的综合日志记录级别。
// 级别按严重性排序，值越高表示问题越严重。
// 这种类型可以在运行时进行细粒度的日志过滤和动态级别调整。
type Level int8

// 用于基于严重性的结构化日志过滤的日志记录级别常量。
// 数值越高表示临界级别越严格，输出过滤越严格。
const (
	// TraceLevel 为深度调试提供极其详细的诊断信息。
	// 适用于跟踪请求流、性能分析和详细的系统状态。
	TraceLevel Level = iota + 1

	// DebugLevel 包含在开发和故障排除过程中有用的调试信息。
	// 包括变量状态、函数入口/出口点和条件分支。
	DebugLevel

	// InfoLevel 包含有关正常应用程序操作的一般信息消息。
	// 跟踪重要的业务事件、服务生命周期和配置更改。
	InfoLevel

	// 警告级别表示不妨碍操作的潜在有害情况。
	// 表示已弃用的用法、配置问题或可恢复的错误。
	WarnLevel

	// ErrorLevel 表示需要立即关注的严重问题。
	// 记录不可恢复的错误、失败的操作和系统不一致。
	ErrorLevel

	// FatalLevel 表示强制应用程序立即终止的严重错误。
	// 用于不可恢复的系统故障、数据损坏或安全漏洞。
	FatalLevel
)

// String 返回日志级别的人类可读字符串表示形式。
// 提供与行业标准和配置解析兼容的大写级别名称。
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

// ParseLevel 通过不区分大小写的解析将字符串表示形式转换为 Level 枚举值。
// 返回无效输入的 InfoLevel，确保配置场景中的安全默认值。
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
	return InfoLevel // 对于无法识别的输入，默认为 InfoLevel。
}
