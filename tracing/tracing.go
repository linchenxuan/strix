package tracing

import (
	"encoding/hex"
	"sync/atomic"

	"github.com/linchenxuan/strix/log"
)

const (
	// MetaKeyTraceID 跨服务传播时 Meta 中的 traceID 键。
	MetaKeyTraceID = "traceId"
	// MetaKeySpanID 跨服务传播时 Meta 中的 spanID 键。
	MetaKeySpanID = "spanId"

	// PluginType 插件类型。
	PluginType = "tracer"
	// PluginName 插件名称。
	PluginName = "ringtracer"

	// defaultRingBufferPower Ring Buffer 默认 2 的幂次 16（容量 65536）。
	defaultRingBufferPower = 16
)

// Tracer 高性能链路追踪器。
// 使用无锁 Ring Buffer 收集 spanRecord，后台协程异步输出。
type Tracer struct {
	idGen   *IDGenerator    // ID 生成器。
	sampler Sampler         // 采样策略。
	ringBuf *SpanRingBuffer // 无锁环形缓冲区。
	flusher *Flusher        // 后台 flush 协程。
}

// globalTracer 全局 Tracer 实例。
var globalTracer atomic.Pointer[Tracer]

// SetTracer 设置全局 Tracer。
func SetTracer(t *Tracer) {
	globalTracer.Store(t)
}

// getTracer 获取全局 Tracer。
func getTracer() *Tracer {
	return globalTracer.Load()
}

// Config 配置。
type Config struct {
	log.LogCfg `mapstructure:",squash"`

	// ServerID 服务器 ID，用于生成全局唯一的 TraceID。
	ServerID uint16 `mapstructure:"serverId"`

	// SampleRate 采样率分母。1=全量，100=1%，0=不采样。
	SampleRate uint32 `mapstructure:"sampleRate"`

	// RingBufferPower Ring Buffer 大小的 2 的幂次。实际容量为 1 << power。默认 16（65536）。
	RingBufferPower int `mapstructure:"ringBufferPower"`
}

// NewTracer 创建一个新的 Tracer。
func NewTracer(cfg Config, logger log.Logger) *Tracer {
	power := cfg.RingBufferPower
	if power <= 0 {
		power = defaultRingBufferPower
	}

	var sampler Sampler
	switch cfg.SampleRate {
	case 0:
		sampler = NeverSampler{}
	case 1:
		sampler = AlwaysSampler{}
	default:
		sampler = NewRateSampler(cfg.SampleRate)
	}

	rb := NewSpanRingBuffer(power)
	flusher := NewFlusher(rb, logger, 0)

	t := &Tracer{
		idGen:   NewIDGenerator(cfg.ServerID),
		sampler: sampler,
		ringBuf: rb,
		flusher: flusher,
	}

	// 启动后台 flush 协程。
	go flusher.Run()

	return t
}

// Stop 停止 Tracer，等待最后一次 flush 完成。
func (t *Tracer) Stop() {
	if t.flusher != nil {
		t.flusher.Stop()
	}
}

// DroppedCount 返回因缓冲区满而丢弃的记录总数。
func (t *Tracer) DroppedCount() uint64 {
	return t.ringBuf.DroppedCount()
}

// produce 将一条 spanRecord 写入 Ring Buffer。
func (t *Tracer) produce(rec *spanRecord) {
	t.ringBuf.Produce(rec)
}

// startSpan 创建根 Span。
func (t *Tracer) startSpan(name string) Span {
	opCode := getOrRegisterOp(name)

	if t.sampler.ShouldSample(opCode) == Drop {
		return NilSpan
	}

	traceID := t.idGen.NewTraceID()
	spanID := t.idGen.NewSpanID()

	rec := acquireRecord()
	rec.TraceID = traceID
	rec.SpanID = spanID
	rec.OpCode = opCode
	rec.Cmd = spanCmdStart
	rec.Timestamp = monotimeNano()
	t.ringBuf.Produce(rec)
	releaseRecord(rec)

	return Span{
		sc: SpanContext{
			TraceID: traceID,
			SpanID:  spanID,
			Sampled: true,
		},
		opCode: opCode,
		tracer: t,
	}
}

// startChild 创建子 Span。
func (t *Tracer) startChild(parent SpanContext, name string, _ ...SpanStartOption) Span {
	if !parent.Sampled {
		return NilSpan
	}

	opCode := getOrRegisterOp(name)
	childSpanID := t.idGen.NewSpanID()

	rec := acquireRecord()
	rec.TraceID = parent.TraceID
	rec.SpanID = childSpanID
	rec.ParentID = parent.SpanID
	rec.OpCode = opCode
	rec.Cmd = spanCmdStart
	rec.Timestamp = monotimeNano()
	t.ringBuf.Produce(rec)
	releaseRecord(rec)

	return Span{
		sc: SpanContext{
			TraceID: parent.TraceID,
			SpanID:  childSpanID,
			Sampled: true,
		},
		opCode: opCode,
		tracer: t,
	}
}

// spanByMeta 从 Meta 字典恢复 Span（跨服务传播）。
func (t *Tracer) spanByMeta(meta map[string]string) Span {
	traceIDStr, ok1 := meta[MetaKeyTraceID]
	spanIDStr, ok2 := meta[MetaKeySpanID]
	if !ok1 || !ok2 {
		return NilSpan
	}

	traceIDBytes, err1 := hex.DecodeString(traceIDStr)
	spanIDBytes, err2 := hex.DecodeString(spanIDStr)
	if err1 != nil || err2 != nil || len(traceIDBytes) != 16 || len(spanIDBytes) != 8 {
		return NilSpan
	}

	var traceID TraceID
	var spanID SpanID
	copy(traceID[:], traceIDBytes)
	copy(spanID[:], spanIDBytes)

	return Span{
		sc: SpanContext{
			TraceID: traceID,
			SpanID:  spanID,
			Sampled: true,
		},
		tracer: t,
	}
}

// finishSpan 结束一个 Span。
func (t *Tracer) finishSpan(sc SpanContext, opCode uint16, status StatusCode) {
	if !sc.Sampled {
		return
	}

	rec := acquireRecord()
	rec.TraceID = sc.TraceID
	rec.SpanID = sc.SpanID
	rec.OpCode = opCode
	rec.Cmd = spanCmdEnd
	rec.Timestamp = monotimeNano()
	rec.Status = status
	t.ringBuf.Produce(rec)
	releaseRecord(rec)
}

// --- 全局便捷函数 ---

// Start 创建一个根 Span。
// 如果全局 Tracer 未初始化，返回 NilSpan（安全空操作）。
func Start(name string) Span {
	t := getTracer()
	if t == nil {
		return NilSpan
	}
	return t.startSpan(name)
}

// SpanByMeta 从 Meta 字典恢复 Span。
// 如果全局 Tracer 未初始化，返回 NilSpan。
func SpanByMeta(meta map[string]string) Span {
	t := getTracer()
	if t == nil {
		return NilSpan
	}
	return t.spanByMeta(meta)
}
