package handler

import (
	"github.com/linchenxuan/strix/network/message"
	"github.com/linchenxuan/strix/network/pb"
	"google.golang.org/protobuf/proto"
)

// Delivery defines the abstract interface for a message delivery object.
// This abstraction is used to decouple the message processing layer from the
// concrete dispatcher implementation, breaking circular dependencies.
type Delivery interface {
	GetPkgHdr() *pb.PackageHead          // Returns the raw package header.
	GetProtoInfo() *message.MsgProtoInfo // Returns protocol metadata for the message.
	GetSrcEntityID() uint32              // Returns the source entity ID from the route header.
	GetReqSrcActorID() uint64            // Returns the source actor ID from the package header.
	GetReqDstActorID() uint64            // Returns the destination actor ID from the package header.
	GetSrcClientVersion() int64          // Returns the source client version from the package header.
	DecodeBody() (proto.Message, error)  // Decodes the message body into a protobuf message.
	// We only expose what MsgLayerReceiver needs.
}

// MsgLayerReceiver defines the contract for application-level message handlers.
// Different layers of the application (e.g., stateless, stateful) implement this
// interface to process messages dispatched by the central Dispatcher.
type MsgLayerReceiver interface {
	// OnRecvDispatcherPkg processes a dispatched message delivery.
	// The `delivery` parameter is an abstract representation of the incoming message,
	// decoupling this handler from the concrete dispatcher implementation.
	OnRecvDispatcherPkg(delivery Delivery) error
}

// MsgLayer defines the abstract interface for a major message processing component.
// It combines the message handling capabilities of a MsgLayerReceiver with lifecycle
// management methods, ensuring that layers can be properly initialized and shut down.
type MsgLayer interface {
	// MsgLayerReceiver is an embedded interface that provides the OnRecvDispatcherPkg method,
	// making any MsgLayer a valid target for the dispatcher.
	MsgLayerReceiver

	// Init is called during server startup to initialize the message layer. This is where
	// a layer would set up its resources, such as database connections or internal state.
	Init() error

	// Shutdown is called before the server shuts down. This method should be used to
	// gracefully release any resources held by the layer.
	Shutdown()
}
