// Package sender defines the core data structures and interfaces for network messages.
// This file specifically defines a set of interfaces that abstract various message
// sending patterns, such as notification, RPC, and deferred sending.
package sender

import (
	"time"

	"github.com/linchenxuan/strix/network/transport"
)

// NtfPkgSender defines the contract for sending "fire-and-forget" notification messages.
// This pattern is used for sending information that does not require a direct response.
type NtfPkgSender interface {
	// NtfClient sends a notification package to a client. This is typically used for
	// pushing state updates or events from the server to a connected game client.
	NtfClient(pkg transport.TransSendPkg)

	// NtfServer sends a notification package to another server. This enables inter-service
	// communication for tasks like broadcasting state changes or cache invalidation.
	NtfServer(pkg transport.TransSendPkg)
}

// RPCPkgSender defines the contract for performing synchronous Remote Procedure Calls (RPC).
// It embeds the NtfPkgSender interface, providing both notification and request-response capabilities.
// This pattern is used when the sender must block and wait for a response before continuing.
type RPCPkgSender interface {
	NtfPkgSender

	// RPCServer performs a synchronous RPC call to another server and blocks until a response
	// is received or a timeout occurs. It returns the response package or an error.
	RPCServer(pkg transport.TransSendPkg) (resPkg transport.TransRecvPkg, _ error)

	// GetRPCDeadline retrieves the deadline for the current RPC operation. This is crucial for
	// propagating timeout information across distributed calls. It returns the deadline and
	// a boolean indicating whether a deadline is set.
	GetRPCDeadline() (time.Time, bool)
}

// ARPCPkgSender defines the contract for sending Asynchronous RPC messages.
// This non-blocking pattern allows the sender to continue processing immediately after
// sending a request, with the response being handled later by a callback function.
type ARPCPkgSender interface {
	// AsyncRPCToServer sends an RPC request to another server and provides a callback
	// function to handle the response. This is ideal for high-throughput scenarios
	// where the sender does not need to wait for the result.
	AsyncRPCToServer(callback func(transport.TransRecvPkg, error), pkg transport.TransSendPkg)
}

// DeferPkgSender defines the contract for deferring the sending of notification messages.
// This is used to queue notifications that should only be sent after the primary processing
// of a request (and its response) is complete, ensuring a specific message order.
type DeferPkgSender interface {
	// DeferNtfClient queues a notification to be sent to a client. For example, after a
	// player buys an item, the "success" response is sent first, and then a deferred
	// "inventory updated" notification follows.
	DeferNtfClient(pkg transport.TransSendPkg)

	// DeferNtfServer queues a notification to be sent to another server, following the same
	// deferred execution pattern as DeferNtfClient.
	DeferNtfServer(pkg transport.TransSendPkg)
}

// PkgSender is a comprehensive, unified interface for sending packages.
// It combines capabilities for notifications, synchronous RPC, and asynchronous RPC.
// Unlike some of the other sender interfaces, its methods return an error, allowing the
// caller to immediately check if the message was successfully dispatched.
type PkgSender interface {
	// NtfClient sends a notification to a client. Returns an error if dispatch fails.
	NtfClient(pkg transport.TransSendPkg) error

	// NtfServer sends a notification to another server. Returns an error if dispatch fails.
	NtfServer(pkg transport.TransSendPkg) error

	// RPCServer performs a synchronous RPC call to another server.
	RPCServer(pkg transport.TransSendPkg) (transport.TransRecvPkg, error)

	// AsyncRPCToServer sends an asynchronous RPC and registers a callback for the response.
	// Returns an error if the request cannot be dispatched.
	AsyncRPCToServer(func(transport.TransRecvPkg, error), transport.TransSendPkg) error
}
