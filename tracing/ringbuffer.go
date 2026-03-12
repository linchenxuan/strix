package tracing

import (
	"sync"
	"sync/atomic"
)

// SpanRingBuffer 是环形缓冲区，用于多个游戏协程与后台上报协程之间的数据传递。
// 多生产者通过 mutex 串行写入，单消费者无锁批量读取。
// 生产者和消费者操作不同的槽位区域，天然无冲突。
// 缓冲区满时丢弃新数据，绝不阻塞游戏逻辑。
type SpanRingBuffer struct {
	mu       sync.Mutex     // 仅保护多生产者之间的互斥。
	buf      []spanRecord   // 预分配固定大小数组，值类型避免 GC 扫描。
	mask     uint64         // size - 1，用位运算取模。
	writePos atomic.Uint64  // 生产者写入位置。
	readPos  atomic.Uint64  // 消费者读取位置。

	dropCount atomic.Uint64 // 因缓冲区满而丢弃的记录计数。
}

// NewSpanRingBuffer 创建一个环形缓冲区。
// power 为 2 的幂次，实际容量为 1 << power。
// 例如传入 14，实际容量为 16384（2^14）。
func NewSpanRingBuffer(power int) *SpanRingBuffer {
	size := uint64(1) << power
	return &SpanRingBuffer{
		buf:  make([]spanRecord, size),
		mask: size - 1,
	}
}

// Produce 将一条 spanRecord 写入缓冲区。
// 多生产者安全。缓冲区满时返回 false 并丢弃数据，绝不阻塞。
func (rb *SpanRingBuffer) Produce(rec *spanRecord) bool {
	rb.mu.Lock()

	w := rb.writePos.Load()

	// 缓冲区满：丢弃。
	if w-rb.readPos.Load() >= uint64(len(rb.buf)) {
		rb.mu.Unlock()
		rb.dropCount.Add(1)
		return false
	}

	rb.buf[w&rb.mask] = *rec
	rb.writePos.Store(w + 1)
	rb.mu.Unlock()
	return true
}

// ConsumeBatch 批量读取缓冲区中的记录。
// 无需加锁：消费者读取 [readPos, writePos) 区间，
// 生产者写入 writePos 位置，两者操作不同槽位。
// 返回实际读取的数量。
func (rb *SpanRingBuffer) ConsumeBatch(out []spanRecord) int {
	r := rb.readPos.Load()
	w := rb.writePos.Load()

	n := int(w - r)
	if n == 0 {
		return 0
	}
	if n > len(out) {
		n = len(out)
	}

	for i := 0; i < n; i++ {
		out[i] = rb.buf[(r+uint64(i))&rb.mask]
	}
	rb.readPos.Add(uint64(n))
	return n
}

// DroppedCount 返回因缓冲区满而丢弃的记录总数。
func (rb *SpanRingBuffer) DroppedCount() uint64 {
	return rb.dropCount.Load()
}
