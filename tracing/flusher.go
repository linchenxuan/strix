package tracing

import (
	"encoding/hex"
	"time"

	"github.com/linchenxuan/strix/log"
)

// Flusher 后台上报协程，从 Ring Buffer 批量消费 spanRecord 并输出到 Logger。
type Flusher struct {
	ringBuf  *SpanRingBuffer // 数据来源。
	batchBuf []spanRecord    // 预分配批量消费缓冲区。
	logger   log.Logger      // 输出目标。
	stopCh   chan struct{}    // 停止信号。
	doneCh   chan struct{}    // 停止完成信号。
	interval time.Duration   // flush 周期。
}

// NewFlusher 创建一个后台 Flusher。
func NewFlusher(rb *SpanRingBuffer, logger log.Logger, interval time.Duration) *Flusher {
	if interval <= 0 {
		interval = time.Second
	}
	return &Flusher{
		ringBuf:  rb,
		batchBuf: make([]spanRecord, 256),
		logger:   logger,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
		interval: interval,
	}
}

// Run 启动后台 flush 循环，应在独立 goroutine 中运行。
func (f *Flusher) Run() {
	defer close(f.doneCh)

	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			f.flush()
		case <-f.stopCh:
			f.flush() // 退出前最后刷一次。
			return
		}
	}
}

// Stop 停止 Flusher 并等待最后一次 flush 完成。
func (f *Flusher) Stop() {
	close(f.stopCh)
	<-f.doneCh
}

// flush 批量消费 Ring Buffer 并输出到 Logger。
func (f *Flusher) flush() {
	for {
		n := f.ringBuf.ConsumeBatch(f.batchBuf)
		if n == 0 {
			return
		}
		for i := 0; i < n; i++ {
			f.writeRecord(&f.batchBuf[i])
		}
	}
}

// writeRecord 将一条 spanRecord 输出为结构化日志。
func (f *Flusher) writeRecord(rec *spanRecord) {
	if f.logger == nil {
		return
	}

	e := f.logger.Info()
	if e == nil {
		return
	}

	// 基础字段。
	e.Str("traceId", hex.EncodeToString(rec.TraceID[:]))
	e.Str("spanId", hex.EncodeToString(rec.SpanID[:]))

	if !rec.ParentID.IsEmpty() {
		e.Str("parentId", hex.EncodeToString(rec.ParentID[:]))
	}

	// 操作名。
	opName := OpName(rec.OpCode)
	if opName != "" {
		e.Str("op", opName)
	}

	// 命令类型。
	switch rec.Cmd {
	case spanCmdStart:
		e.Str("cmd", "start")
	case spanCmdEnd:
		e.Str("cmd", "end")
	}

	// 状态。
	if rec.Cmd == spanCmdEnd {
		e.Uint8("status", uint8(rec.Status))
	}

	// 时间戳（纳秒）。
	e.Int64("ts", rec.Timestamp)

	// Tags。
	for j := uint8(0); j < rec.TagLen && j < maxTags; j++ {
		tagName := OpName(rec.TagKeys[j])
		if tagName == "" {
			continue
		}
		e.Uint64(tagName, rec.TagVals[j])
	}

	e.Msg("trace")
}
