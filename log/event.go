package log

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"time"
)

// Define context keys for trace information
type traceContextKey string

const (
	// TraceIDKey is the key for trace ID in context
	TraceIDKey traceContextKey = "trace_id"
	// SpanIDKey is the key for span ID in context
	SpanIDKey traceContextKey = "span_id"
	// ParentSpanIDKey is the key for parent span ID in context
	ParentSpanIDKey traceContextKey = "parent_span_id"
)

// LogEvent represents a single structured logging event.
// It provides a fluent API for adding key-value pairs to log entries
// and handles the complete lifecycle of a log message from creation to output.
type LogEvent struct {
	buf    *bytes.Buffer // Buffer for accumulating the formatted log data
	logger Logger        // Reference to the parent logger for routing and configuration
	level  Level         // Severity level of the log event
}

// newEvent creates a new LogEvent instance with pre-allocated buffer.
// This function is used internally by the logger to obtain reusable event objects
// from the object pool, ensuring zero-allocation performance characteristics.
func newEvent(l Logger) *LogEvent {
	e := &LogEvent{
		logger: l,
		level:  DebugLevel,
	}

	// Initialize buffer if not already present
	if e.buf == nil {
		e.buf = &bytes.Buffer{}
	}

	// Pre-allocate buffer space to minimize memory allocations during logging
	if e.buf.Cap() == 0 {
		e.buf.Grow(1024) // Pre-allocate 1KB space to reduce allocations during formatting
	}
	return e
}

// Reset prepares the LogEvent for reuse by clearing all previous state and reinitializing necessary components.
// This method is critical for maintaining the object pool's efficiency by ensuring that log events can be
// reused without retaining any residual data from previous logging operations. Key steps include:
//  1. Resetting the internal buffer to clear all accumulated data.
//  2. Resetting the log level to DebugLevel as the default for new log entries.
//  3. Optimizing buffer capacity - if the buffer has grown beyond 4KB, it is resized to 1KB to prevent
//     excessive memory usage while still providing adequate space for most log entries.
//  4. Appending the begin marker to prepare the buffer for new log data.
//
// This method is typically called when returning events to the object pool, ensuring a clean state
// for subsequent logging operations and preventing data leakage between different log events.
func (e *LogEvent) Reset() {
	e.buf.Reset()
	e.level = DebugLevel

	if e.buf.Cap() > 4096 {
		e.buf.Grow(1024)
	}

	AppendBeginMarker(e.buf)

}

// Time appends the provided time value to the log event's buffer in the format 'YYYY-MM-DD HH:MM:SS.000'.
// The format uses Go's reference time layout (2006-01-02 15:04:05.000) to ensure consistent timestamping.
// This method provides standardized time formatting for log entries across the application.
// It returns the LogEvent instance to support method chaining.
func (e *LogEvent) Time(k string, v *time.Time) *LogEvent {
	if e == nil {
		return nil
	}

	// 写入key（复用原有逻辑）
	AppendKey(e.buf, k)

	// 直接获取时间字段，避免Date()和Clock()的二次解析
	y := v.Year()
	mo := int(v.Month()) // Month是1-12的int类型
	d := v.Day()
	h := v.Hour()
	m := v.Minute()
	s := v.Second()
	ms := v.Nanosecond() / 1000000 // 毫秒

	// 时间格式固定为23字节（YYYY-MM-DD HH:MM:SS.000）
	const timeLen = 23
	var timeBuf [timeLen]byte // 栈上临时数组，零分配

	// 填充时间字段到临时数组（一次分配，无函数调用开销）
	// 年份（0-3）
	timeBuf[0] = byte('0' + y/1000)
	timeBuf[1] = byte('0' + (y/100)%10)
	timeBuf[2] = byte('0' + (y/10)%10)
	timeBuf[3] = byte('0' + y%10)
	// 分隔符（4）
	timeBuf[4] = '-'
	// 月份（5-6）
	timeBuf[5] = byte('0' + mo/10)
	timeBuf[6] = byte('0' + mo%10)
	// 分隔符（7）
	timeBuf[7] = '-'
	// 日期（8-9）
	timeBuf[8] = byte('0' + d/10)
	timeBuf[9] = byte('0' + d%10)
	// 空格（10）
	timeBuf[10] = ' '
	// 小时（11-12）
	timeBuf[11] = byte('0' + h/10)
	timeBuf[12] = byte('0' + h%10)
	// 分隔符（13）
	timeBuf[13] = ':'
	// 分钟（14-15）
	timeBuf[14] = byte('0' + m/10)
	timeBuf[15] = byte('0' + m%10)
	// 分隔符（16）
	timeBuf[16] = ':'
	// 秒（17-18）
	timeBuf[17] = byte('0' + s/10)
	timeBuf[18] = byte('0' + s%10)
	// 分隔符（19）
	timeBuf[19] = '.'
	// 毫秒（20-22）
	timeBuf[20] = byte('0' + ms/100)
	timeBuf[21] = byte('0' + (ms/10)%10)
	timeBuf[22] = byte('0' + ms%10)

	// 一次性写入所有时间字节（1次Write调用替代24次WriteByte）
	e.buf.WriteByte('"')
	e.buf.Write(timeBuf[:])
	e.buf.WriteByte('"')

	return e
}

// Int appends an integer value with the specified key to the log event buffer.
// This method follows a consistent pattern for appending key-value pairs to log entries.
// It first appends the key using AppendKey, then the integer value using AppendInt.
// Returns the LogEvent instance to support method chaining.
func (e *LogEvent) Int(k string, v int) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInt(e.buf, v)

	return e
}

// Ints appends an array of integer values with the specified key to the log event buffer.
// This method handles the formatting and appending of array data for structured logging.
// It first appends the key using AppendKey, then the array of integers using AppendInts.
// Returns the LogEvent instance to support method chaining.
func (e *LogEvent) Ints(k string, v []int) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInts(e.buf, v)

	return e
}

// Context adds trace information from the provided context to the log event.
// This method extracts trace_id, span_id, and parent_span_id from the context
// and adds them as structured fields to the log event.
// Returns the LogEvent instance to support method chaining.
func (e *LogEvent) Context(ctx context.Context) *LogEvent {
	if e == nil || ctx == nil {
		return e
	}

	// Extract trace information from context
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok && traceID != "" {
		AppendKey(e.buf, "trace_id")
		e.buf.WriteString(`"` + traceID + `"`)
	}

	if spanID, ok := ctx.Value(SpanIDKey).(string); ok && spanID != "" {
		AppendKey(e.buf, "span_id")
		e.buf.WriteString(`"` + spanID + `"`)
	}

	if parentSpanID, ok := ctx.Value(ParentSpanIDKey).(string); ok && parentSpanID != "" {
		AppendKey(e.buf, "parent_span_id")
		e.buf.WriteString(`"` + parentSpanID + `"`)
	}

	return e
}

// Int8 appends an int8 value with the specified key to the log event buffer.
// This method provides type-specific handling for 8-bit integer values.
// It first appends the key using AppendKey, then the int8 value using AppendInt8.
// Returns the LogEvent instance to support method chaining.
func (e *LogEvent) Int8(k string, v int8) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInt8(e.buf, v)
	return e
}

// Uint8s appends an array of uint8 values with the specified key to the log event buffer.
// This method handles the formatting and appending of unsigned 8-bit integer array data.
// It first appends the key using AppendKey, then the array of uint8 values using AppendUint8s.
// Returns the LogEvent instance to support method chaining.
func (e *LogEvent) Uint8s(k string, v []uint8) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendUint8s(e.buf, v)
	return e
}

// Uint8 appends a uint8 value with the specified key to the log event buffer.
// This method provides type-specific handling for unsigned 8-bit integer values.
// It first appends the key using AppendKey, then the uint8 value using AppendUint8.
// Returns the LogEvent instance to support method chaining.
func (e *LogEvent) Uint8(k string, v uint8) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendUint8(e.buf, v)
	return e
}

// Int8s appends an array of int8 values with the specified key to the log event buffer.
// This method handles the formatting and appending of signed 8-bit integer array data.
// It first appends the key using AppendKey, then the array of int8 values using AppendInt8s.
// Returns the LogEvent instance to support method chaining.
func (e *LogEvent) Int8s(k string, v []int8) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInt8s(e.buf, v)
	return e
}

// Uint16 appends a uint16 value with the specified key to the log event buffer.
// This method provides type-specific handling for unsigned 16-bit integer values.
// It first appends the key using AppendKey, then the uint16 value using AppendUint16.
// Returns the LogEvent instance to support method chaining.
func (e *LogEvent) Uint16(k string, v uint16) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendUint16(e.buf, v)
	return e
}

// Uint16s prints uint16 array.
func (e *LogEvent) Uint16s(k string, v []uint16) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendUint16s(e.buf, v)
	return e
}

// Int32 prints int32 value.
func (e *LogEvent) Int32(k string, v int32) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInt32(e.buf, v)
	return e
}

// Int32s prints int32 array.
func (e *LogEvent) Int32s(k string, v []int32) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInt32s(e.buf, v)
	return e
}

// Uint32 prints uint32 value.
func (e *LogEvent) Uint32(k string, v uint32) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendUint32(e.buf, v)
	return e
}

// Uint32s prints uint32 array.
func (e *LogEvent) Uint32s(k string, v []uint32) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendUint32s(e.buf, v)
	return e
}

// Int64 prints int64 value.
func (e *LogEvent) Int64(k string, v int64) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInt64(e.buf, v)
	return e
}

// Int64s prints int64 array.
func (e *LogEvent) Int64s(k string, v []int64) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInt64s(e.buf, v)
	return e
}

// Uint64 prints uint64 value.
func (e *LogEvent) Uint64(k string, v uint64) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)

	AppendUint64(e.buf, v)

	return e
}

// Uint64s prints uint64 array.
func (e *LogEvent) Uint64s(k string, v []uint64) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendUint64s(e.buf, v)
	return e
}

// Float64 prints float64 value.
func (e *LogEvent) Float64(k string, v float64) *LogEvent {
	if e == nil {
		return nil
	}
	AppendKey(e.buf, k)
	AppendFloat64(e.buf, v)
	return e
}

// Float64s prints float64 array.
func (e *LogEvent) Float64s(k string, v []float64) *LogEvent {
	if e == nil {
		return nil
	}
	AppendKey(e.buf, k)
	AppendFloat64s(e.buf, v)
	return e
}

// Float32 prints float32 value.
func (e *LogEvent) Float32(k string, v float32) *LogEvent {
	if e == nil {
		return nil
	}
	AppendKey(e.buf, k)
	AppendFloat32(e.buf, v)
	return e
}

// Float32s prints float32 array.
func (e *LogEvent) Float32s(k string, v []float32) *LogEvent {
	if e == nil {
		return nil
	}
	AppendKey(e.buf, k)
	AppendFloat32s(e.buf, v)
	return e
}

// Bool prints bool value.
func (e *LogEvent) Bool(k string, v bool) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendBool(e.buf, v)
	return e
}

// Bools prints bool array.
func (e *LogEvent) Bools(k string, v []bool) *LogEvent {
	if e == nil {
		return nil
	}
	AppendKey(e.buf, k)
	AppendBools(e.buf, v)
	return e
}

// Caller adds caller information to the log event.
// This includes function name, file path, and line number for debugging purposes.
func (e *LogEvent) Caller(file string, function string, line int) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, "caller")
	e.buf.WriteByte('"')
	e.buf.WriteString(file)
	e.buf.WriteString(".")
	e.buf.WriteString(function)
	e.buf.WriteByte(':')
	e.buf.WriteString(strconv.Itoa(line))
	e.buf.WriteByte('"')

	return e
}

// Str prints string value.
func (e *LogEvent) Str(k string, s string) *LogEvent {
	if e == nil {
		return nil
	}
	AppendKey(e.buf, k)
	AppendString(e.buf, s)
	return e
}

// Strs prints string array.
func (e *LogEvent) Strs(k string, v []string) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendStrings(e.buf, v)
	return e
}

// Err prints error value.
// It safely handles nil errors by logging them as null values.
func (e *LogEvent) Err(v error) *LogEvent {
	if e == nil {
		return nil
	}
	AppendKey(e.buf, "error")
	if v != nil {
		AppendString(e.buf, v.Error())
	} else {
		AppendNil(e.buf)
	}
	return e
}

// Errs prints error list.
// It logs multiple errors as an array, handling nil values appropriately.
func (e *LogEvent) Errs(v []error) *LogEvent {
	for _, err := range v {
		_ = e.Err(err)
	}
	return e
}

// LogObjectMarshaler allows custom objects to implement their own JSON serialization.
// Objects implementing this interface can be passed directly to the logging API
// and will control their own JSON representation for maximum flexibility.
type LogObjectMarshaler interface {
	MarshalLogObj(e *LogEvent)
}

// Obj prints custom object that implements LogObjectMarshaler interface.
// It provides a way to log complex objects with custom serialization logic.
func (e *LogEvent) Obj(k string, v LogObjectMarshaler) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	if v == nil {
		AppendNil(e.buf)
	} else {
		v.MarshalLogObj(e)
	}
	return e
}

func (e *LogEvent) Any(k string, v any) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)

	b, err := json.Marshal(v)
	if err != nil {
		AppendString(e.buf, err.Error())
	} else {
		AppendString(e.buf, string(b))
	}

	return e
}

// Msg adds the final message to the log event and triggers output.
// This is the terminal method in the fluent API chain that completes the log entry
// and sends it to all configured appenders for output.
func (e *LogEvent) Msg(v string) {
	if e == nil {
		return
	}
	e.Str("msg", v)
	e.End()
}

// End finalizes the log event and sends it to all configured appenders.
// This method is typically called automatically by Msg() but can be used
// directly for custom message formatting scenarios.
func (e *LogEvent) End() {
	if e == nil {
		return
	}

	AppendEndMarker(e.buf)

	AppendLineBreak(e.buf)

	// Return the event to the object pool for reuse
	e.logger.OnEventEnd(e)
}
