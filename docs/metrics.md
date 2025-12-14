# Metrics系统设计

## 1. 概述

Asura游戏服务器框架的Metrics系统采用**观察者模式**架构，提供高性能、低延迟的指标收集和上报机制。系统设计注重性能优化，支持多种指标类型和灵活的Reporter扩展机制。

## 2. 设计目标

### 2.1 核心目标
- **高性能**：无锁设计，异步处理，最小化对业务逻辑的影响
- **低延迟**：纳秒级指标记录，支持批量合并和异步上报
- **可扩展**：支持自定义指标类型和Reporter实现
- **易用性**：简洁的API设计，支持维度标签

### 2.2 架构特点
- **观察者模式**：指标变化自动通知所有注册的Reporter
- **策略驱动**：不同指标类型采用不同的聚合策略
- **维度支持**：支持多维度标签，便于细粒度监控
- **内存优化**：对象池复用，减少GC压力

## 3. 核心架构

### 3.1 观察者模式架构
```
┌─────────────────────────────────────────────────────────────┐
│                    Metrics Core                             │
├─────────────────┬─────────────────┬───────────────────────┤
│   Counter       │    Gauge        │    StopWatch          │
│  (Policy_Sum)    │  (Policy_Set)    │  (Policy_Stopwatch)    │
└────────┬────────┴────────┬────────┴──────────┬────────────┘
         │               │                  │
         ▼               ▼                  ▼
┌─────────────────────────────────────────────────────────────┐
│              Reporter Registry                              │
├─────────────────┬─────────────────┬───────────────────────┤
│   Prometheus    │    Console      │    Custom Reporter    │
│    Reporter     │    Reporter     │                       │
└─────────────────┴─────────────────┴───────────────────────┘
```

### 3.2 核心接口设计

#### Metrics接口（<mcfile name="metrics.go" path="strix/metrics/metrics.go"></mcfile>）
```go
type Metrics interface {
    Name() string    // 指标名称
    Group() string   // 指标分组
    Policy() Policy  // 聚合策略
}
```

#### Reporter接口（<mcfile name="reporter.go" path="strix/metrics/reporter.go"></mcfile>）
```go
type Reporter interface {
    Report(r Record)  // 上报指标记录
}
```

#### Record结构（<mcfile name="record.go" path="strix/metrics/record.go"></mcfile>）
```go
type Record struct {
    metrics   Metrics    // 指标定义
    value     Value      // 指标值
    cnt       int        // 计数（用于平均值计算）
    dimensions Dimension  // 维度标签
}
```

## 4. 指标类型与策略

### 4.1 指标类型

#### Counter（计数器）
- **文件**：<mcfile name="counter.go" path="strix/metrics/counter.go"></mcfile>
- **策略**：`Policy_Sum`
- **特点**：只增不减，支持批量累加
- **用途**：请求次数、错误次数、消息数量等

```go
type Counter interface {
    IncrBy(delta Value, dimensions Dimension)
    Incr(delta Value)
}
```

#### Gauge（仪表盘）
- **文件**：<mcfile name="gauge.go" path="strix/metrics/gauge.go"></mcfile>
- **策略**：`Policy_Set`
- **特点**：记录瞬时值，覆盖更新
- **用途**：在线人数、队列长度、内存使用等

```go
type Gauge interface {
    Update(value Value, dimensions Dimension)
    UpdateWithDim(value Value, dimensions Dimension)
}
```

#### StopWatch（计时器）
- **文件**：<mcfile name="stopwatch.go" path="strix/metrics/stopwatch.go"></mcfile>
- **策略**：`Policy_Stopwatch`
- **特点**：自动计算时间差，毫秒单位
- **用途**：请求耗时、函数执行时间等

```go
type StopWatch interface {
    RecordWithDim(dimensions Dimension, startTime time.Time) time.Duration
}
```

#### 衍生指标类型
- **AvgGauge**：平均值仪表盘（<mcfile name="avggauge.go" path="strix/metrics/avggauge.go"></mcfile>）
- **MaxGauge**：最大值仪表盘（<mcfile name="maxgauge.go" path="strix/metrics/maxgauge.go"></mcfile>）
- **MinGauge**：最小值仪表盘（<mcfile name="mingauge.go" path="strix/metrics/mingauge.go"></mcfile>）

### 4.2 聚合策略（<mcfile name="types.go" path="strix/metrics/types.go"></mcfile>）
```go
type Policy int

const (
    Policy_None Policy = iota
    Policy_Set         // 设置值
    Policy_Sum         // 求和
    Policy_Max         // 最大值
    Policy_Min         // 最小值
    Policy_Avg         // 平均值
    Policy_Stopwatch   // 计时器（毫秒）
)
```

## 5. Prometheus集成（实际实现分析）

### 5.1 当前实现状态
基于<mcfile name="prometheus.go" path="strix/metrics/reporter/prometheus.go"></mcfile>的实际代码分析：

#### ✅ 已实现功能：
- **HTTP服务**：自动创建HTTP服务器暴露指标
- **Push模式**：支持Push Gateway推送
- **指标映射**：完整的Prometheus指标类型转换
- **多维度支持**：标签系统完美集成
- **指标合并**：同类型指标智能合并
- **TTL健康检查**：基于指标通道使用率和检查时间的健康状态检查机制

### 5.2 核心实现

#### HTTP服务暴露
```go
func (x *PrometheusReporter) startHTTPSvr() (net.Addr, error) {
    l, err := net.ListenTCP("tcp", &net.TCPAddr{IP: nil, Port: 0})
    if err != nil {
        return nil, err
    }

    mux := http.NewServeMux()
    mux.Handle(x.cfg.MetricPath, promhttp.Handler())
    x.promSvr = &http.Server{Handler: mux}
    go x.promSvr.Serve(l)
    
    return l.Addr(), nil
}
```

#### 指标类型映射
| 系统指标类型 | Prometheus类型 | 策略处理 |
|-------------|---------------|----------|
| Counter | Counter | 直接累加 |
| Gauge | Gauge | 设置值 |
| AvgGauge | Gauge | 计算平均值 |
| MaxGauge | Gauge | 取最大值 |
| MinGauge | Gauge | 取最小值 |
| StopWatch | Gauge | 毫秒转换 |

#### 异步处理机制
```go
func (x *PrometheusReporter) Report(r metrics.Record) {
    select {
    case x.metricsChan <- r:
    default:
        log.Error().Msg("metrics chan full")
    }
}
```

## 6. 使用示例

### 6.1 基本使用
```go
// 创建Counter
counter := &counter{name: "player_login", group: "player"}
counter.Incr(1)  // 简单计数
counter.IncrWithDim(1, Dimension{"server": "s1", "region": "cn"})

// 创建Gauge
gauge := &gauge{name: "online_players", group: "player"}
gauge.Update(100)

// 使用StopWatch
sw := &stopwatch{name: "request_time", group: "network"}
duration := sw.RecordWithDim(nil, startTime)
```

### 6.2 Reporter注册
```go
// 注册Prometheus Reporter
cfg := &PrometheusReporterConfig{
    MetricPath:   "/metrics",
    HTTPListenIP: "0.0.0.0:0",
}
promReporter := NewPrometheusReporter(cfg)
metrics.RegisterReporter(promReporter)
```

### 6.3 维度使用
```go
dimensions := Dimension{
    "server_id": "game-001",
    "region":    "asia",
    "version":   "v1.0.0",
}
counter.IncrWithDim(1, dimensions)
```

## 7. 性能优化策略

### 7.1 无锁设计
- **Channel通信**：使用有缓冲Channel进行异步上报
- **批量处理**：支持指标记录的批量合并
- **背压机制**：Channel满时丢弃新记录，避免阻塞

### 7.2 内存优化
- **对象复用**：Record对象可复用，减少GC压力
- **字符串处理**：维度键值使用字符串替换优化
- **预分配**：使用`strings.Builder`预分配内存

### 7.3 并发安全
- **Goroutine安全**：每个Reporter独立处理指标
- **原子操作**：关键计数使用原子操作
- **Context管理**：支持优雅关闭和资源清理

## 8. 监控指标建议

### 8.1 系统层指标
```go
// Goroutine数量
goroutineGauge := &gauge{name: "goroutine_count", group: "system"}

// 内存使用
memoryGauge := &gauge{name: "memory_usage_mb", group: "system"}

// GC次数
gcCounter := &counter{name: "gc_count", group: "system"}
```

### 8.2 网络层指标
```go
// 连接数
connGauge := &gauge{name: "connection_count", group: "network"}

// 消息吞吐量
msgCounter := &counter{name: "message_total", group: "network"}

// 请求延迟
latencySw := &stopwatch{name: "request_latency_ms", group: "network"}
```

### 8.3 业务层指标
```go
// 在线玩家
onlineGauge := &gauge{name: "online_players", group: "player"}

// 登录次数
loginCounter := &counter{name: "login_total", group: "player"}

// 请求处理时间
handleSw := &stopwatch{name: "handle_time_ms", group: "business"}
```

## 9. 扩展性设计

### 9.1 自定义Reporter
```go
type MyReporter struct {
    // 自定义字段
}

func (r *MyReporter) Report(record metrics.Record) {
    // 自定义处理逻辑
    // 可以发送到InfluxDB、Elasticsearch等
}
```

### 9.2 自定义指标类型
```go
type MyMetrics struct {
    name  string
    group string
}

func (m *MyMetrics) Name() string { return m.name }
func (m *MyMetrics) Group() string { return m.group }
func (m *MyMetrics) Policy() metrics.Policy { 
    return metrics.Policy_Sum 
}
```

## 10. 最佳实践

### 10.1 指标命名规范
- 使用小写字母和下划线分隔
- 采用`group_name`格式，如`player_login_total`
- 保持命名的一致性和可读性

### 10.2 维度设计原则
- 控制维度数量（建议不超过5个）
- 避免高基数维度（如用户ID）
- 使用有意义的维度名称

### 10.3 性能考虑
- 为关键路径添加指标监控
- 合理设置采集频率（避免过于频繁）
- 监控Reporter自身的性能表现

### 10.4 错误处理
- Reporter错误不影响业务逻辑
- 记录Reporter错误日志便于排查
- 实现Reporter健康检查机制

## 11. 总结

Asura的Metrics系统设计注重实用性和性能，采用观察者模式实现了灵活的指标收集机制。系统支持多种指标类型和维度标签，通过Prometheus Reporter提供了完整的监控解决方案。现有的HTTP暴露和Push模式已经能够满足大部分监控需求。

该系统特别适合游戏服务器的高并发场景，能够在不影响游戏性能的前提下提供详细的运行时监控数据。