// Package transport defines the interfaces and foundational data structures for network
// communication. It establishes the contracts for different transport protocols (like TCP,
// WebSocket, etc.) and defines how they interact with the upper layers of the network stack.
package transport

// Transport is the fundamental interface for any network transport implementation.
// It defines the basic lifecycle methods for starting and stopping the transport service.
type Transport interface {
	// Start initializes and brings the transport online. It takes a TransportOption
	// which provides necessary dependencies like a message factory and a handler
	// for incoming messages. This method should be non-blocking.
	Start(opt TransportOption) error

	// StopRecv gracefully stops the transport from accepting new incoming messages.
	// This allows the system to drain in-flight requests before a full shutdown.
	// It should not close existing connections immediately.
	StopRecv() error

	// Stop immediately and completely shuts down the transport service.
	// It closes the listener and all active connections, and releases all resources.
	Stop() error
}

// CSTransport specializes the base Transport interface for Client-to-Server communication.
// It adds methods specific to sending messages to individual game clients.
type CSTransport interface {
	Transport
	// SendToClient sends a package to a specific client. The implementation is
	// responsible for looking up the client's connection and transmitting the data.
	SendToClient(pkg TransSendPkg) error
}

// SSTransport specializes the base Transport interface for Server-to-Server communication.
// It adds methods for sending messages to other services within the distributed cluster.
type SSTransport interface {
	Transport
	// SendToServer sends a package to another server. The implementation handles
	// service discovery, routing, and the underlying inter-process communication.
	SendToServer(pkg *TransSendPkg) error
}

// SendBackFunc is a function type that defines a callback for sending a response.
// It encapsulates the logic required to send a reply back on the same connection or
// channel from which the original request was received.
type SendBackFunc func(pkg *TransSendPkg) error

// TransportDelivery is the data structure that carries a received message from the
// transport layer up to the next layer (the dispatcher). It's a data transfer object
// that bundles the message payload with the means to reply to it.
type TransportDelivery struct {
	// TransSendBack is a callback function that can be invoked by the application layer
	// to send a response back to the originator of the message.
	TransSendBack SendBackFunc
	// Pkg is the received package, containing the headers and the raw or decoded body.
	Pkg *TransRecvPkg
}

// DispatcherReceiver defines the contract for the component that sits above the transport layer.
// Any struct that implements this interface can receive and process messages from a transport.
// In this framework, this is typically implemented by the Dispatcher.
type DispatcherReceiver interface {
	// OnRecvTransportPkg is the callback method invoked by the transport layer for each
	// successfully received and framed message. The transport layer's responsibility
	// ends after calling this method.
	OnRecvTransportPkg(td *TransportDelivery) error
}
