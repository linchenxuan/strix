# 网络系统架构 (Network)

## 1. 概述

`strix` 的网络系统采用接口驱动的分层设计，当前代码中的主链路是：

`Transport -> Dispatcher -> MsgLayer -> Business Handler`

其中，`Transport` 负责收发与编解码，`Dispatcher` 负责路由与过滤，`MsgLayer` 负责具体执行模型（例如 stateful actor 模型），业务处理函数通过消息注册系统绑定到具体消息 ID。

## 2. 设计目标

### 2.1 核心目标

- 高性能：连接收发、消息解码、分发路径尽量精简，并集成指标埋点。
- 可扩展：传输层、分发过滤器、消息处理层都可替换或扩展。
- 可观测：关键链路集成 metrics / tracing，便于定位延迟与错误。
- 解耦：网络协议细节与业务逻辑分离，业务只关注消息处理。

## 3. 核心分层

### 3.1 Transport 层（`network/transport`）

Transport 层对外暴露统一接口：

- `Start(opt TransportOption) error`
- `StopRecv() error`
- `Stop() error`

关键点：

- `TransportOption` 注入两个依赖：
  - `Creator`：消息工厂（按 `msgID` 创建 protobuf 消息）。
  - `Handler`：上层接收器（通常是 `Dispatcher`，实现 `OnRecvTransportPkg`）。
- 收到完整包后，Transport 通过 `TransportDelivery` 上送：
  - `Pkg *TransRecvPkg`：解码后的请求包。
  - `TransSendBack`：回包函数，支持沿原链路返回响应。

当前实现：

- `network/transport/tcp`：TCP 长连接传输。
- `network/transport/sidecar`：通过 Unix Socket + 共享内存与 sidecar 通信。

### 3.2 Dispatcher 层（`network/dispatcher`）

Dispatcher 是网络消息中枢，入口方法为：

- `OnRecvTransportPkg(td *transport.TransportDelivery) error`

核心职责：

- 根据 `msgID` 从消息注册表读取 `MsgProtoInfo`。
- 构建 `DispatcherDelivery` 并进入过滤器链。
- 按 `MsgLayerType` 选择目标 `MsgLayerReceiver`。
- 调用目标层 `OnRecvDispatcherPkg`。

内置机制：

- 消息过滤器链（`DispatcherFilterChain`）。
- 接收限流（token bucket，`DispatcherRecvLimiter`）。
- 指标埋点：接收总量、失败数、处理耗时等。

### 3.3 MsgLayer 层（`network/handler`）

`MsgLayerReceiver` 统一入口：

- `OnRecvDispatcherPkg(delivery handler.Delivery) error`

当前仓库内的主要实现为：

- `network/handler/stateful`：基于 Actor 的有状态模型。

stateful 模型特征：

- 一个 actor 对应一个串行处理循环，避免同 actor 内部并发竞争。
- 按需创建 actor，支持生命周期管理、定时 tick、空闲回收。
- 请求消息可通过 `TransSendBack` 直接回包。

说明：

- `message.MsgLayerType` 中包含 `Stateless` 与 `Stateful` 两种层类型。
- 当前仓库主要提供 `Stateful` 的完整实现；`Stateless` 可按同一接口自行实现并注册。

### 3.4 业务处理层（消息处理函数）

业务处理函数不直接耦合 Transport/Dispatcher，而是通过消息注册表关联：

- `message.RegisterMsgInfo(...)`：注册消息元信息。
- `message.RegisterMsgHandle(msgID, handle, msgLayerType)`：绑定处理函数和目标层。

Dispatcher 根据 `msgID -> MsgProtoInfo -> MsgLayerType` 路由到对应处理层。

## 4. 关键数据结构

### 4.1 `transport.TransportDelivery`

Transport 上送给 Dispatcher 的载体：

- `Pkg`：解码后的包头/包体。
- `TransSendBack`：发送响应的回调。

### 4.2 `dispatcher.DispatcherDelivery`

Dispatcher 内部载体，基于 `TransportDelivery` 增强：

- `ProtoInfo`：消息协议元信息（消息类型、目标层等）。
- `ResOpts`：回包可选参数。

### 4.3 `handler.Delivery`

MsgLayer 看到的是抽象接口，而非 Dispatcher 具体类型，降低层间耦合。

## 5. 端到端消息流程

以 TCP 入站消息为例：

1. `TCPTransport` 读取 prehead + payload，解码为 `TransRecvPkg`。
2. 构建 `TransportDelivery{Pkg, TransSendBack}`。
3. 调用 `Dispatcher.OnRecvTransportPkg(...)`。
4. Dispatcher 通过 `msgID` 查询 `message.GetProtoInfo`。
5. 经过过滤器链（消息过滤、限流等）。
6. 按 `MsgLayerType` 选中目标 `MsgLayerReceiver`。
7. 目标层处理消息（例如 stateful 层投递到 actor mailbox）。
8. 若为请求消息，处理层通过 `TransSendBack` 返回响应。

## 6. 最小装配示例（基于当前 API）

下面示例演示真实 API 的装配方式，突出网络主链路；业务细节省略。

```go
package main

import (
	"fmt"

	"github.com/linchenxuan/strix/network/dispatcher"
	"github.com/linchenxuan/strix/network/handler"
	"github.com/linchenxuan/strix/network/message"
	"github.com/linchenxuan/strix/network/transport"
	"github.com/linchenxuan/strix/network/transport/tcp"
	"google.golang.org/protobuf/proto"
)

// 适配 message 包的全局函数为 MsgCreator 接口。
type msgCreator struct{}

func (msgCreator) CreateMsg(msgID string) (proto.Message, error) { return message.CreateMsg(msgID) }
func (msgCreator) ContainsMsg(msgID string) bool                 { return message.ContainsMsg(msgID) }

// 示例 MsgLayerReceiver。
type demoLayer struct{}

func (demoLayer) OnRecvDispatcherPkg(delivery handler.Delivery) error {
	fmt.Println("recv msg:", delivery.GetPkgHdr().GetMsgID())
	return nil
}

func main() {
	// 1) 创建 transport
	tp, err := tcp.NewTCPTransport(&tcp.TCPTransportCfg{
		Addr:            ":9000",
		IdleTimeout:     60,
		SendChannelSize: 1024,
		MaxBufferSize:   1 << 20,
	})
	if err != nil {
		panic(err)
	}

	// 2) 创建 dispatcher
	dp, err := dispatcher.NewDispatcher(&dispatcher.DispatcherConfig{
		RecvRateLimit: 10000,
		TokenBurst:    1000,
	}, []transport.Transport{tp})
	if err != nil {
		panic(err)
	}

	// 3) 注册消息层（这里用示例 layer）
	if err := dp.RegisterMsglayer(message.MsgLayerType_Stateless, demoLayer{}); err != nil {
		panic(err)
	}

	// 4) 启动 transport，并把 dispatcher 作为上层 handler 注入
	if err := tp.Start(transport.TransportOption{
		Creator: msgCreator{},
		Handler: dp,
	}); err != nil {
		panic(err)
	}

	select {}
}
```

## 7. 配置与扩展建议

- 新增传输协议：实现 `transport.Transport`，并在 `Start` 中接入 `TransportOption.Handler`。
- 新增消息处理模型：实现 `handler.MsgLayerReceiver`，并调用 `RegisterMsglayer`。
- 新增治理能力：通过 `DispatcherFilter` 扩展认证、灰度、审计等中间逻辑。
- 新增消息类型：注册 `MsgProtoInfo` 与对应 handler，确保 `msgID`、`MsgLayerType` 一致。

## 8. 总结

当前代码下，`strix` 网络系统的真实架构是以 `Transport -> Dispatcher -> MsgLayer` 为骨干，辅以消息注册表完成协议到业务处理函数的映射。它将网络收发、分发治理与业务执行模型清晰分层，便于在高并发场景下持续演进和扩展。
