# 链路追踪 (Tracing)

`strix` 框架内置了一个强大的链路追踪库，位于 `tracing` 包中。它旨在提供一个与 [OpenTracing](https://opentracing.io/) 兼容的 API，并为分布式系统中的请求提供端到端的可见性。该库的设计重点是高性能、低开销和灵活的扩展性。

## 设计理念与目标

### 1. OpenTracing 兼容性
`strix` 的链路追踪 API 在设计上遵循 OpenTracing 的规范和理念。这确保了开发者可以在 `strix` 中使用一套熟悉的 API 来进行链路追踪，同时也使得 `strix` 的追踪系统能够与各种支持 OpenTracing 标准的后端（如 Jaeger, Zipkin）轻松集成。

### 2. 高性能与低开销
在游戏服务器等高性能场景下，链路追踪的开销必须尽可能低。`strix` 通过以下方式实现低开销：
- **对象池 (`sync.Pool`)**: `Span`, `SpanContext` 等核心对象的实例通过 `sync.Pool` 进行复用，显著减少了内存分配和 GC 压力。
- **采样 (Sampling)**: 支持基于概率的采样，允许开发者在性能和追踪数据量之间做出权衡。
- **异步上报**: 追踪数据（Spans）通过异步 Reporter 上报到后端，避免了 I/O 操作阻塞关键业务逻辑。

### 3. 灵活的扩展性
`strix` 的追踪系统是可扩展的。开发者可以轻松地实现自定义的 `Reporter` 来对接不同的追踪后端，或者实现自定义的 `Propagator` 来支持不同的跨进程上下文传播协议。

## 快速入门

要在您的应用程序中使用链路追踪，您需要在启动时进行初始化。以下是一个将追踪数据上报到 Zipkin 的基本配置示例：

```go
package main

import (
    "context"
    "time"

    "github.com/linchenxuan/strix/tracing" // 确保路径正确
)

func main() {
    // 定义链路追踪配置
    cfg := &tracing.TracerConfig{
        ServiceName:    "my-awesome-service",
        Enabled:        true,
        SampleRate:     1.0, // 100% 采样
        ReporterType:   "zipkin",
        ReporterConfig: map[string]interface{}{
            "endpoint": "http://localhost:9411/api/v2/spans",
        },
    }

    // 初始化全局 Tracer
    tracer, err := tracing.InitTracing(cfg)
    if err != nil {
        panic("Failed to initialize tracer: " + err.Error())
    }
    defer tracer.Close() // 确保在应用程序关闭时上报所有追踪数据

    // --- 您的应用程序代码从这里开始 ---

    // 创建一个根 Span
    span, ctx := tracing.CreateGlobalSpan("mainOperation")
    defer span.End()

    span.SetTag("component", "main")
    span.LogEvent("Application started")

    // 模拟一些工作
    doWork(ctx, "task1")
    doWork(ctx, "task2")

    span.LogEvent("Application finished")
}

func doWork(ctx context.Context, taskName string) {
    // 从上下文中创建一个子 Span
    span, _ := tracing.CreateChildSpanFromContext(ctx, "doWork")
    defer span.End()

    span.SetTag("task.name", taskName)
    span.LogKV("event", "doing work", "duration", "100ms")
    time.Sleep(100 * time.Millisecond)
}
```

### 配置 `TracerConfig`

`tracing.TracerConfig` 结构体提供了链路追踪的配置选项：

| 字段                 | 类型                     | 描述                                                                    |
| :------------------- | :----------------------- | :---------------------------------------------------------------------- |
| `ServiceName`        | `string`                 | 服务名称，用于在追踪系统中标识您的服务。                                |
| `Enabled`            | `bool`                   | 是否启用链路追踪。                                                      |
| `SampleRate`         | `float64`                | 采样率（0.0 到 1.0）。例如，`0.5` 表示采样 50% 的追踪数据。             |
| `MaxSpans`           | `int`                    | 内存中保留的最大 Span 数量。                                            |
| `SpanTimeout`        | `time.Duration`          | Span 的最大活动时间。                                                   |
| `ReportInterval`     | `time.Duration`          | 向后端上报 Span 的时间间隔。                                            |
| `ReporterType`       | `string`                 | 上报器类型。支持 `console`, `in_memory`, `http`, `zipkin`。               |
| `ReporterConfig`     | `map[string]interface{}` | 上报器的具体配置。例如，`http` 和 `zipkin` 类型需要 `endpoint`。       |
| `PropagationFormats` | `[]string`               | 支持的跨进程上下文传播格式。例如 `text_map`, `http_headers`。           |

### 创建和使用 Spans

`strix` 的链路追踪库提供了简单易用的 API 来创建和操作 Spans。

**创建根 Span:**
当一个新的请求到达您的服务时，您应该创建一个“根” Span。

```go
// 创建一个名为 "handleRequest" 的根 Span
span, ctx := tracing.CreateGlobalSpan("handleRequest")
defer span.End() // 确保在操作结束时关闭 Span
```

**创建子 Span:**
在一个请求的处理流程中，您可以为内部的各个操作创建子 Span，以形成一个完整的调用链。

```go
// 从父级上下文中创建一个子 Span
childSpan, childCtx := tracing.CreateChildSpanFromContext(ctx, "databaseQuery")
defer childSpan.End()

// 在 childCtx 中执行数据库查询...
```

**添加元数据:**
您可以向 Span 中添加标签（Tags）和日志（Logs）来提供更多关于操作的上下文信息。

```go
// 添加标签
span.SetTag("http.method", "GET")
span.SetTag("http.status_code", 200)

// 添加日志
span.LogEvent("Query started")
span.LogKV("event", "user authenticated", "userID", "12345")
```

### 上下文传播 (Context Propagation)

为了在分布式系统中将不同服务的 Spans 连接成一个完整的 Trace，您需要在服务间传递追踪上下文。`strix` 的 `Propagator` 接口和 `Inject`/`Extract` 方法简化了这一过程。

**注入上下文（客户端）:**
在发起一个 RPC 或 HTTP 请求之前，将当前 Span 的上下文注入到请求中。

```go
// 假设 req 是一个 HTTP 请求
carrier := make(map[string]string)
err := tracing.InjectToCarrier(span.Context(), "http_headers", carrier)
if err == nil {
    for k, v := range carrier {
        req.Header.Set(k, v)
    }
}
```

**提取上下文（服务端）:**
在收到一个请求时，从请求中提取追踪上下文，并用它来创建一个子 Span。

```go
// 假设 req 是一个 HTTP 请求
carrier := make(map[string]string)
for k, v := range req.Header {
    carrier[k] = v[0]
}

parentCtx, err := tracing.ExtractFromCarrier("http_headers", carrier)
if err == nil {
    // 使用提取的上下文创建子 Span
    span, ctx := tracing.CreateChildSpanFromContext(context.Background(), "handleRequest", tracing.WithParent(parentCtx))
    // ...
}
```

## 总结

`strix` 链路追踪库提供了一套强大而灵活的工具，用于在分布式系统中实现端到端的请求追踪。其与 OpenTracing 兼容的 API、对高性能的关注以及可扩展的设计，使其成为构建可观测的游戏服务器和其他高性能应用的理想选择。
