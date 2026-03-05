package log

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"time"
)

// 定义跟踪信息的上下文键。
type traceContextKey string

const (
	// TraceIDKey 是上下文中跟踪 ID 的键。
	TraceIDKey traceContextKey = "trace_id"
	// SpanIDKey 是上下文中 Span ID 的键。
	SpanIDKey traceContextKey = "span_id"
	// ParentSpanIDKey 是上下文中父级 Span ID 的键。
	ParentSpanIDKey traceContextKey = "parent_span_id"
)

// LogEvent 表示单个结构化日志记录事件。
// 它提供了一个流畅的 API，用于将键值对添加到日志条目中。
// 并处理日志消息从创建到输出的完整生命周期。
type LogEvent struct {
	buf    *bytes.Buffer // 用于累积格式化日志数据的缓冲区。
	logger Logger        // 引用父日志器，用于路由和配置。
	level  Level         // 日志事件的严重级别。
}

// newEvent 创建 LogEvent 并预分配缓冲区。
// 该函数仅供日志器内部复用事件对象时使用。
func newEvent(l Logger) *LogEvent {
	e := &LogEvent{
		logger: l,
		level:  DebugLevel,
	}

	// 初始化缓冲区（如尚未创建）。
	if e.buf == nil {
		e.buf = &bytes.Buffer{}
	}

	// 预分配缓冲区空间，减少日志格式化时的分配。
	if e.buf.Cap() == 0 {
		e.buf.Grow(1024) // 预分配1KB空间以减少格式化时的分配。
	}
	return e
}

// Reset 通过清除所有以前的状态并重新初始化必要的组件来准备 LogEvent 以供重用。
// 该方法通过确保日志事件可以被记录下来，对于维持对象池的效率至关重要。
// 重复使用，不保留以前记录操作的任何残留数据。关键步骤包括：
// 1. 重置内部缓冲区以清除所有累积数据。
// 2. 将日志级别重置为 DebugLevel 作为新日志条目的默认值。
// 3. 优化缓冲区容量 - 如果缓冲区增长超过 4KB，则将其大小调整为 1KB 以防止。
// 内存使用过多，同时仍为大多数日志条目提供足够的空间。
// 4. 附加开始标记以为新日志数据准备缓冲区。
//
// 该方法通常在将事件返回到对象池时调用，以确保干净的状态。
// 用于后续的日志操作并防止不同日志事件之间的数据泄漏。
func (e *LogEvent) Reset() {
	e.buf.Reset()
	e.level = DebugLevel

	if e.buf.Cap() > 4096 {
		e.buf.Grow(1024)
	}

	AppendBeginMarker(e.buf)

}

// Time 将提供的时间值以“YYYY-MM-DD HH:MM:SS.000”格式附加到日志事件的缓冲区。
// 该格式使用 Go 的参考时间布局 (2006-01-02 15:04:05.000) 以确保时间戳一致。
// 此方法为整个应用程序中的日志条目提供标准化的时间格式。
// 它返回 LogEvent 实例以支持方法链接。
func (e *LogEvent) Time(k string, v *time.Time) *LogEvent {
	if e == nil {
		return nil
	}

	// 写入key（复用原有逻辑）
	AppendKey(e.buf, k)

	// 直接获取时间字段，避免Date()和Clock()的二次解析。
	y := v.Year()
	mo := int(v.Month()) // Month是1-12的int类型。
	d := v.Day()
	h := v.Hour()
	m := v.Minute()
	s := v.Second()
	ms := v.Nanosecond() / 1000000 // 毫秒。

	// 时间格式固定为23字节（YYYY-MM-DD HH:MM:SS.000）
	const timeLen = 23
	var timeBuf [timeLen]byte // 栈上临时数组，零分配。

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

// Int 将具有指定键的整数值附加到日志事件缓冲区。
// 此方法遵循将键值对附加到日志条目的一致模式。
// 它首先使用 AppendKey 附加键，然后使用 AppendInt 附加整数值。
// 返回 LogEvent 实例以支持方法链接。
func (e *LogEvent) Int(k string, v int) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInt(e.buf, v)

	return e
}

// Ints 将具有指定键的整数值数组附加到日志事件缓冲区。
// 此方法处理数组数据的格式化和附加以进行结构化日志记录。
// 它首先使用 AppendKey 附加键，然后使用 AppendInts 附加整数数组。
// 返回 LogEvent 实例以支持方法链接。
func (e *LogEvent) Ints(k string, v []int) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInts(e.buf, v)

	return e
}

// Context 从 context 中提取链路字段并写入日志事件。
// 支持 `trace_id`、`span_id`、`parent_span_id`。
func (e *LogEvent) Context(ctx context.Context) *LogEvent {
	if e == nil || ctx == nil {
		return e
	}

	// 从上下文中提取跟踪信息。
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

// Int8 将具有指定键的 int8 值附加到日志事件缓冲区。
// 此方法为 8 位整数值提供特定于类型的处理。
// 它首先使用 AppendKey 附加键，然后使用 AppendInt8 附加 int8 值。
// 返回 LogEvent 实例以支持方法链接。
func (e *LogEvent) Int8(k string, v int8) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInt8(e.buf, v)
	return e
}

// Uint8s 将具有指定键的 uint8 值数组附加到日志事件缓冲区。
// 此方法处理无符号 8 位整数数组数据的格式化和附加。
// 它首先使用 AppendKey 附加键，然后使用 AppendUint8s 附加 uint8 值数组。
// 返回 LogEvent 实例以支持方法链接。
func (e *LogEvent) Uint8s(k string, v []uint8) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendUint8s(e.buf, v)
	return e
}

// Uint8 将具有指定键的 uint8 值附加到日志事件缓冲区。
// 此方法为无符号 8 位整数值提供特定于类型的处理。
// 它首先使用 AppendKey 附加键，然后使用 AppendUint8 附加 uint8 值。
// 返回 LogEvent 实例以支持方法链接。
func (e *LogEvent) Uint8(k string, v uint8) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendUint8(e.buf, v)
	return e
}

// Int8s 将具有指定键的 int8 值数组附加到日志事件缓冲区。
// 此方法处理有符号 8 位整数数组数据的格式化和附加。
// 它首先使用 AppendKey 附加键，然后使用 AppendInt8s 附加 int8 值数组。
// 返回 LogEvent 实例以支持方法链接。
func (e *LogEvent) Int8s(k string, v []int8) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInt8s(e.buf, v)
	return e
}

// Uint16 将具有指定键的 uint16 值附加到日志事件缓冲区。
// 此方法为无符号 16 位整数值提供特定于类型的处理。
// 它首先使用 AppendKey 附加键，然后使用 AppendUint16 附加 uint16 值。
// 返回 LogEvent 实例以支持方法链接。
func (e *LogEvent) Uint16(k string, v uint16) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendUint16(e.buf, v)
	return e
}

// uint16s 打印 uint16 数组。
func (e *LogEvent) Uint16s(k string, v []uint16) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendUint16s(e.buf, v)
	return e
}

// Int32 打印 int32 值。
func (e *LogEvent) Int32(k string, v int32) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInt32(e.buf, v)
	return e
}

// Int32s 打印 int32 数组。
func (e *LogEvent) Int32s(k string, v []int32) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInt32s(e.buf, v)
	return e
}

// Uint32 打印 uint32 值。
func (e *LogEvent) Uint32(k string, v uint32) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendUint32(e.buf, v)
	return e
}

// uint32s 打印 uint32 数组。
func (e *LogEvent) Uint32s(k string, v []uint32) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendUint32s(e.buf, v)
	return e
}

// Int64 打印 int64 值。
func (e *LogEvent) Int64(k string, v int64) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInt64(e.buf, v)
	return e
}

// Int64s 打印 int64 数组。
func (e *LogEvent) Int64s(k string, v []int64) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendInt64s(e.buf, v)
	return e
}

// Uint64 打印 uint64 值。
func (e *LogEvent) Uint64(k string, v uint64) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)

	AppendUint64(e.buf, v)

	return e
}

// uint64s 打印 uint64 数组。
func (e *LogEvent) Uint64s(k string, v []uint64) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendUint64s(e.buf, v)
	return e
}

// Float64 打印 float64 值。
func (e *LogEvent) Float64(k string, v float64) *LogEvent {
	if e == nil {
		return nil
	}
	AppendKey(e.buf, k)
	AppendFloat64(e.buf, v)
	return e
}

// Float64s 打印 float64 数组。
func (e *LogEvent) Float64s(k string, v []float64) *LogEvent {
	if e == nil {
		return nil
	}
	AppendKey(e.buf, k)
	AppendFloat64s(e.buf, v)
	return e
}

// Float32 打印 float32 值。
func (e *LogEvent) Float32(k string, v float32) *LogEvent {
	if e == nil {
		return nil
	}
	AppendKey(e.buf, k)
	AppendFloat32(e.buf, v)
	return e
}

// Float32s 打印 float32 数组。
func (e *LogEvent) Float32s(k string, v []float32) *LogEvent {
	if e == nil {
		return nil
	}
	AppendKey(e.buf, k)
	AppendFloat32s(e.buf, v)
	return e
}

// Bool 打印布尔值。
func (e *LogEvent) Bool(k string, v bool) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendBool(e.buf, v)
	return e
}

// Bools 打印布尔数组。
func (e *LogEvent) Bools(k string, v []bool) *LogEvent {
	if e == nil {
		return nil
	}
	AppendKey(e.buf, k)
	AppendBools(e.buf, v)
	return e
}

// Caller 将调用方文件、函数和行号写入日志事件。
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

// Str 写入字符串字段。
func (e *LogEvent) Str(k string, s string) *LogEvent {
	if e == nil {
		return nil
	}
	AppendKey(e.buf, k)
	AppendString(e.buf, s)
	return e
}

// Strs 写入字符串数组字段。
func (e *LogEvent) Strs(k string, v []string) *LogEvent {
	if e == nil {
		return nil
	}

	AppendKey(e.buf, k)
	AppendStrings(e.buf, v)
	return e
}

// Err 写入 error 字段；当 v 为 nil 时写入 null。
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

// Errs 依次写入多个错误。
func (e *LogEvent) Errs(v []error) *LogEvent {
	for _, err := range v {
		_ = e.Err(err)
	}
	return e
}

// LogObjectMarshaler 允许自定义对象实现自己的 JSON 序列化。
// 实现此接口的对象可以直接传递给日志记录 API。
// 并将控制自己的 JSON 表示以获得最大的灵活性。
type LogObjectMarshaler interface {
	MarshalLogObj(e *LogEvent)
}

// Obj 写入实现 LogObjectMarshaler 的对象字段。
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

// Msg 写入 `msg` 字段并结束事件。
func (e *LogEvent) Msg(v string) {
	if e == nil {
		return
	}
	e.Str("msg", v)
	e.End()
}

// End 结束日志事件并分发到所有 Appender。
func (e *LogEvent) End() {
	if e == nil {
		return
	}

	AppendEndMarker(e.buf)

	AppendLineBreak(e.buf)

	// 将事件返回到对象池以供复用。
	e.logger.OnEventEnd(e)
}
