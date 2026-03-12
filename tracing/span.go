// Package tracing 提供高性能内存 Ring Buffer 链路追踪模块。
package tracing

import (
	"sync"
	"time"
)

// StatusCode 表示 Span 的完成状态。
type StatusCode uint8

const (
	// StatusOK 表示正常完成。
	StatusOK StatusCode = 0
	// StatusError 表示出错。
	StatusError StatusCode = 1
)

// spanCmd 标识 spanRecord 是开始还是结束。
type spanCmd uint8

const (
	spanCmdStart spanCmd = 1
	spanCmdEnd   spanCmd = 2
)

// maxTags 每个 spanRecord 最多携带的 Tag 数量。
const maxTags = 4

// spanRecord 是写入 Ring Buffer 的记录单元。
// 使用值类型和固定大小数组，避免堆分配和 GC 压力。
type spanRecord struct {
	TraceID  TraceID // 所属链路 ID。
	SpanID   SpanID  // 当前 Span ID。
	ParentID SpanID  // 父 Span ID，根 Span 为零值。

	Timestamp int64 // 单调时钟纳秒。

	OpCode uint16     // 操作名枚举，见 opcode.go。
	Cmd    spanCmd    // start 或 end。
	Status StatusCode // 完成状态，仅 end 记录有效。

	// 固定大小 Tag 槽位，避免 map 分配。
	TagKeys [maxTags]uint16 // Tag 键枚举。
	TagVals [maxTags]uint64 // Tag 值。
	TagLen  uint8           // 实际使用的 Tag 数量。
}

// programStart 用于计算单调时钟偏移。
var programStart = time.Now()

// monotimeNano 返回基于程序启动的单调时钟纳秒数。
// 避免 NTP 跳变导致 EndTime < StartTime。
func monotimeNano() int64 {
	return time.Since(programStart).Nanoseconds()
}

// spanRecordPool 用于复用 spanRecord 对象，减少 GC 压力。
var spanRecordPool = sync.Pool{
	New: func() any {
		return &spanRecord{}
	},
}

func acquireRecord() *spanRecord {
	r := spanRecordPool.Get().(*spanRecord)
	*r = spanRecord{} // 重置为零值。
	return r
}

func releaseRecord(r *spanRecord) {
	spanRecordPool.Put(r)
}

// SpanContext 是跨服务传播的最小信息单元。
// 进程内通过值传递，跨进程通过 Meta 或包头传播。
type SpanContext struct {
	TraceID TraceID // 链路 ID。
	SpanID  SpanID  // Span ID。
	Sampled bool    // 是否被采样。
}

// IsNil 判断 SpanContext 是否为空（未采样或未初始化）。
func (sc SpanContext) IsNil() bool {
	return !sc.Sampled || sc.SpanID.IsEmpty()
}

// Span 是业务层使用的追踪句柄。
// 持有 SpanContext 和对 Tracer 的引用，提供 End/StartChild/AddEvent 等方法。
type Span struct {
	sc     SpanContext // 传播上下文。
	opCode uint16     // 操作名枚举。
	tracer *Tracer    // 所属 Tracer，NilSpan 时为 nil。
}

// NilSpan 是一个空 Span，所有方法都是安全的空操作。
var NilSpan = Span{}

// SpanContext 返回当前 Span 的传播上下文。
func (s Span) SpanContext() SpanContext {
	return s.sc
}

// TracingID 返回 TraceID 字符串。
func (s Span) TracingID() string {
	return s.sc.TraceID.String()
}

// IsNil 判断是否为空 Span。
func (s Span) IsNil() bool {
	return s.tracer == nil || s.sc.IsNil()
}

// End 结束当前 Span。
func (s Span) End(opts ...SpanEndOption) {
	if s.IsNil() {
		return
	}
	cfg := spanEndConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	s.tracer.finishSpan(s.sc, s.opCode, cfg.status)
}

// StartChild 创建子 Span。
func (s Span) StartChild(name string, opts ...SpanStartOption) Span {
	if s.IsNil() {
		return NilSpan
	}
	return s.tracer.startChild(s.sc, name, opts...)
}

// AddEvent 记录一个事件。
func (s Span) AddEvent(name string) {
	if s.IsNil() {
		return
	}
	opCode := getOrRegisterOp(name)
	rec := acquireRecord()
	rec.TraceID = s.sc.TraceID
	rec.SpanID = s.sc.SpanID
	rec.OpCode = opCode
	rec.Cmd = spanCmdStart
	rec.Timestamp = monotimeNano()
	s.tracer.produce(rec)
	releaseRecord(rec)
}

// MarkMeta 将追踪信息写入 Meta 字典，用于跨服务传播。
func (s Span) MarkMeta(meta map[string]string) {
	if s.IsNil() {
		return
	}
	meta[MetaKeyTraceID] = s.sc.TraceID.String()
	meta[MetaKeySpanID] = s.sc.SpanID.String()
}
