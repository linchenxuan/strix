# Network Architecture

The network layer of `strix` is designed to provide a high-performance, scalable, and protocol-agnostic communication foundation for real-time multiplayer games. Through a layered, interface-oriented design, it cleanly decouples underlying network connection management, network session maintenance, and message routing/dispatch from upper-level game business logic.

## Design Philosophy

### 1. Layering & Decoupling
The network layer is carefully designed into independent layers, with each layer communicating with others through interfaces.
- **Transport Layer (`Transport`)**: A unified interface responsible for all network interactions. Depending on its specific implementation, it can act as a **listener** to accept new connections or represent an **individual connection** to handle data transmission and reception.
- **Network Entity Layer (`Entity`)**: An abstraction of a client network session. It manages network-related states for that connection (e.g., whether it is authenticated) and can encapsulate the underlying `Transport`. The `Entity`'s responsibilities are limited to the network level.
- **Dispatch Layer (`Dispatcher`)**: Acts as a bridge between the transport layer and the business logic layer (indirectly through the `Entity`). It receives complete message packets from a `Transport` connection, parses the message header, and routes the message to the corresponding `Entity`.
- **Game Logic Layer**: This part (usually not belonging to the `network` package) will drive the real entities of the game world (`GameEntity`) by interacting with the `Entity`.

This layered design allows any layer to be replaced independently. For example, we can easily switch the underlying transport from TCP to KCP without changing any logic in the `Dispatcher` and `Entity`.

### 2. Interface-Driven
The entire network architecture is built around a set of core interfaces (`Transport`, `Dispatcher`, `Entity`). This design encourages users to provide custom implementations according to their specific needs, giving the framework great flexibility.

### 3. Session-Oriented
The core focus of the network layer is managing client sessions. Each client connection is mapped to an `Entity`, which carries all network-level information and operations for that connection, acting as a bridge between the connection and the upper-level business logic.

## Core Components

- **`Transport`**: A unified network interface with a dual role:
    - **Listener Role**: An implementation of `Transport` (e.g., `tcp.Listener`) is responsible for listening on a specified network address. When it accepts a new connection, it creates a new `Transport` instance for that connection (connection role).
    - **Connection Role**: Represents an individual client connection, encapsulating all underlying I/O operations, responsible for reading, writing, and framing, and delivering complete message packets to the `Dispatcher`.
- **`Entity`**: A network entity. It is an abstraction of a client session at the network layer. An `Entity` is associated with a `Transport` (connection role) and manages the network state of that session. **It does not directly contain game logic but is responsible for handling network-level requests (like heartbeats, authentication) and forwarding messages that require game logic processing to the upper-level game logic entity (`GameEntity`).**
- **`EntityManager`**: The network entity manager. Responsible for the lifecycle management of `Entity` instances (creation, retrieval, destruction).
- **`Dispatcher`**: The dispatcher. It implements the core logic of "where network messages should go." It receives raw message packets from the `Transport` (connection role), parses routing information, and delivers them to the corresponding `Entity`.

## Data Flow Between Modules

A typical processing flow for an upstream message (from client to server) is as follows:
1.  **`Transport` (listener)** accepts a client `net.Conn` connection and creates a new **`Transport` (connection)** instance for it.
2.  **`EntityManager`** creates and manages an `Entity` instance for the new `Transport` (connection).
3.  **`Transport` (connection)** reads the byte stream from `net.Conn` and assembles it into a complete message packet (`[]byte`) according to framing rules.
4.  The `Transport` (connection) passes this message packet to the **`Dispatcher`** for processing.
5.  The **`Dispatcher`** receives the `Transport` (connection) instance and the message packet:
    -   Parses the message packet to get routing information and the business message.
    -   Looks up the target **`Entity`** through the `EntityManager` based on the routing information.
    -   Delivers the business message to the target `Entity`.
6.  The **`Entity`** receives the business message and forwards it to the upper-level **`GameEntity`** (game logic entity).
7.  The **`GameEntity`** updates its state and may send a response message back to the client through the `Entity`.
8.  The **`Entity`** receives the response message and sends it out through its associated `Transport` (connection).
9.  The **`Transport` (connection)** receives the response message, performs serialization and framing, and then sends it back to the client.
10. **Connection Closure**: The client disconnects or the server actively closes the `Transport` (connection). The `onClose` callback is triggered, notifying the `EntityManager` to clean up the related `Entity`.

## Quick Start

The following is a conceptual example showing how to assemble the various network components in the `strix` framework and start a server.

```go
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/linchenxuan/strix/network"
    // Assuming we have these concrete implementation packages
	"github.com/linchenxuan/strix/transport/tcp" // A specific TCP transport implementation
	"github.com/linchenxuan/strix/network" // DefaultDispatcher is now under the network package

)

func main() {
    // 1. Create a network entity manager
    entityManager := network.NewInMemoryEntityManager()
    log.Println("Network entity manager created.")

    // 2. Create a message dispatcher
    appDispatcher := network.NewDefaultDispatcher()
    log.Println("Message dispatcher created.")

    // 3. Create and configure the network listener
    // Note: We are creating a listener implementation of the Transport interface
    listener := tcp.NewListener(":8080")
    log.Println("TCP listener created, listening on :8080.")

    // 4. Define callback functions
    // When a new connection is made, an Entity is created to manage it
    onNewConnection := func(conn network.Transport) {
        log.Printf("New connection established: %s (ID: %d)", conn.RemoteAddr(), conn.ID())
        // Create an Entity for the new Transport connection
        _, err := entityManager.CreateEntity(conn)
        if err != nil {
            log.Printf("Failed to create Entity for connection %d: %v", conn.ID(), err)
            conn.Close()
        }
    }

    // When a connection receives a message, it calls the Dispatcher for processing
    onMessage := func(conn network.Transport, message []byte) {
        appDispatcher.Dispatch(conn, message)
    }

    // When a connection is closed, clean up the related Entity
    onClose := func(conn network.Transport) {
        log.Printf("Connection closed: %s (ID: %d)", conn.RemoteAddr(), conn.ID())
        entityManager.RemoveEntity(entityManager.GetEntityByTransportID(conn.ID()).ID())
    }

    // 5. Start the listener
    // The Listen method will block or run in the background and start accepting connections
    go func() {
        err := listener.Listen(onNewConnection, onMessage, onClose)
        if err != nil {
            log.Fatalf("Failed to start listener: %v", err)
        }
    }()

    log.Println("Server started, waiting for client connections...")

    // Wait for an interrupt signal to achieve graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    log.Println("Server is shutting down...")
    listener.Stop() // Stopping the listener will close all accepted connections
    log.Println("Server shut down successfully.")
}
```

## Summary

Through clear layering and interface abstraction, the `strix` network layer provides a robust and flexible underlying communication framework. The introduction of the unified `Transport` interface and the `Entity` clearly defines the boundary between network session management and game business logic. This design allows developers to focus on their respective domains while ensuring the framework's advantages in terms of functional extension and maintenance.
