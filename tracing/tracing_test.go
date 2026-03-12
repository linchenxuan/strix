package tracing

import (
	"errors"
	"testing"
	"time"

	"github.com/linchenxuan/strix/log"
)

// --- 辅助函数 ---

// newTestTracer 创建一个用于测试的 Tracer，带真实 Logger 以覆盖 Flusher 输出路径。
func newTestTracer(sampleRate uint32) *Tracer {
	cfg := Config{
		ServerID:   1,
		SampleRate: sampleRate,
	}
	cfg.LogCfg.LogLevel = log.DebugLevel
	cfg.LogCfg.ConsoleAppender = true
	cfg.LogCfg.FileSplitMB = 50

	logger := log.NewLogger(&cfg.LogCfg)
	return NewTracer(cfg, logger)
}

// --- ID 生成器 ---

func TestIDGenerator(t *testing.T) {
	gen := NewIDGenerator(1)

	// TraceID 不重复。
	seen := make(map[TraceID]struct{})
	for i := 0; i < 1000; i++ {
		id := gen.NewTraceID()
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate TraceID at iteration %d", i)
		}
		seen[id] = struct{}{}
	}

	// SpanID 不重复。
	seenSpan := make(map[SpanID]struct{})
	for i := 0; i < 1000; i++ {
		id := gen.NewSpanID()
		if _, ok := seenSpan[id]; ok {
			t.Fatalf("duplicate SpanID at iteration %d", i)
		}
		seenSpan[id] = struct{}{}
	}
}

func TestIDStringRoundTrip(t *testing.T) {
	gen := NewIDGenerator(42)
	tid := gen.NewTraceID()
	sid := gen.NewSpanID()

	// String() 应返回 32/16 位十六进制字符串。
	if len(tid.String()) != 32 {
		t.Fatalf("TraceID string length expected 32, got %d", len(tid.String()))
	}
	if len(sid.String()) != 16 {
		t.Fatalf("SpanID string length expected 16, got %d", len(sid.String()))
	}
}

func TestSpanIDIsEmpty(t *testing.T) {
	var empty SpanID
	if !empty.IsEmpty() {
		t.Fatal("zero SpanID should be empty")
	}
	gen := NewIDGenerator(1)
	nonEmpty := gen.NewSpanID()
	if nonEmpty.IsEmpty() {
		t.Fatal("generated SpanID should not be empty")
	}
}

// --- Ring Buffer ---

func TestRingBuffer(t *testing.T) {
	rb := NewSpanRingBuffer(4) // 2^4 = 16 slots

	// 写入 16 条应该全部成功。
	for i := 0; i < 16; i++ {
		rec := &spanRecord{OpCode: uint16(i)}
		if !rb.Produce(rec) {
			t.Fatalf("Produce failed at %d, expected success", i)
		}
	}

	// 第 17 条应该被丢弃。
	rec := &spanRecord{OpCode: 99}
	if rb.Produce(rec) {
		t.Fatal("Produce should fail when buffer is full")
	}
	if rb.DroppedCount() != 1 {
		t.Fatalf("expected 1 dropped, got %d", rb.DroppedCount())
	}

	// 批量消费。
	out := make([]spanRecord, 32)
	n := rb.ConsumeBatch(out)
	if n != 16 {
		t.Fatalf("expected 16, got %d", n)
	}
	for i := 0; i < 16; i++ {
		if out[i].OpCode != uint16(i) {
			t.Fatalf("expected OpCode %d, got %d", i, out[i].OpCode)
		}
	}

	// 消费后应该为空。
	n = rb.ConsumeBatch(out)
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestRingBufferConsumeBatchPartial(t *testing.T) {
	rb := NewSpanRingBuffer(4) // 2^4 = 16 slots

	// 写入 10 条。
	for i := 0; i < 10; i++ {
		rec := &spanRecord{OpCode: uint16(i)}
		rb.Produce(rec)
	}

	// 用小缓冲区消费，每次最多 3 条。
	out := make([]spanRecord, 3)
	total := 0
	for {
		n := rb.ConsumeBatch(out)
		if n == 0 {
			break
		}
		total += n
	}
	if total != 10 {
		t.Fatalf("expected total 10, got %d", total)
	}
}

func TestRingBufferWrapAround(t *testing.T) {
	rb := NewSpanRingBuffer(4) // 2^4 = 16 slots

	out := make([]spanRecord, 16)

	// 写满 → 消费 → 再写满 → 再消费，验证环形绕回正确。
	for round := 0; round < 5; round++ {
		for i := 0; i < 16; i++ {
			rec := &spanRecord{OpCode: uint16(round*100 + i)}
			if !rb.Produce(rec) {
				t.Fatalf("round %d, Produce %d failed", round, i)
			}
		}
		n := rb.ConsumeBatch(out)
		if n != 16 {
			t.Fatalf("round %d, expected 16, got %d", round, n)
		}
		for i := 0; i < 16; i++ {
			expected := uint16(round*100 + i)
			if out[i].OpCode != expected {
				t.Fatalf("round %d, expected OpCode %d, got %d", round, expected, out[i].OpCode)
			}
		}
	}
}

// --- NilSpan 安全性 ---

func TestNilSpanSafety(t *testing.T) {
	s := NilSpan
	if !s.IsNil() {
		t.Fatal("NilSpan should be nil")
	}
	s.End()
	s.End(WithCode(1))
	s.End(WithErr(nil))
	s.End(WithStatusError())
	s.AddEvent("test")
	s.MarkMeta(map[string]string{})
	child := s.StartChild("child")
	if !child.IsNil() {
		t.Fatal("child of NilSpan should be nil")
	}
	// NilSpan TracingID 应为全零十六进制（[16]byte 零值）。
	zeroTraceID := TraceID{}
	if s.TracingID() != zeroTraceID.String() {
		t.Fatalf("NilSpan TracingID should be all zeros, got %s", s.TracingID())
	}
}

// --- OpCode 注册表 ---

func TestOpRegistry(t *testing.T) {
	code1 := RegisterOp("test.op1")
	code2 := RegisterOp("test.op2")
	if code1 == code2 {
		t.Fatal("different ops should have different codes")
	}

	// 重复注册应返回相同 code。
	code1Again := RegisterOp("test.op1")
	if code1 != code1Again {
		t.Fatal("same op should return same code")
	}

	if OpName(code1) != "test.op1" {
		t.Fatalf("expected test.op1, got %s", OpName(code1))
	}

	// 未注册的 code 应返回空字符串。
	if OpName(65535) != "" {
		t.Fatal("unknown code should return empty string")
	}
}

func TestGetOrRegisterOp(t *testing.T) {
	// 首次获取自动注册。
	code := getOrRegisterOp("test.auto_register")
	if code == 0 {
		t.Fatal("auto registered code should not be 0")
	}
	// 再次获取应一致。
	code2 := getOrRegisterOp("test.auto_register")
	if code != code2 {
		t.Fatal("getOrRegisterOp should return same code for same name")
	}
}

// --- 采样器 ---

func TestSampler(t *testing.T) {
	always := AlwaysSampler{}
	if always.ShouldSample(0) != Sample {
		t.Fatal("AlwaysSampler should always sample")
	}

	never := NeverSampler{}
	if never.ShouldSample(0) != Drop {
		t.Fatal("NeverSampler should never sample")
	}

	// rate=2 应精确 50% 采样。
	rate := NewRateSampler(2)
	sampled := 0
	for i := 0; i < 100; i++ {
		if rate.ShouldSample(0) == Sample {
			sampled++
		}
	}
	if sampled != 50 {
		t.Fatalf("expected exactly 50 samples with rate=2, got %d", sampled)
	}

	// rate=0 不采样。
	rateZero := NewRateSampler(0)
	if rateZero.ShouldSample(0) != Drop {
		t.Fatal("rate=0 should never sample")
	}

	// rate=1 全量采样。
	rateOne := NewRateSampler(1)
	for i := 0; i < 10; i++ {
		if rateOne.ShouldSample(0) != Sample {
			t.Fatal("rate=1 should always sample")
		}
	}
}

// --- Option ---

func TestWithCodeOption(t *testing.T) {
	tracer := newTestTracer(1)
	defer tracer.Stop()
	SetTracer(tracer)

	// WithCode(0) 应为 StatusOK。
	span := Start("test.option.code0")
	span.End(WithCode(0))

	// WithCode(非0) 应为 StatusError。
	span2 := Start("test.option.code1")
	span2.End(WithCode(-1))
}

func TestWithErrOption(t *testing.T) {
	tracer := newTestTracer(1)
	defer tracer.Stop()
	SetTracer(tracer)

	// WithErr(nil) 应为 StatusOK。
	span := Start("test.option.err_nil")
	span.End(WithErr(nil))

	// WithErr(非nil) 应为 StatusError。
	span2 := Start("test.option.err")
	span2.End(WithErr(errors.New("some error")))
}

func TestWithStatusErrorOption(t *testing.T) {
	tracer := newTestTracer(1)
	defer tracer.Stop()
	SetTracer(tracer)

	span := Start("test.option.status_error")
	span.End(WithStatusError())
}

// --- Tracer 核心流程 ---

func TestTracerStartEnd(t *testing.T) {
	tracer := newTestTracer(1)
	defer tracer.Stop()
	SetTracer(tracer)

	span := Start("test.root")
	if span.IsNil() {
		t.Fatal("span should not be nil with AlwaysSampler")
	}

	child := span.StartChild("test.child")
	if child.IsNil() {
		t.Fatal("child span should not be nil")
	}

	if span.SpanContext().TraceID != child.SpanContext().TraceID {
		t.Fatal("child should share parent's TraceID")
	}

	child.AddEvent("some_event")
	child.End()
	span.End()
}

func TestTracerNeverSample(t *testing.T) {
	tracer := newTestTracer(0) // NeverSampler
	defer tracer.Stop()
	SetTracer(tracer)

	span := Start("test.never")
	if !span.IsNil() {
		t.Fatal("NeverSampler should return NilSpan")
	}
}

func TestTracerDroppedCount(t *testing.T) {
	cfg := Config{
		ServerID:        1,
		SampleRate:      1,
		RingBufferPower: 2, // 2^2 = 4 slots，很容易满。
	}
	cfg.LogCfg.LogLevel = log.DebugLevel
	cfg.LogCfg.ConsoleAppender = true
	cfg.LogCfg.FileSplitMB = 50

	tracer := NewTracer(cfg, nil)
	defer tracer.Stop()
	SetTracer(tracer)

	// 快速写入远超 buffer 容量的 Span。
	for i := 0; i < 100; i++ {
		span := Start("test.overflow")
		span.End()
	}
	// 应有丢弃。
	if tracer.DroppedCount() == 0 {
		t.Fatal("expected some drops with tiny ring buffer")
	}
}

func TestStartChildWithUnsampledParent(t *testing.T) {
	tracer := newTestTracer(1)
	defer tracer.Stop()

	// 构造一个 Sampled=false 的 parent。
	unsampledSC := SpanContext{Sampled: false}
	child := tracer.startChild(unsampledSC, "test.unsampled_child")
	if !child.IsNil() {
		t.Fatal("child of unsampled parent should be NilSpan")
	}
}

func TestFinishSpanUnsampled(t *testing.T) {
	tracer := newTestTracer(1)
	defer tracer.Stop()

	// 不应 panic。
	tracer.finishSpan(SpanContext{Sampled: false}, 0, StatusOK)
}

// --- 跨服务传播 ---

func TestSpanByMeta(t *testing.T) {
	tracer := newTestTracer(1)
	defer tracer.Stop()
	SetTracer(tracer)

	span := Start("test.meta")
	meta := make(map[string]string)
	span.MarkMeta(meta)

	// 验证 Meta 字段存在。
	if meta[MetaKeyTraceID] == "" || meta[MetaKeySpanID] == "" {
		t.Fatal("MarkMeta should set traceId and spanId")
	}

	// 从 Meta 恢复。
	restored := SpanByMeta(meta)
	if restored.IsNil() {
		t.Fatal("restored span should not be nil")
	}
	if restored.SpanContext().TraceID != span.SpanContext().TraceID {
		t.Fatal("restored span should have same TraceID")
	}
}

func TestSpanByMetaInvalid(t *testing.T) {
	tracer := newTestTracer(1)
	defer tracer.Stop()
	SetTracer(tracer)

	// 空 Meta。
	s := SpanByMeta(map[string]string{})
	if !s.IsNil() {
		t.Fatal("SpanByMeta with empty meta should return NilSpan")
	}

	// 缺少 spanId。
	s = SpanByMeta(map[string]string{MetaKeyTraceID: "0000000000000000"})
	if !s.IsNil() {
		t.Fatal("SpanByMeta with missing spanId should return NilSpan")
	}

	// 非法十六进制。
	s = SpanByMeta(map[string]string{MetaKeyTraceID: "zzzz", MetaKeySpanID: "xxxx"})
	if !s.IsNil() {
		t.Fatal("SpanByMeta with invalid hex should return NilSpan")
	}

	// 长度不对。
	s = SpanByMeta(map[string]string{MetaKeyTraceID: "aabb", MetaKeySpanID: "ccdd"})
	if !s.IsNil() {
		t.Fatal("SpanByMeta with wrong length should return NilSpan")
	}
}

func TestGlobalTracerNil(t *testing.T) {
	// 保存并清空全局 Tracer。
	old := getTracer()
	SetTracer(nil)
	defer SetTracer(old)

	s := Start("test.no_tracer")
	if !s.IsNil() {
		t.Fatal("Start with nil global tracer should return NilSpan")
	}

	s = SpanByMeta(map[string]string{MetaKeyTraceID: "00000000000000000000000000000001", MetaKeySpanID: "0000000000000001"})
	if !s.IsNil() {
		t.Fatal("SpanByMeta with nil global tracer should return NilSpan")
	}
}

// --- Flusher ---

func TestFlusherWithLogger(t *testing.T) {
	// 使用真实 Logger 创建 Tracer，验证 Flusher 输出不 panic。
	tracer := newTestTracer(1)
	SetTracer(tracer)

	for i := 0; i < 20; i++ {
		span := Start("test.flusher")
		child := span.StartChild("test.flusher.child")
		child.AddEvent("event_a")
		child.End(WithCode(0))
		span.End(WithErr(errors.New("test err")))
	}

	// 等一个 flush 周期确保 Flusher 处理了数据。
	time.Sleep(1200 * time.Millisecond)
	tracer.Stop()
}

func TestFlusherWithNilLogger(t *testing.T) {
	// logger 为 nil 时 Flusher 不应 panic。
	rb := NewSpanRingBuffer(4)
	flusher := NewFlusher(rb, nil, 100*time.Millisecond)
	go flusher.Run()

	rec := &spanRecord{OpCode: 1, Cmd: spanCmdStart}
	rb.Produce(rec)

	time.Sleep(200 * time.Millisecond)
	flusher.Stop()
}

// --- Plugin ---

func TestPluginFactory(t *testing.T) {
	f := &factory{}
	if f.Type() != PluginType {
		t.Fatalf("expected type %s, got %s", PluginType, f.Type())
	}
	if f.Name() != PluginName {
		t.Fatalf("expected name %s, got %s", PluginName, f.Name())
	}
	cfgType := f.ConfigType()
	if cfgType == nil {
		t.Fatal("ConfigType should not be nil")
	}
	if _, ok := cfgType.(*Config); !ok {
		t.Fatal("ConfigType should return *Config")
	}
}

func TestPluginSetupAndDestroy(t *testing.T) {
	f := &factory{}
	cfg := f.ConfigType().(*Config)
	cfg.ServerID = 99
	cfg.SampleRate = 1
	cfg.LogCfg.LogLevel = log.DebugLevel
	cfg.LogCfg.ConsoleAppender = true
	cfg.LogCfg.FileSplitMB = 50

	p, err := f.Setup(cfg)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	if p == nil {
		t.Fatal("Setup should return non-nil plugin")
	}
	if p.FactoryName() != PluginName {
		t.Fatalf("expected factory name %s, got %s", PluginName, p.FactoryName())
	}

	// 验证全局 Tracer 已设置。
	if getTracer() == nil {
		t.Fatal("global tracer should be set after Setup")
	}

	// Destroy 不应 panic。
	f.Destroy(p)

	// Destroy nil plugin 不应 panic。
	f.Destroy(nil)
}

// --- 压力测试 ---

func TestConcurrentStartEnd(t *testing.T) {
	tracer := newTestTracer(1)
	defer tracer.Stop()
	SetTracer(tracer)

	done := make(chan struct{})
	for g := 0; g < 8; g++ {
		go func() {
			for i := 0; i < 1000; i++ {
				span := Start("test.concurrent")
				child := span.StartChild("test.concurrent.child")
				child.AddEvent("evt")
				child.End()
				span.End()
			}
			done <- struct{}{}
		}()
	}
	for g := 0; g < 8; g++ {
		<-done
	}
}

// --- Benchmark ---

func BenchmarkRingBufferProduce(b *testing.B) {
	rb := NewSpanRingBuffer(20) // 2^20 = 1M slots
	rec := &spanRecord{OpCode: 1}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Produce(rec)
	}
}

func BenchmarkIDGeneration(b *testing.B) {
	gen := NewIDGenerator(1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gen.NewTraceID()
	}
}

func BenchmarkStartEnd(b *testing.B) {
	tracer := newTestTracer(1)
	defer tracer.Stop()
	SetTracer(tracer)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		span := Start("bench.op")
		span.End()
	}
}

func BenchmarkStartChildEnd(b *testing.B) {
	tracer := newTestTracer(1)
	defer tracer.Stop()
	SetTracer(tracer)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		span := Start("bench.parent")
		child := span.StartChild("bench.child")
		child.End()
		span.End()
	}
}

func BenchmarkStartEndParallel(b *testing.B) {
	tracer := newTestTracer(1)
	defer tracer.Stop()
	SetTracer(tracer)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			span := Start("bench.parallel")
			span.End()
		}
	})
}
