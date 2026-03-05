package log

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"unicode/utf8"
)

// AppendBeginMarker 将映射起始字符“{”插入目标缓冲区。
// 此函数初始化用于结构化日志记录的 JSON 对象结构。
// 它是一个零分配操作，直接将左大括号写入缓冲区。
//
// 参数：
// - buf：要附加开始标记的缓冲区
func AppendBeginMarker(buf *bytes.Buffer) {
	buf.WriteByte('{')
}

// AppendEndMarker 将映射结束字符“}”插入目标缓冲区。
// 此函数完成结构化日志记录的 JSON 对象结构。
// 这是一个零分配操作，直接将右大括号写入缓冲区。
//
// 参数：
// - buf：附加结束标记的缓冲区
func AppendEndMarker(buf *bytes.Buffer) {
	buf.WriteByte('}')
}

// AppendKey 将新键附加到具有正确格式的输出 JSON。
// 此函数处理 JSON 对象中键的插入，确保正确。
// 必要时与之前的键值对用逗号分隔。然后它附加。
// 键（正确转义）后跟冒号分隔符。
//
// 参数：
// - buf：将键附加到的缓冲区
// - key：要追加的字符串键
func AppendKey(buf *bytes.Buffer, key string) {
	if buf.Len() >= 1 && buf.Bytes()[buf.Len()-1] != '{' {
		buf.WriteByte(',')
	}
	AppendString(buf, key)
	buf.WriteByte(':')
}

// AppendNil 将 JSON“null”值插入到缓冲区中。
// 此函数提供了一种将空值附加到 JSON 结构的标准化方法。
// 在日志输出中，确保整个日志系统的格式一致。
//
// 参数：
// - buf：将空值附加到的缓冲区
func AppendNil(buf *bytes.Buffer) {
	buf.WriteString("null")
}

// AppendLineBreak 将换行符“\n”附加到缓冲区。
// 该函数用于通过插入换行符来提高日志的可读性。
// 日志条目之间或复杂日志消息的不同部分之间。
//
// 参数：
// - buf：附加换行符的缓冲区
func AppendLineBreak(buf *bytes.Buffer) {
	buf.WriteByte('\n')
}

// AppendArrayStart 添加 JSON 数组起始字符“[”来指示数组的开头。
// 此函数初始化用于结构化日志记录的 JSON 数组结构，允许。
// 将多个值组合到一个字段中。
//
// 参数：
// - buf：将数组起始字符附加到的缓冲区
func AppendArrayStart(buf *bytes.Buffer) {
	buf.WriteByte('[')
}

// AppendArrayEnd 添加 JSON 数组结束字符“]”以指示数组的完成。
// 该函数关闭之前启动的 JSON 数组结构，标记结束。
// 结构化日志记录中的一系列值。
//
// 参数：
// - buf：将数组结束字符附加到的缓冲区
func AppendArrayEnd(buf *bytes.Buffer) {
	buf.WriteByte(']')
}

// AppendArrayDelim 必要时在数组元素之间添加逗号分隔符“,”。
// 此函数通过仅在以下情况下插入逗号来确保正确的 JSON 数组格式。
// 缓冲区不为空，防止尾随逗号导致 JSON 无效。
//
// 参数：
// - buf：附加逗号分隔符的缓冲区
func AppendArrayDelim(buf *bytes.Buffer) {
	if buf.Len() > 0 {
		buf.WriteByte(',')
	}
}

// AppendBool 将布尔值转换为其 JSON 字符串表示形式并将其附加到缓冲区。
// 该函数有效地将布尔值转换为“true”或“false”并将其写入。
// 直接到提供的缓冲区，确保布尔日志值正确的 JSON 序列化。
//
// 参数：
// - buf：要附加布尔值的缓冲区
// - val：要转换和附加的布尔值
func AppendBool(buf *bytes.Buffer, val bool) {
	buf.Write(strconv.AppendBool(buf.Bytes(), val))
}

// AppendBools 将布尔值的 JSON 数组附加到缓冲区。
// 处理将 bool 切片转换为其 JSON 字符串表示形式。
// 正确格式化为具有“true”或“false”值的 JSON 数组。
//
// 参数：
// - buf：将格式化布尔数组附加到的缓冲区
// - bools：要转换和附加的布尔值切片
//
// 返回：
// - 修改后的缓冲区附加了布尔数组
func AppendBools(buf *bytes.Buffer, vals []bool) {
	if len(vals) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteByte('[')
	buf.Write(strconv.AppendBool(buf.Bytes(), vals[0]))
	for i := 1; i < len(vals); i++ {
		buf.WriteByte(',')
		buf.Write(strconv.AppendBool(buf.Bytes(), vals[i]))
	}
	buf.WriteByte(']')
}

// AppendInt 将 int 转换为字符串并附加到缓冲区。
func AppendInt(buf *bytes.Buffer, val int) {
	buf.WriteString(strconv.FormatInt(int64(val), 10))
}

// AppendInts 将 []int 编码为 JSON 数组。
func AppendInts(buf *bytes.Buffer, vals []int) {
	if len(vals) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteByte('[')
	strconv.AppendInt(buf.Bytes(), int64(vals[0]), 10)
	for i := 1; i < len(vals); i++ {
		buf.WriteByte(',')
		strconv.AppendInt(buf.Bytes(), int64(vals[i]), 10)
	}
	buf.WriteByte(']')
}

// AppendInt8 将 int8 转换为字符串并追加到缓冲区。
func AppendInt8(buf *bytes.Buffer, val int8) {
	buf.WriteString(strconv.FormatInt(int64(val), 10))
}

// AppendInt8s 将 []int8 编码为 JSON 数组。
func AppendInt8s(buf *bytes.Buffer, vals []int8) {
	if len(vals) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteByte('[')
	strconv.AppendInt(buf.Bytes(), int64(vals[0]), 10)
	for i := 1; i < len(vals); i++ {
		buf.WriteByte(',')
		strconv.AppendInt(buf.Bytes(), int64(vals[i]), 10)
	}
	buf.WriteByte(']')
}

// AppendInt16 将 int16 转换为字符串并追加到缓冲区。
func AppendInt16(buf *bytes.Buffer, val int16) {
	buf.WriteString(strconv.FormatInt(int64(val), 10))
}

// AppendInt16s 将 []int16 编码为 JSON 数组。
func AppendInt16s(buf *bytes.Buffer, vals []int16) {
	if len(vals) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteByte('[')
	strconv.AppendInt(buf.Bytes(), int64(vals[0]), 10)
	for i := 1; i < len(vals); i++ {
		buf.WriteByte(',')
		strconv.AppendInt(buf.Bytes(), int64(vals[i]), 10)
	}
	buf.WriteByte(']')
}

// AppendInt32 将 int32 转换为字符串并追加到缓冲区。
func AppendInt32(buf *bytes.Buffer, val int32) {
	buf.WriteString(strconv.FormatInt(int64(val), 10))
}

// AppendInt32s 将 []int32 编码为 JSON 数组。
func AppendInt32s(buf *bytes.Buffer, vals []int32) {
	if len(vals) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteByte('[')
	strconv.AppendInt(buf.Bytes(), int64(vals[0]), 10)
	for i := 1; i < len(vals); i++ {
		buf.WriteByte(',')
		strconv.AppendInt(buf.Bytes(), int64(vals[i]), 10)
	}
	buf.WriteByte(']')
}

// AppendInt64 将 int64 转换为字符串并追加到缓冲区。
func AppendInt64(buf *bytes.Buffer, val int64) {
	buf.WriteString(strconv.FormatInt(val, 10))
}

// AppendInt64s 将 []int64 编码为 JSON 数组。
func AppendInt64s(buf *bytes.Buffer, vals []int64) {
	if len(vals) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteByte('[')
	strconv.AppendInt(buf.Bytes(), vals[0], 10)
	for i := 1; i < len(vals); i++ {
		buf.WriteByte(',')
		strconv.AppendInt(buf.Bytes(), vals[i], 10)
	}
	buf.WriteByte(']')
}

// AppendUint 将 uint 转换为字符串并附加到缓冲区。
func AppendUint(buf *bytes.Buffer, val uint) {
	buf.WriteString(strconv.FormatUint(uint64(val), 10))
}

// AppendUints 将 []uint 编码为 JSON 数组。
func AppendUints(buf *bytes.Buffer, vals []uint) {
	if len(vals) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteByte('[')
	buf.Write(strconv.AppendUint(buf.Bytes(), uint64(vals[0]), 10))
	for i := 1; i < len(vals); i++ {
		buf.WriteByte(',')
		buf.Write(strconv.AppendUint(buf.Bytes(), uint64(vals[i]), 10))
	}
	buf.WriteByte(']')
}

// AppendUint8 将 uint8 转换为字符串并附加到缓冲区。
func AppendUint8(buf *bytes.Buffer, val uint8) {
	buf.WriteString(strconv.FormatUint(uint64(val), 10))
}

// AppendUint8s 将 []uint8 编码为 JSON 数组。
func AppendUint8s(buf *bytes.Buffer, vals []uint8) {
	if len(vals) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteByte('[')
	buf.Write(strconv.AppendUint(buf.Bytes(), uint64(vals[0]), 10))
	for i := 1; i < len(vals); i++ {
		buf.WriteByte(',')
		buf.Write(strconv.AppendUint(buf.Bytes(), uint64(vals[i]), 10))
	}
	buf.WriteByte(']')
}

// AppendUint16 将 uint16 转换为字符串并附加到缓冲区。
func AppendUint16(buf *bytes.Buffer, val uint16) {
	buf.WriteString(strconv.FormatUint(uint64(val), 10))
}

// AppendUint16s 将 []uint16 编码为 JSON 数组。
func AppendUint16s(buf *bytes.Buffer, vals []uint16) {
	if len(vals) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteByte('[')
	buf.Write(strconv.AppendUint(buf.Bytes(), uint64(vals[0]), 10))
	for i := 1; i < len(vals); i++ {
		buf.WriteByte(',')
		buf.Write(strconv.AppendUint(buf.Bytes(), uint64(vals[i]), 10))
	}
	buf.WriteByte(']')
}

// AppendUint32 将 uint32 转换为字符串并附加到缓冲区。
func AppendUint32(buf *bytes.Buffer, val uint32) {
	buf.WriteString(strconv.FormatUint(uint64(val), 10))
}

// AppendUint32s 将 []uint32 编码为 JSON 数组。
func AppendUint32s(buf *bytes.Buffer, vals []uint32) {
	if len(vals) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteByte('[')
	buf.Write(strconv.AppendUint(buf.Bytes(), uint64(vals[0]), 10))
	for i := 1; i < len(vals); i++ {
		buf.WriteByte(',')
		buf.Write(strconv.AppendUint(buf.Bytes(), uint64(vals[i]), 10))
	}
	buf.WriteByte(']')
}

// AppendUint64 将 uint64 转换为字符串并附加到缓冲区。
func AppendUint64(buf *bytes.Buffer, val uint64) {
	buf.WriteString(strconv.FormatUint(val, 10))
}

// AppendUint64s 将 []uint64 编码为 JSON 数组。
func AppendUint64s(buf *bytes.Buffer, vals []uint64) {
	if len(vals) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteByte('[')
	buf.Write(strconv.AppendUint(buf.Bytes(), vals[0], 10))
	for i := 1; i < len(vals); i++ {
		buf.WriteByte(',')
		buf.Write(strconv.AppendUint(buf.Bytes(), vals[i], 10))
	}
	buf.WriteByte(']')
}

// AppendFloat32 将 float32 转换为字符串并附加到缓冲区。
func AppendFloat32(buf *bytes.Buffer, val float32) {
	appendFloat(buf, float64(val), 32)
}

// AppendFloats32 将 []float32 编码为 JSON 数组。
func AppendFloat32s(buf *bytes.Buffer, vals []float32) {
	if len(vals) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteByte('[')
	appendFloat(buf, float64(vals[0]), 32)
	for i := 1; i < len(vals); i++ {
		buf.WriteByte(',')
		appendFloat(buf, float64(vals[i]), 32)
	}
	buf.WriteByte(']')
}

// AppendFloat64 将 float64 转换为字符串并附加到缓冲区。
func AppendFloat64(buf *bytes.Buffer, val float64) {
	appendFloat(buf, val, 64)
}

// AppendFloat64s 将 []float64 编码为 JSON 数组。
func AppendFloat64s(buf *bytes.Buffer, vals []float64) {
	if len(vals) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteByte('[')
	appendFloat(buf, vals[0], 64)
	for i := 1; i < len(vals); i++ {
		buf.WriteByte(',')
		appendFloat(buf, vals[i], 64)
	}
	buf.WriteByte(']')
}

// AppendInterface 将接口编组为 JSON 并附加到缓冲区。
func AppendInterface(buf *bytes.Buffer, i any) {
	marshaled, err := json.Marshal(i)
	if err != nil {
		AppendString(buf, fmt.Sprintf("marshaling error: %v", err))
		return
	}
	buf.Write(marshaled)
}

// AppendType 将参数类型作为字符串附加到缓冲区。
func AppendType(buf *bytes.Buffer, i any) {
	if i == nil {
		AppendString(buf, "<nil>")
		return
	}
	AppendString(buf, reflect.TypeOf(i).String())
}

// appendFloat 处理特殊浮点值（NaN、Inf）并附加到缓冲区。
func appendFloat(buf *bytes.Buffer, val float64, bitSize int) {
	switch {
	case math.IsNaN(val):
		buf.WriteString(`"NaN"`)
	case math.IsInf(val, 1):
		buf.WriteString(`"Inf"`)
	case math.IsInf(val, -1):
		buf.WriteString(`"-Inf"`)
	}
	strconv.AppendFloat(buf.Bytes(), val, 'f', -1, bitSize)
}

const _hex = "0123456789abcdef"

var _noEscapeTable = [256]bool{}

func init() {
	for i := 0; i <= 0x7e; i++ {
		_noEscapeTable[i] = i >= 0x20 && i != '\\' && i != '"'
	}
}

// AppendStrings 将输入字符串编码为 JSON 数组并附加到缓冲区。
func AppendStrings(buf *bytes.Buffer, vals []string) {
	if len(vals) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteByte('[')
	AppendString(buf, vals[0])
	for i := 1; i < len(vals); i++ {
		buf.WriteByte(',')
		AppendString(buf, vals[i])
	}
	buf.WriteByte(']')
}

// AppendString 将输入字符串编码为 JSON 并附加到缓冲区。
//
// 该操作循环遍历字符串中的每个字节，查找。
// 对于需要 JSON 或 UTF-8 编码的字符。如果字符串。
// 不需要编码，则将字符串附加到其。
// 整个字节片。
func AppendString(buf *bytes.Buffer, s string) {
	// 以双引号开头。
	buf.WriteByte('"')

	for i := 0; i < len(s); i++ {
		if !_noEscapeTable[s[i]] {
			appendStringComplex(buf, s)
			buf.WriteByte('"')
			return
		}
	}

	buf.WriteString(s)
	buf.WriteByte('"')
}

// AppendStringers 将提供的 Stringer 列表编码为 JSON 数组。
func AppendStringers(buf *bytes.Buffer, vals []fmt.Stringer) {
	if len(vals) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteByte('[')
	if vals[0] == nil {
		AppendString(buf, "<nil>")
	} else {
		AppendString(buf, vals[0].String())
	}
	for i := 1; i < len(vals); i++ {
		buf.WriteByte(',')
		if vals[i] == nil {
			AppendString(buf, "<nil>")
		} else {
			AppendString(buf, vals[i].String())
		}
	}
	buf.WriteByte(']')
}

// AppendStringer 将输入 Stringer 编码为 JSON 并附加到缓冲区。
func AppendStringer(buf *bytes.Buffer, val fmt.Stringer) {
	if val == nil {
		AppendString(buf, "<nil>")
		return
	}
	AppendString(buf, val.String())
}

// appendStringComplex 处理需要转义的字符的字符串编码。
func appendStringComplex(buf *bytes.Buffer, s string) {
	start := 0
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= utf8.RuneSelf {
			// 处理 UTF-8 多字节字符。
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				// 无效的 UTF-8 序列。
				if start < i {
					buf.WriteString(s[start:i])
				}
				buf.WriteString(`\ufffd`)
				i += size - 1
				start = i + 1
				continue
			}
			// 有效 UTF-8，跳到下一个字符。
			i += size - 1
			continue
		}

		if _noEscapeTable[b] {
			continue
		}

		// 字符需要编码。
		if start < i {
			buf.WriteString(s[start:i])
		}

		switch b {
		case '"', '\\':
			buf.WriteByte('\\')
			buf.WriteByte(b)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			// 控制字符。
			buf.WriteString(`\u00`)
			buf.WriteByte(_hex[b>>4])
			buf.WriteByte(_hex[b&0xF])
		}
		start = i + 1
	}

	if start < len(s) {
		buf.WriteString(s[start:])
	}
	buf.WriteByte('"')
}
