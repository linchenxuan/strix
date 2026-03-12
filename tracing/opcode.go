package tracing

import "sync"

// opRegistry 管理 operation 名称到 uint16 枚举值的双向映射。
// 核心目的：让 spanRecord 用 uint16 存储操作名，而非 string。
// 这使 spanRecord 保持纯值类型、固定大小、无指针，写入 Ring Buffer 时
// 是纯内存拷贝，不逃逸到堆，GC 无需扫描。
// 只有后台 Flusher 输出日志时才通过 OpName() 反查字符串。
type opRegistry struct {
	mu      sync.RWMutex      // 读写锁，保护并发注册。
	names   map[uint16]string // code → name 反查表。
	codes   map[string]uint16 // name → code 正查表。
	nextSeq uint16            // 下一个分配的枚举值。
}

var globalOpRegistry = &opRegistry{
	names: make(map[uint16]string),
	codes: make(map[string]uint16),
}

// RegisterOp 注册一个 operation 名称并返回其枚举值。
// 通常在 init() 或启动阶段调用。
func RegisterOp(name string) uint16 {
	return globalOpRegistry.register(name)
}

func (r *opRegistry) register(name string) uint16 {
	r.mu.Lock()
	defer r.mu.Unlock()

	if code, ok := r.codes[name]; ok {
		return code
	}
	r.nextSeq++
	r.names[r.nextSeq] = name
	r.codes[name] = r.nextSeq
	return r.nextSeq
}

// getOrRegisterOp 获取 operation 的枚举值，不存在则自动注册。
func getOrRegisterOp(name string) uint16 {
	r := globalOpRegistry
	r.mu.RLock()
	if code, ok := r.codes[name]; ok {
		r.mu.RUnlock()
		return code
	}
	r.mu.RUnlock()
	return r.register(name)
}

// OpName 根据枚举值获取 operation 名称。
func OpName(code uint16) string {
	globalOpRegistry.mu.RLock()
	defer globalOpRegistry.mu.RUnlock()
	return globalOpRegistry.names[code]
}
