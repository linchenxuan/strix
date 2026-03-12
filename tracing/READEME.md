# Tracing — 链路追踪模块

## 设计目标

```
游戏服务器：长连接推送模型，消息频率极高，延迟容忍 < 16ms（一帧）
```

核心原则：**追踪系统本身的开销必须趋近于零，绝不能影响游戏体验。**

---

## 一、整体架构

```
┌─────────────────────────────────────────────────────────────┐
│                        游戏服务器进程                          │
│                                                             │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐    │
│  │   网关 Gate   │   │  逻辑服 Game  │   │  DB Proxy    │    │
│  │              │   │              │   │              │    │
│  │   Start()    │   │ StartChild() │   │ StartChild() │    │
│  │   span.End() │   │ span.End()   │   │ span.End()   │    │
│  └──────┬───────┘   └──────┬───────┘   └──────┬───────┘    │
│         │                  │                   │            │
│         ▼                  ▼                   ▼            │
│  ┌──────────────────────────────────────────────────┐       │
│  │          Ring Buffer（无锁环形缓冲区）              │       │
│  │       游戏主 goroutine 写入 ~8ns，零阻塞            │       │
│  └──────────────────────┬───────────────────────────┘       │
│                         │                                   │
│                         ▼                                   │
│  ┌──────────────────────────────────────────────────┐       │
│  │      Background Flusher（后台上报协程）              │       │
│  │    独立 goroutine，批量消费 → 结构化 JSON 日志       │       │
│  └──────────────────────┬───────────────────────────┘       │
│                         │                                   │
│                         ▼                                   │
│  ┌──────────────────────────────────────────────────┐       │
│  │       log.Logger（异步文件日志）                     │       │
│  │       tracer.log → filebeat → 追踪后端平台          │       │
│  └──────────────────────────────────────────────────┘       │
└─────────────────────────────────────────────────────────────┘
```

---

## 二、文件结构

```
tracing/
├── idgen.go        # TraceID/SpanID 定义与高性能 ID 生成器
├── span.go         # Span/SpanContext 定义与生命周期方法
├── option.go       # Span 选项（WithCode/WithErr/WithStatusError）
├── opcode.go       # Operation 名称 ↔ uint16 枚举注册表
├── ringbuffer.go   # 无锁 Ring Buffer（生产者零阻塞）
├── sampler.go      # 采样器（AlwaysSampler/NeverSampler/RateSampler）
├── flusher.go      # 后台批量消费协程，输出结构化 JSON 日志
├── tracing.go      # Tracer 核心 + Config + 全局便捷函数
├── plugin.go       # strix 插件工厂注册
└── tracing_test.go # 单元测试 + 基准测试
```

---

## 三、核心设计

### 3.1 ID 生成 — 零系统调用

```go
// 格式：[2B serverID][6B 时间戳微秒][8B 自增序号]
// 无锁 atomic 自增，~75ns/op，0 allocs
gen := NewIDGenerator(serverID)
traceID := gen.NewTraceID()
spanID  := gen.NewSpanID()
```

### 3.2 Span 结构 — 零 GC 压力

内部使用 `spanRecord` 值类型 + 固定大小数组，写入 Ring Buffer 时做值拷贝，不逃逸到堆：

```go
type spanRecord struct {
    TraceID  TraceID         // [16]byte
    SpanID   SpanID          // [8]byte
    ParentID SpanID          // [8]byte
    Timestamp int64          // 单调时钟纳秒
    OpCode   uint16          // Operation 枚举
    Cmd      spanCmd         // start/end
    Status   StatusCode      // ok/error
    TagKeys  [4]uint16       // 固定槽位，避免 map
    TagVals  [4]uint64
    TagLen   uint8
}
```

### 3.3 无锁 Ring Buffer

```go
rb := NewSpanRingBuffer(16) // 2^16 = 65536 槽位

// 生产者（游戏主协程）：~8ns/op，0 allocs
rb.Produce(rec)

// 消费者（后台协程）：批量读取
n := rb.ConsumeBatch(buf)
```

缓冲区满时**丢弃新数据，绝不阻塞游戏逻辑**。

### 3.4 采样器

```go
AlwaysSampler{}           // 全量采样
NeverSampler{}            // 不采样
NewRateSampler(100)       // 1% 采样（每 100 次采 1 次）
```

---

## 四、使用方式

### 4.1 插件配置

```yaml
plugin:
  tracer:
    ringtracer:
      serverId: 1
      sampleRate: 1          # 1=全量，100=1%，0=不采样
      ringBufferPower: 16    # Ring Buffer 大小 2^16 = 65536
      path: "/data/log/tracer.log"
      level: 2               # Debug
      fileAppender: true
      splitMB: 100
      isAsync: true
```

### 4.2 业务代码

```go
import "github.com/linchenxuan/strix/tracing"

// 创建根 Span（网关入口）
span := tracing.Start("Gateway.HandleBuy")
defer span.End()

// 跨服务传播：写入 Meta
meta := make(map[string]string)
span.MarkMeta(meta)
// ... 将 meta 随消息发送到下游服务 ...

// 下游服务：从 Meta 恢复
span := tracing.SpanByMeta(meta)
child := span.StartChild("Game.ProcessBuy")
defer child.End()

// 记录事件
child.AddEvent("db_query_start")

// 错误时标记状态
child.End(tracing.WithErr(err))
child.End(tracing.WithCode(retCode))
```

### 4.3 NilSpan 安全

当 Tracer 未初始化或消息未被采样时，所有 Span 方法都是安全的空操作：

```go
span := tracing.Start("some.op") // Tracer 未初始化 → 返回 NilSpan
span.End()                        // 安全空操作，不 panic
span.StartChild("child")          // 返回 NilSpan
span.MarkMeta(meta)               // 安全空操作
```

---

## 五、性能基准

测试环境：Intel Xeon 8255C @ 2.50GHz

| 操作 | 耗时 | 内存分配 |
|------|------|---------|
| Ring Buffer 写入 | **~8.5 ns/op** | 0 allocs |
| ID 生成 | **~76 ns/op** | 0 allocs |
| Start + End 完整链路 | **~297 ns/op** | 1 allocs |

参考：一帧 16ms = 16,000,000ns。即使每帧 1000 个 Span = 297us ≈ 帧时间的 1.8%。

---

## 六、关键设计决策

| 设计点 | 选择与理由 |
|--------|-----------|
| ID 生成 | serverID+时间戳+atomic 自增，零系统调用 |
| Span 内部结构 | 值类型 spanRecord + uint16 枚举，不用 string/map |
| 内存管理 | sync.Pool + 值拷贝写入 Ring Buffer |
| 生产者-消费者 | 无锁 Ring Buffer（~8ns），不用 channel（~100ns） |
| 满载策略 | 丢弃新数据，绝不阻塞游戏逻辑 |
| 时钟 | time.Since 单调时钟，避免 NTP 跳变 |
| 输出 | 后台协程批量消费 → 结构化 JSON 日志 → 复用现有日志基础设施 |
| 插件化 | 实现 plugin.Factory 接口，配置驱动初始化 |

核心思想：**追踪系统是"旁观者"，宁可丢追踪数据，也不能丢游戏帧。**
