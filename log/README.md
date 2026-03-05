# strix/log

`strix/log` 是一个面向高并发服务的结构化日志库，支持：
- 同步/异步写入
- 文件与控制台双输出
- 按大小/时间轮转
- 链路字段（`trace_id`/`span_id`/`parent_span_id`）
- 低分配日志路径

## 安装与导入

```go
import "github.com/linchenxuan/strix/log"
```

## 快速开始

```go
package main

import (
	"github.com/linchenxuan/strix/log"
)

func main() {
	if err := log.Initialize(&log.LogCfg{
		LogPath:           "./app.log",
		LogLevel:          log.InfoLevel,
		FileSplitMB:       100,
		FileSplitHour:     0,
		IsAsync:           true,
		AsyncCacheSize:    2048,
		AsyncWriteMillSec: 200,
		FileAppender:      true,
		ConsoleAppender:   true,
		EnabledCallerInfo: true,
	}); err != nil {
		panic(err)
	}
	defer log.Close()

	log.Info().Str("service", "match").Int("online", 128).Msg("server started")
	log.Warn().Err(nil).Msg("something happened")
}
```

## 配置说明

| 字段 | 说明 | 典型值 |
|---|---|---|
| `LogPath` | 日志文件路径 | `./strix.log` |
| `LogLevel` | 最低日志级别 | `DebugLevel` / `InfoLevel` |
| `FileSplitMB` | 按大小轮转阈值（MB） | `50` |
| `FileSplitHour` | 按时间轮转小时（0-23，`0` 表示关闭） | `0` |
| `IsAsync` | 是否异步写入 | `true` |
| `AsyncCacheSize` | 异步队列容量 | `1024` |
| `AsyncWriteMillSec` | 异步批量刷盘间隔（ms） | `200` |
| `FileAppender` | 启用文件输出 | `true` |
| `ConsoleAppender` | 启用控制台输出 | `true` |
| `EnabledCallerInfo` | 输出 `caller` 字段 | `true` |

说明：
- `Initialize` 会执行配置校验，非法配置会返回错误。
- 进程退出前请调用 `Close()`，异步模式下可确保缓冲日志落盘。

## 常用 API

### 日志级别入口
- `log.Debug()`
- `log.Info()`
- `log.Warn()`
- `log.Error()`
- `log.Fatal()`

### 常用字段方法
- 数值：`Int/Int64/Uint64/Float32/Float64`
- 文本：`Str/Strs`
- 布尔：`Bool/Bools`
- 错误：`Err/Errs`
- 对象：`Any/Obj`
- 链路：`Context(ctx)`（自动提取 `trace_id`/`span_id`/`parent_span_id`）

### 生命周期方法
- `Initialize(cfg)`：初始化默认日志器
- `Refresh()`：主动刷新（异步场景常用）
- `Close()`：关闭并释放资源

## 输出样例

```json
{"time":"2026-03-05 22:00:00.123","level":"INFO","caller":"log/example.go:42 main","service":"match","online":128,"msg":"server started"}
```

## 性能基准（本仓库当前实测）

命令：

```bash
go test -test.fullpath=true -benchmem -run=^$ -bench '^Benchmark(AsyncLogging|SyncLogging)$' github.com/linchenxuan/strix/log
```

结果（Linux/amd64）：
- `BenchmarkSyncLogging`: `2657 ns/op`, `101 B/op`, `0 allocs/op`
- `BenchmarkAsyncLogging`: `861.9 ns/op`, `48 B/op`, `0 allocs/op`

## 最佳实践

- 生产环境建议开启 `IsAsync=true`，并按吞吐调大 `AsyncCacheSize`。
- 关键路径避免写入超大 `Any()` 对象，优先拆成明确字段。
- 日志量高时可关闭 `EnabledCallerInfo` 进一步降低开销。
- 服务停止前务必执行 `Close()`。
