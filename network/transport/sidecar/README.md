# Sidecar 传输插件

`sidecar` 包实现了一个基于本地 sidecar 进程的传输层插件。它采用“控制平面 + 数据平面”分离的通信设计：

- 控制平面：`Unix Domain Socket (UDS)`。
- 数据平面：`Shared Memory (SHM)` 环形队列。

## UDS 是什么

UDS 是 `Unix Domain Socket`（Unix 域套接字）：

- 编程模型与 TCP socket 类似，支持连接、读写、超时、请求-响应。
- 只用于同机进程通信，不经过 IP 网络栈。
- 更适合本地控制信令，通常开销更低、时序和状态更清晰。

本项目里 UDS 路径由 `SidecarConfig.getUnixSockPath()` 给出，连接由 `net.DialUnix("unix", ...)` 建立。

## 为什么控制面走 UDS，数据面走 SHM

这是基于代码实现的明确分工，而不是文档约定。

### 控制面走 UDS

控制面消息（注册、反注册、心跳、切换 pipe）都走 UDS：

- 注册：`register()` 写 `PBMT_REQ_UNIXSOCK_REGISTER`，读 `PBMT_RSP_UNIXSOCK_REGISTER`。
- 心跳：`updateUnixMsg()` 周期发送 `PBMT_NTY_UNIXSOCK_HEATBEAT`。
- 迁移控制：`NtfChgRoute()` 发控制消息，`tryReadAndHandleUxSockMsg()` 处理回包。

为何适合：

- 控制消息量小但语义强，需要可靠请求-响应和连接状态机。
- `readFull/writeFull` 明确保证“读满/写满”，并配合 deadline 与重连状态流转，行为可控。

### 数据面走 SHM

业务消息（服务间消息包）最终写入共享内存队列：

- 发送：`SendToServer()` -> `mWriteShm()` -> `shmChannel.writeQueue.mWrite(...)`。
- 接收：`recv()` 循环 `readShm()` -> `DecodeSSMsgWithoutBody()` -> 分发上层 handler。

为何适合：

- 数据面追求高吞吐、低延迟、低系统调用成本。
- SHM ring queue 使用 head/tail 原子索引，支持连续空间分配与回绕，尽量减少额外拷贝。

## 核心组件

- `plugin.go`: 插件工厂，创建/销毁 `Sidecar`，并完成 shmpipe 选择、加锁、续租启动。
- `sidecar.go`: Sidecar 主逻辑，管理状态机、UDS 控制通道、SHM 收发与迁移行为。
- `shmchannel.go`: SHM ring queue 实现，负责 mmap、消息头校验、读写与空间管理。
- `config.go`: Sidecar 配置定义（shm 文件、uds 名称、心跳/重试参数等）。
- `unixsock.go`: UDS 头编解码与 readFull/writeFull。

## 生命周期与状态机

`Sidecar` 的核心由 `run` goroutine 驱动：

1. 初始化（`plugin.go`）
- 选择可用 shmpipe：`GetAvailableShmpipeIdx`。
- 锁定 shmpipe：`LockShmpipe`。
- mmap 挂载读写 shm 文件。
- 启动 `run` 与 `recv` goroutine。
- 启动锁续租 goroutine：`RenewShmpipePeriodically`。

2. 运行（`sidecar.go`）
- `_stateInited` / `_stateConnecting`：尝试 UDS 注册。
- `_stateSucceed`：周期心跳、处理控制消息、上报使用统计。
- `_stateStopped`：退出。

3. 接收（`recv`）
- 从 SHM 读队列拉取消息。
- 解码并通过 `handler.OnRecvTransportPkg` 分发。

## 消息收发流程

- 发送业务消息
1. 上层调用 `SendToServer`。
2. 编码消息后写入 SHM 写队列（`mWriteShm`）。

- 接收业务消息
1. `recv` 从 SHM 读队列读取二进制数据。
2. 解码为消息包。
3. 分发到上层处理器。

## 关键配套机制

- 消息有效性治理：SHM 消息头包含 `validation/version/msgTime`，支持损坏校验、版本控制、过期丢弃。
- Pipe 续租保活：锁文件定期续租，续租失败会尝试重锁，严重失败触发停止信号。
- 迁移协同：消息元数据附带 `ShmPipeIdx`，配合 `IsToCursvr()` 判断当前实例是否应处理该消息。

## 服务迁移支持

迁移期可存在新旧实例。插件通过以下机制避免消息丢失和误处理：

- 检查旧实例锁状态：`CheckOldsvrStatus()`。
- 路由切换通知：`NtfChgRoute()`。
- 按 `ShmPipeIdx` 判定消息归属，不匹配时可转发。
