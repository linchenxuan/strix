// Package message defines the core data structures, interfaces, and factory functions
// for network messages within the Strix framework. It establishes the contracts for
// different message types (e.g., RPC, Notification) and provides utilities for
// constructing transport-level packages with appropriate routing headers.
package message

import (
	"errors"

	"github.com/linchenxuan/strix/network/pb"
	"github.com/linchenxuan/strix/network/transport"
	"github.com/linchenxuan/strix/runtime"
	"google.golang.org/protobuf/proto"
)

// MsgReqType enumerates the different kinds of messages based on their role in
// a communication pattern.
type MsgReqType int

const (
	// MRTNone indicates an invalid or uninitialized message type.
	MRTNone MsgReqType = iota
	// MRTReq represents a request message that expects a corresponding response.
	MRTReq
	// MRTRes represents a response message sent in reply to a previous request.
	MRTRes
	// MRTNtf represents a notification message that is "fire-and-forget" and does not expect a response.
	MRTNtf
)

// MsgLayerType enumerates the types of application layers that can handle messages.
// This allows the dispatcher to route messages to the correct processing logic
// based on whether they require access to state.
type MsgLayerType uint8

const (
	// MsgLayerType_None indicates an invalid or uninitialized layer type.
	MsgLayerType_None MsgLayerType = iota
	// MsgLayerType_Stateless is for messages that can be handled without any session
	// or actor-specific context. These are typically generic, shared services.
	MsgLayerType_Stateless
	// MsgLayerType_Stateful is for messages that are tied to a specific actor's
	// state, such as a player, a game room, or another stateful entity.
	MsgLayerType_Stateful
	// MsgLayerType_Max is a sentinel value used for validating the range of layer types.
	MsgLayerType_Max
)

// MsgProtoInfo holds all metadata associated with a specific message protocol type.
// The framework uses this struct as a "descriptor" to understand how to create, handle,
// and route a message based on its ID.
type MsgProtoInfo struct {
	// New is a factory function that returns a new, empty instance of the message.
	New func() proto.Message
	// MsgID is the unique string identifier for this message type.
	MsgID string
	// ResMsgID is the message ID of the corresponding response, if this is a request message.
	ResMsgID string
	// MsgReqType categorizes the message as a request, response, or notification.
	MsgReqType MsgReqType
	// IsCS indicates whether this is a Client-to-Server message.
	IsCS bool
	// MsgHandle is a reference to the handler function that processes this message.
	// It is of type `any` to allow for different handler function signatures.
	MsgHandle any
	// MsgLayerType specifies which application layer (e.g., stateless, stateful) should handle this message.
	MsgLayerType // Changed to handlers.MsgLayerType
	// HasRecvSvr is a flag indicating if the message is handled by a receiving server.
	HasRecvSvr bool
}

// IsNtf returns true if the message is a notification.
func (pi *MsgProtoInfo) IsNtf() bool {
	return pi != nil && pi.MsgReqType == MRTNtf
}

// IsSSReq returns true if the message is a server-to-server request.
func (pi *MsgProtoInfo) IsSSReq() bool {
	return pi != nil && pi.MsgReqType == MRTReq && !pi.IsCS
}

// IsSSRes returns true if the message is a server-to-server response.
func (pi *MsgProtoInfo) IsSSRes() bool {
	return pi != nil && pi.MsgReqType == MRTRes && !pi.IsCS
}

// IsReq returns true if the message is any type of request (client-server or server-server).
func (pi *MsgProtoInfo) IsReq() bool {
	return pi != nil && pi.MsgReqType == MRTReq
}

// IsRes returns true if the message is any type of response (client-server or server-server).
func (pi *MsgProtoInfo) IsRes() bool {
	return pi != nil && pi.MsgReqType == MRTRes
}

// GetResMsgID returns the message ID of the expected response. Returns empty string if not a request.
func (pi *MsgProtoInfo) GetResMsgID() string {
	if pi != nil {
		return pi.ResMsgID
	}
	return ""
}

// GetMsgID returns the unique identifier for this message type.
func (pi *MsgProtoInfo) GetMsgID() string {
	if pi != nil {
		return pi.MsgID
	}
	return ""
}

// GetMsgHandle returns the handler function registered for this message type.
func (pi *MsgProtoInfo) GetMsgHandle() any {
	if pi != nil {
		return pi.MsgHandle
	}
	return nil
}

// MsgCreator defines a factory interface for creating protobuf message instances from a message ID.
// This allows the network layer to dynamically instantiate message objects without being
// tightly coupled to concrete message types.
type MsgCreator interface {
	// CreateMsg constructs a new proto.Message instance for the given message ID.
	CreateMsg(msgID string) (proto.Message, error)
	// ContainsMsg checks if this creator knows how to create a message for the given ID.
	ContainsMsg(msgID string) bool
}

// NewPkgHead is a factory function that creates a new PkgHead with a specified message ID.
func NewPkgHead(msgID string) *pb.PackageHead {
	return &pb.PackageHead{
		MsgID: msgID,
	}
}

// AsuraMsg is the base interface for all messages transferrable within the Strix framework.
// It combines the standard proto.Message interface with methods for framework-level metadata.
type AsuraMsg interface {
	proto.Message
	// MsgID returns the unique string identifier for the message protocol type.
	MsgID() string
	// IsRequest returns true if the message is a request that expects a response.
	IsRequest() bool
	// IsResponse returns true if the message is a response to a previous request.
	IsResponse() bool
}

// SCNtfMsg is the interface for Server-to-Client Notification messages.
// These are "fire-and-forget" messages sent from the server to the client.
type SCNtfMsg interface {
	AsuraMsg
	// SCNtfMsgID returns the message's unique ID.
	SCNtfMsgID() string
	// NtfClient sends the notification to a specific client.
	// NtfClient(sender NtfPkgSender, dstEntity uint32, dstActorID uint64, opts ...transport.TransPkgOption)
	// DeferNtfClient schedules the notification to be sent to a client at a later time.
	// DeferNtfClient(sender DeferPkgSender, dstEntity uint32, dstActorID uint64, opts ...transport.TransPkgOption)
}

// SSMsg is the interface for Server-to-Server messages.
// It's a marker interface for messages intended for inter-service communication.
type SSMsg interface {
	AsuraMsg
	// IsSS is a marker method that confirms the message is for server-to-server communication.
	IsSS() bool
}

// RPCReqMsg is the interface for RPC-style request messages.
// It defines the contract for messages that follow the request-reply pattern.
type RPCReqMsg interface {
	AsuraMsg

	// ResMsgID returns the message ID of the corresponding response message.
	ResMsgID() string
	// CreateResMsg is a factory method that creates an empty instance of the corresponding response message.
	CreateResMsg() proto.Message
}

// NewResPkg creates a response package intended to be sent back to the originator of a request.
// It populates the routing and package headers based on the incoming request package, ensuring
// the response is correctly routed back to the source.
func NewResPkg(reqPkg *transport.TransRecvPkg, resMsgID string, retCode int32,
	resBody proto.Message, opts ...transport.TransPkgOption) (*transport.TransSendPkg, error) {
	if reqPkg == nil {
		return nil, errors.New("NewResPkg: reqPkg is nil")
	}
	if resMsgID == "" {
		return nil, errors.New("NewResPkg: resMsgID is empty")
	}

	// Create a package with a P2P route targeting the original sender.
	resPkg := &transport.TransSendPkg{
		RouteHdr: &pb.RouteHead{
			SrcEntityID: runtime.GetEntityID(),
			RouteType: &pb.RouteHead_P2P{
				P2P: &pb.P2PRoute{
					DstEntityID: reqPkg.RouteHdr.GetSrcEntityID(),
				},
			},
			MsgID: resMsgID,
		},
		PkgHdr: NewPkgHead(resMsgID),
		Body:   resBody,
	}

	// Populate response headers from the request.
	resPkg.PkgHdr.RetCode = retCode
	resPkg.PkgHdr.DstActorID = reqPkg.PkgHdr.GetSrcActorID()
	resPkg.PkgHdr.SrcActorID = reqPkg.PkgHdr.GetDstActorID()

	for _, opt := range opts {
		opt(resPkg)
	}
	return resPkg, nil
}

// NewSCNtfPkg builds a server-to-client notification package.
// It configures the package for delivery to a specific actor on a specific entity (client).
func NewSCNtfPkg(m SCNtfMsg, dstEntity uint32, dstActorID uint64, opts ...transport.TransPkgOption) transport.TransSendPkg {
	h := transport.TransSendPkg{
		PkgHdr: NewPkgHead(m.MsgID()),
		Body:   m,
	}
	for _, opt := range opts {
		opt(&h)
	}

	h.SetDstActorID(dstActorID)

	// If the destination entity is different from the current one, set up a P2P route header.
	if dstEntity != runtime.GetEntityID() {
		h.SetRouteHdr(&pb.RouteHead{
			SrcEntityID: runtime.GetEntityID(),
			RouteType: &pb.RouteHead_P2P{
				P2P: &pb.P2PRoute{
					DstEntityID: dstEntity,
				},
			},
		})
	}

	return h
}

// NewPostLocalPkg builds a package for local, in-process message delivery.
// This is an optimization for when a component needs to send a message to another
// component running within the same server instance. The route is set to the current entity.
func NewPostLocalPkg(m AsuraMsg, opts ...transport.TransPkgOption) transport.TransSendPkg {
	h := transport.TransSendPkg{
		RouteHdr: &pb.RouteHead{
			SrcEntityID: 0, // Source entity is not relevant for local messages.
			RouteType: &pb.RouteHead_P2P{
				P2P: &pb.P2PRoute{
					DstEntityID: runtime.GetEntityID(), // Target is the local entity.
				},
			},
			MsgID: m.MsgID(),
		},
		PkgHdr: NewPkgHead(m.MsgID()),
		Body:   m,
	}
	for _, opt := range opts {
		opt(&h)
	}
	return h
}

// NewP2PPkg builds a point-to-point (P2P) server-to-server package.
// This is used for sending a message directly to a specific server entity.
func NewP2PPkg(m SSMsg, dstID uint32, opts ...transport.TransPkgOption) transport.TransSendPkg {
	h := transport.TransSendPkg{
		RouteHdr: &pb.RouteHead{
			SrcEntityID: runtime.GetEntityID(),
			RouteType: &pb.RouteHead_P2P{
				P2P: &pb.P2PRoute{
					DstEntityID: dstID,
				},
			},
			MsgID: m.MsgID(),
		},
		PkgHdr: NewPkgHead(m.MsgID()),
		Body:   m,
	}

	for _, opt := range opts {
		opt(&h)
	}
	return h
}

// NewP2PPkgActor builds a point-to-point (P2P) server-to-server package that targets
// a specific actor within the destination server entity.
func NewP2PPkgActor(m SSMsg, dstEntityID uint32, dstActorID uint64, opts ...transport.TransPkgOption) transport.TransSendPkg {
	pkg := NewP2PPkg(m, dstEntityID, opts...)
	pkg.PkgHdr.DstActorID = dstActorID
	return pkg
}
