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

// AppendBeginMarker inserts a map start character '{' into the destination buffer.
// This function initializes a JSON object structure for structured logging.
// It is a zero-allocation operation that directly writes the opening brace to the buffer.
//
// Parameters:
//   - buf: Buffer to append the begin marker to
func AppendBeginMarker(buf *bytes.Buffer) {
	buf.WriteByte('{')
}

// AppendEndMarker inserts a map end character '}' into the destination buffer.
// This function completes a JSON object structure for structured logging.
// It is a zero-allocation operation that directly writes the closing brace to the buffer.
//
// Parameters:
//   - buf: Buffer to append the end marker to
func AppendEndMarker(buf *bytes.Buffer) {
	buf.WriteByte('}')
}

// AppendKey appends a new key to the output JSON with proper formatting.
// This function handles the insertion of a key in a JSON object, ensuring proper
// comma separation from previous key-value pairs when necessary. It then appends
// the key (properly escaped) followed by a colon separator.
//
// Parameters:
//   - buf: Buffer to append the key to
//   - key: String key to append
func AppendKey(buf *bytes.Buffer, key string) {
	if buf.Len() >= 1 && buf.Bytes()[buf.Len()-1] != '{' {
		buf.WriteByte(',')
	}
	AppendString(buf, key)
	buf.WriteByte(':')
}

// AppendNil inserts a JSON 'null' value into the buffer.
// This function provides a standardized way to append null values to JSON structures
// in the log output, ensuring consistent formatting across the logging system.
//
// Parameters:
//   - buf: Buffer to append the null value to
func AppendNil(buf *bytes.Buffer) {
	buf.WriteString("null")
}

// AppendLineBreak appends a newline character '\n' to the buffer.
// This function is used to improve log readability by inserting line breaks
// between log entries or between different sections of complex log messages.
//
// Parameters:
//   - buf: Buffer to append the line break to
func AppendLineBreak(buf *bytes.Buffer) {
	buf.WriteByte('\n')
}

// AppendArrayStart adds a JSON array start character '[' to indicate the beginning of an array.
// This function initializes a JSON array structure for structured logging, allowing
// multiple values to be grouped together in a single field.
//
// Parameters:
//   - buf: Buffer to append the array start character to
func AppendArrayStart(buf *bytes.Buffer) {
	buf.WriteByte('[')
}

// AppendArrayEnd adds a JSON array end character ']' to indicate the completion of an array.
// This function closes a previously started JSON array structure, marking the end of
// a sequence of values in structured logging.
//
// Parameters:
//   - buf: Buffer to append the array end character to
func AppendArrayEnd(buf *bytes.Buffer) {
	buf.WriteByte(']')
}

// AppendArrayDelim adds a comma separator ',' between array elements when necessary.
// This function ensures proper JSON array formatting by inserting a comma only when
// the buffer is not empty, preventing trailing commas that would invalidate the JSON.
//
// Parameters:
//   - buf: Buffer to append the comma separator to
func AppendArrayDelim(buf *bytes.Buffer) {
	if buf.Len() > 0 {
		buf.WriteByte(',')
	}
}

// AppendBool converts a boolean value to its JSON string representation and appends it to the buffer.
// This function efficiently converts a boolean value to either "true" or "false" and writes it
// directly to the provided buffer, ensuring proper JSON serialization for boolean log values.
//
// Parameters:
//   - buf: Buffer to append the boolean value to
//   - val: Boolean value to be converted and appended
func AppendBool(buf *bytes.Buffer, val bool) {
	buf.Write(strconv.AppendBool(buf.Bytes(), val))
}

// AppendBools appends a JSON array of boolean values to the buffer. This function
// handles the conversion of a slice of bools into their JSON string representation,
// properly formatted as a JSON array with 'true' or 'false' values.
//
// Parameters:
// - buf: The buffer to append the formatted boolean array to
// - bools: The slice of boolean values to convert and append
//
// Returns:
// - The modified buffer with the boolean array appended
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

// AppendInt converts int to string and appends to buffer.
func AppendInt(buf *bytes.Buffer, val int) {
	buf.WriteString(strconv.FormatInt(int64(val), 10))
}

// AppendInts encodes []int to JSON array.
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

// AppendInt8 converts int8 to string and appends to buffer.
func AppendInt8(buf *bytes.Buffer, val int8) {
	buf.WriteString(strconv.FormatInt(int64(val), 10))
}

// AppendInt8s encodes []int8 to JSON array.
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

// AppendInt16 converts int16 to string and appends to buffer.
func AppendInt16(buf *bytes.Buffer, val int16) {
	buf.WriteString(strconv.FormatInt(int64(val), 10))
}

// AppendInt16s encodes []int16 to JSON array.
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

// AppendInt32 converts int32 to string and appends to buffer.
func AppendInt32(buf *bytes.Buffer, val int32) {
	buf.WriteString(strconv.FormatInt(int64(val), 10))
}

// AppendInt32s encodes []int32 to JSON array.
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

// AppendInt64 converts int64 to string and appends to buffer.
func AppendInt64(buf *bytes.Buffer, val int64) {
	buf.WriteString(strconv.FormatInt(val, 10))
}

// AppendInt64s encodes []int64 to JSON array.
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

// AppendUint converts uint to string and appends to buffer.
func AppendUint(buf *bytes.Buffer, val uint) {
	buf.WriteString(strconv.FormatUint(uint64(val), 10))
}

// AppendUints encodes []uint to JSON array.
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

// AppendUint8 converts uint8 to string and appends to buffer.
func AppendUint8(buf *bytes.Buffer, val uint8) {
	buf.WriteString(strconv.FormatUint(uint64(val), 10))
}

// AppendUint8s encodes []uint8 to JSON array.
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

// AppendUint16 converts uint16 to string and appends to buffer.
func AppendUint16(buf *bytes.Buffer, val uint16) {
	buf.WriteString(strconv.FormatUint(uint64(val), 10))
}

// AppendUint16s encodes []uint16 to JSON array.
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

// AppendUint32 converts uint32 to string and appends to buffer.
func AppendUint32(buf *bytes.Buffer, val uint32) {
	buf.WriteString(strconv.FormatUint(uint64(val), 10))
}

// AppendUint32s encodes []uint32 to JSON array.
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

// AppendUint64 converts uint64 to string and appends to buffer.
func AppendUint64(buf *bytes.Buffer, val uint64) {
	buf.WriteString(strconv.FormatUint(val, 10))
}

// AppendUint64s encodes []uint64 to JSON array.
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

// AppendFloat32 converts float32 to string and appends to buffer.
func AppendFloat32(buf *bytes.Buffer, val float32) {
	appendFloat(buf, float64(val), 32)
}

// AppendFloats32 encodes []float32 to JSON array.
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

// AppendFloat64 converts float64 to string and appends to buffer.
func AppendFloat64(buf *bytes.Buffer, val float64) {
	appendFloat(buf, val, 64)
}

// AppendFloat64s encodes []float64 to JSON array.
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

// AppendInterface marshals interface to JSON and appends to buffer.
func AppendInterface(buf *bytes.Buffer, i any) {
	marshaled, err := json.Marshal(i)
	if err != nil {
		AppendString(buf, fmt.Sprintf("marshaling error: %v", err))
		return
	}
	buf.Write(marshaled)
}

// AppendType appends the parameter type as string to buffer.
func AppendType(buf *bytes.Buffer, i any) {
	if i == nil {
		AppendString(buf, "<nil>")
		return
	}
	AppendString(buf, reflect.TypeOf(i).String())
}

// appendFloat handles special float values (NaN, Inf) and appends to buffer.
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

// AppendStrings encodes the input strings to JSON array and appends to buffer.
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

// AppendString encodes the input string to JSON and appends to buffer.
//
// The operation loops through each byte in the string looking
// for characters that need JSON or UTF-8 encoding. If the string
// does not need encoding, then the string is appended in its
// entirety to the byte slice.
func AppendString(buf *bytes.Buffer, s string) {
	// Start with a double quote
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

// AppendStringers encodes the provided Stringer list to JSON array.
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

// AppendStringer encodes the input Stringer to JSON and appends to buffer.
func AppendStringer(buf *bytes.Buffer, val fmt.Stringer) {
	if val == nil {
		AppendString(buf, "<nil>")
		return
	}
	AppendString(buf, val.String())
}

// appendStringComplex handles string encoding for characters that need escaping.
func appendStringComplex(buf *bytes.Buffer, s string) {
	start := 0
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= utf8.RuneSelf {
			// Handle UTF-8 multibyte characters
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError && size == 1 {
				// Invalid UTF-8 sequence
				if start < i {
					buf.WriteString(s[start:i])
				}
				buf.WriteString(`\ufffd`)
				i += size - 1
				start = i + 1
				continue
			}
			// Valid UTF-8, skip to next character
			i += size - 1
			continue
		}

		if _noEscapeTable[b] {
			continue
		}

		// Character needs encoding
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
			// Control characters
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
