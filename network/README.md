# Strix Network Layer Design Document

## 🏗️ Architecture Overview

The Strix network module uses a **four-layer architecture**, designed specifically for distributed game servers to provide high-performance, scalable network communication capabilities.

Transport Layer → Dispatcher Layer → Message Layer (MsgLayer) → Handler Layer

### Transport Layer
Core Design Features:

**Pluggable Architecture**

- Supports multiple transport protocols: TCP, HTTP, Sidecar, LS
- Dynamically loaded via a plugin factory pattern
- Supports coexistence of multiple transport protocols

**TCP Transport Implementation (`/transport/tcp/tcp.go`)**

- **Long connection mode**: Maintains a persistent connection for each UID.
- **Short connection mode**: Closes the connection after a request-response cycle.
- **Connection management**: Uses a `uidToConn` map to maintain connection state.
- **Secure communication**: Supports AES encrypted transmission.

**Message Format**

- **Pre-header (PreHead)**: Fixed length, contains message metadata.
- **Message Header (PkgHead)**: Contains routing, ID, Actor, and other information.
- **Message Body (Body)**: Protobuf-serialized business data.

### Dispatcher Layer
Core Responsibilities:

**Message Routing**

- Dispatches to different `MsgLayer` based on message type.
- Supports three modes: stateful, stateless, and async.
- Response messages are routed back to the original request via RPC ID.

**Flow Control**

- **Token bucket algorithm**: Implements a limit on the number of messages received per second.
- **Hot-reloadable**: Supports runtime adjustment of throttling parameters.
- **Message filter chain**: Supports custom filters.

**Timeout Management**

- **RPC timeout check**: Based on the `deadline` in the message header.
- **Timeout message rejection**: Avoids processing expired requests.

### Message Layer (MsgLayer)
Three runtime models:

**Stateless Model**

- **Concurrency model**: One goroutine per message.
- **Goroutine circuit breaker**: Limits maximum concurrency via `GoroutineFuse`.
- **Message filter chain**: Supports business-customized filters.
- **Applicable scenarios**: API gateways, computationally intensive services.

**Stateful Model**

- **Actor model**: One goroutine per business object.
- **Message serialization**: Messages within the same Actor are processed sequentially.
- **State management**: Supports Actor migration and persistence.
- **Applicable scenarios**: Game servers, session management.

**Async Model**

- **Single-threaded model**: Avoids concurrency issues.
- **Non-blocking processing**: Suitable for high-frequency, low-latency scenarios.
- **Callback mechanism**: Business-customized asynchronous logic.

## Key Design Highlights
1.  **Goroutine Management**

    - **Circuit breaker protection**: Prevents goroutine leaks.
    - **Monitoring and reporting**: Real-time statistics on goroutine usage.
    - **Dynamic adjustment**: Supports runtime modification of concurrency limits.

2.  **Message Processing Flow**
    Transport Layer Reception → 2. Decode Message → 3. Dispatcher Routing → 4. Message Layer Processing → 5. Business Logic → 6. Response Return

3.  **Extensibility Design**

    - **Filter chain**: Supports AOP programming patterns.
    - **Plugin mechanism**: Pluggable transport protocols.
    - **Configuration hot-reloading**: Modify configuration without restarting.

4.  **Monitoring and Observability**

    - **Metric collection**: Message processing time, error rate, concurrency count.
    - **Trace tracking**: Distributed tracing based on trace ID.
    - **Logging**: Complete request lifecycle logs.
