# 日志 (Log)

`strix` 框架内置了一个高性能的日志库，位于 `log` 包中。它的设计灵感来源于 `zerolog`，并专为低内存分配、结构化日志而打造，旨在满足分布式游戏服务器对高性能和可观测性的严苛要求。

## 设计理念与优化思路

### 1. 灵感与目标

`strix` 的日志库受到了 `zerolog` 的启发，其核心设计理念是 **零内存分配（zero-allocation）** 和 **结构化日志**。在高性能服务器中，每一次内存分配都意味着潜在的 GC（垃圾回收）压力，可能导致服务卡顿。因此，日志库致力于在日志事件构建和写入过程中尽可能减少甚至避免内存分配。

选择 **结构化 JSON 格式** 作为输出，是为了便于机器解析和统一日志收集。这使得日志可以轻松地与各种日志分析工具（如 ELK Stack, Loki）集成，实现更高效的日志检索、分析和可视化。

### 2. 核心优化技术

-   **零内存分配的事件构建**: 通过 `LogEvent` 提供的 Fluent API，日志字段直接写入预分配的 `bytes.Buffer`，避免了字符串拼接和中间对象的创建。
-   **对象池 (`sync.Pool`)**: `LogEvent` 实例通过 `sync.Pool` 进行复用，显著减少了 `LogEvent` 对象的创建和销毁开销，从而降低了 GC 压力。
-   **异步写入**: 默认启用异步写入模式，日志事件被缓冲并通过独立的 Goroutine 批量写入，避免了日志 I/O 操作阻塞主业务逻辑，确保了服务主流程的低延迟。
-   **优化的字段追加**: 各种类型（如 `Int`, `Str`, `Time` 等）的日志字段追加都经过优化，直接操作字节切片或使用 `strconv` 包进行高效转换，避免了反射和不必要的装箱拆箱。
-   **caller 信息缓存**: 调用者信息（文件、函数、行号）被缓存，避免了重复的运行时信息查询，进一步提升性能。

## 快速入门

要使用日志库，您需要在应用程序启动时进行初始化。可以通过编程方式配置。

以下是一个基本的配置示例，它将日志同时输出到控制台和文件：

```go
package main

import (
    "errors"
    "github.com/linchenxuan/strix/log" // 确保路径正确
)

func main() {
    // 定义日志配置
    logCfg := &log.LogCfg{
        LogPath:         "./server.log",   // 日志文件路径
        LogLevel:        log.DebugLevel,   // 设置最低日志级别为 Debug
        FileAppender:    true,             // 启用文件日志
        ConsoleAppender: true,             // 启用控制台日志
        IsAsync:         true,             // 启用异步日志以提高性能
    }

    // 使用配置初始化日志记录器
    if err := log.Initialize(logCfg); err != nil {
        panic("Failed to initialize logger: " + err.Error())
    }
    defer log.Close() // 确保在应用程序关闭时刷新日志

    // --- 您的应用程序代码从这里开始 ---

    // 记录一些消息
    log.Info().Str("module", "main").Msg("应用程序启动")
    log.Debug().Int("port", 8080).Msg("服务器正在监听")
    log.Warn().Msg("这是一条警告消息")
    log.Error().Err(errors.New("一个示例错误")).Msg("发生了一些错误")

    log.Info().Msg("应用程序正在关闭")
}
```

### 配置 `LogCfg`

`log.LogCfg` 结构体提供了丰富的配置选项：

| 字段               | 类型                 | 描述                                                                    |
| :----------------- | :------------------- | :---------------------------------------------------------------------- |
| `LogPath`          | `string`             | 日志文件路径。                                                          |
| `LogLevel`         | `Level`              | 最低日志级别。例如 `log.DebugLevel`, `log.InfoLevel`。                    |
| `FileSplitMB`      | `int`                | 文件按大小轮转的阈值（MB）。当文件超过此大小时，将创建新文件。            |
| `FileSplitHour`    | `int`                | 文件按时间轮转的小时点（0-23）。例如，2表示每天凌晨2点轮转。                 |
| `IsAsync`          | `bool`               | 是否启用异步日志。推荐在高性能场景下启用。                                |
| `AsyncCacheSize`   | `int`                | 异步模式下最大缓冲日志条目数。防止内存溢出。                             |
| `AsyncWriteMillSec`| `int`                | 异步写入间隔（毫秒）。                                                  |
| `FileAppender`     | `bool`               | 是否启用文件日志输出。                                                  |
| `ConsoleAppender`  | `bool`               | 是否启用控制台日志输出。                                                |
| `EnabledCallerInfo`| `bool`               | 是否启用调用者信息（文件、函数、行号）捕获。                              |
| `CallerSkip`       | `int`                | 捕获调用者信息时跳过的堆栈帧数。                                        |
| `LevelChange`      | `[]LevelChangeEntry` | 运行时动态调整特定代码位置日志级别的规则。                                |
| `ActorWhiteList`   | `[]uint64`           | 绕过日志级别过滤的玩家/Actor ID 列表，用于定向调试。                      |
| `ActorFileLog`     | `bool`               | 是否为 `ActorLogger` 启用独立文件日志。                                  |

### 编写结构化日志

`strix` 日志库提供了一个 Fluent API 来构建结构化日志：

```go
// 记录信息级别日志，带有一个字符串字段和一个整数字段
log.Info().
    Str("userID", "12345").
    Int("score", 1000).
    Msg("玩家得分更新")

// 记录错误级别日志，附带错误对象和自定义消息
err := errors.New("数据库连接失败")
log.Error().
    Err(err).
    Str("component", "database").
    Msg("关键服务错误")

// 记录调试信息，包含布尔值和浮点数
log.Debug().
    Bool("is_active", true).
    Float64("latency_ms", 15.7).
    Msg("处理请求完成")

// 记录数组类型字段
log.Info().
    Ints("player_ids", []int{1, 2, 3}).
    Strs("items", []string{"sword", "shield"}).
    Msg("多玩家物品交易")
```

### 日志级别 (LogLevel)

日志级别定义了日志的严重性。`strix` 支持以下级别（按严重性递增）：

-   `TraceLevel`: 最详细的诊断信息。
-   `DebugLevel`: 调试信息，开发和故障排除时有用。
-   `InfoLevel`: 一般信息，表示应用程序的正常运行。
-   `WarnLevel`: 潜在有害情况，但不妨碍操作。
-   `ErrorLevel`: 严重问题，可能导致部分功能失效。
-   `FatalLevel`: 致命错误，导致应用程序终止。

### 文件轮转 (File Rotation)

日志文件可以根据大小或时间进行自动轮转，以避免单个日志文件过大。

**按大小轮转：**
通过 `LogCfg.FileSplitMB` 设置，例如设置为 `50`，则日志文件达到 50MB 时会自动轮转。

**按时间轮转：**
通过 `LogCfg.FileSplitHour` 设置，例如设置为 `2`，则每天凌晨 2 点会自动轮转日志文件。

## 总结

`strix` 日志库旨在提供一个高性能、易于使用且功能强大的日志解决方案，特别适合 Go 语言开发的高并发服务器应用。其低内存分配和结构化输出的特性，有助于在生产环境中进行高效的故障排查和数据分析。
