// Package transport defines the interfaces and foundational data structures for network
// communication. This file implements the functional options pattern for configuring
// message packages and transport instances.
package transport

import "github.com/linchenxuan/strix/network/pb"

// TransPkgOption defines the signature for a functional option that modifies a TransSendPkg.
// This pattern provides a clean, readable, and extensible way to configure message packages,
// avoiding constructors with a large number of parameters.
type TransPkgOption func(pkg *TransSendPkg)

// WithSrcActorID returns a TransPkgOption that sets the source actor ID in the package header.
// The source actor ID is essential for identifying the originator of a message, which can be
// used for routing responses or for logging and tracing.
func WithSrcActorID(id uint64) TransPkgOption {
	return func(pkg *TransSendPkg) {
		if pkg.PkgHdr != nil {
			pkg.PkgHdr.SrcActorID = id
		}
	}
}

// WithName returns a TransPkgOption that sets the message ID in the package header.
// The message ID is a unique string that identifies the protocol type of the message body,
// allowing the receiver to correctly decode and handle the message.
func WithName(msgID string) TransPkgOption {
	return func(pkg *TransSendPkg) {
		if pkg.PkgHdr != nil {
			pkg.PkgHdr.MsgID = msgID
		}
	}
}

// WithAreaID returns a TransPkgOption that sets the area ID in the message's routing header.
// This is used in routing strategies like Random or Broadcast to scope the operation to a
// specific geographical or logical group of servers.
func WithAreaID(areaID uint32) TransPkgOption {
	return func(pkg *TransSendPkg) {
		if pkg == nil || pkg.GetRouteHdr() == nil {
			return
		}
		// This type switch handles different kinds of route headers that may contain an AreaID.
		switch v := pkg.GetRouteHdr().GetRouteType().(type) {
		case *pb.RouteHead_Rand:
			if v.Rand != nil {
				v.Rand.AreaID = areaID
			}
		case *pb.RouteHead_BroadCast:
			if v.BroadCast != nil {
				v.BroadCast.AreaID = areaID
			}
		}
	}
}

// WithRetCode returns a TransPkgOption that sets the return code in the package header.
// The return code is used in response messages to indicate the outcome of the original request
// (e.g., success, failure, specific error condition).
func WithRetCode(ret int32) TransPkgOption {
	return func(pkg *TransSendPkg) {
		if pkg.PkgHdr != nil {
			pkg.PkgHdr.RetCode = ret
		}
	}
}

// WithSetVersion returns a TransPkgOption that sets the destination set version in the routing header.
// This is used in advanced routing scenarios, such as service sharding or canary deployments,
// to target a specific version of a service set.
func WithSetVersion(setVersion uint64) TransPkgOption {
	return func(pkg *TransSendPkg) {
		if pkg != nil && pkg.RouteHdr != nil {
			pkg.RouteHdr.DstSetVersion = setVersion
		}
	}
}

// TransportOption is a configuration struct that provides the core dependencies required
// by a Transport instance to operate.
type TransportOption struct {
	// Creator is a message factory responsible for instantiating correct message objects
	// from a message ID. The transport layer uses this to deserialize raw incoming data.
	Creator MsgCreator

	// Handler is the component that processes fully formed, incoming message packages.
	// This is typically the Dispatcher, which sits at the next layer of the network stack.
	Handler DispatcherReceiver
}
