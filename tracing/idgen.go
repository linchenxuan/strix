package tracing

import (
	"encoding/binary"
	"encoding/hex"
	"sync/atomic"
	"time"
)

// TraceID 是链路唯一标识，16 字节。
type TraceID [16]byte

// SpanID 是 Span 唯一标识，8 字节。
type SpanID [8]byte

// emptySpanID 用于判断 SpanID 是否为空。
var emptySpanID SpanID

// String 将 TraceID 转为十六进制字符串。
func (t TraceID) String() string {
	return hex.EncodeToString(t[:])
}

// String 将 SpanID 转为十六进制字符串。
func (s SpanID) String() string {
	return hex.EncodeToString(s[:])
}

// IsEmpty 判断 SpanID 是否为空。
func (s SpanID) IsEmpty() bool {
	return s == emptySpanID
}

// IDGenerator 基于 serverID + 时间戳 + 自增序号生成 ID，无系统调用。
// 格式：[2字节serverID][6字节时间戳微秒][8字节自增序号]
type IDGenerator struct {
	serverID uint16         // 服务器 ID，写入 TraceID 前 2 字节。
	counter  atomic.Uint64  // 无锁自增序号。
}

// NewIDGenerator 创建一个 ID 生成器。
func NewIDGenerator(serverID uint16) *IDGenerator {
	return &IDGenerator{
		serverID: serverID,
	}
}

// NewTraceID 生成一个新的 TraceID。
// 格式：[2字节serverID][6字节时间戳微秒][8字节自增序号]
func (g *IDGenerator) NewTraceID() TraceID {
	var id TraceID
	// 高 8 字节：serverID(16bit) + 时间戳微秒低 48bit。
	hi := uint64(g.serverID)<<48 | uint64(time.Now().UnixMicro())&0xFFFFFFFFFFFF
	binary.BigEndian.PutUint64(id[0:8], hi)
	// 低 8 字节：自增序号。
	binary.BigEndian.PutUint64(id[8:16], g.counter.Add(1))
	return id
}

// NewSpanID 生成一个新的 SpanID。
func (g *IDGenerator) NewSpanID() SpanID {
	var id SpanID
	binary.BigEndian.PutUint64(id[0:8], g.counter.Add(1))
	return id
}
