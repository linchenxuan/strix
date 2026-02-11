// Package stateful provides the implementation for the stateful message processing layer.
// This file defines the HandleContext, which encapsulates all necessary information and
// services for a message handler to process a single message.
package stateful

import (
	"fmt"

	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/network/dispatcher" // Import dispatcher package
	"github.com/linchenxuan/strix/network/message"    // Import message package
	"github.com/linchenxuan/strix/network/transport"  // Import transport package
	"google.golang.org/protobuf/proto"
)

// beforeSendPkgToClientFunc defines the signature for a callback function that can
// inspect or modify a package just before it is sent to a client.
type beforeSendPkgToClientFunc func(resPkg *transport.TransSendPkg) error

// HandleContext is a per-request object that encapsulates all the state and services
// needed by a message handler. It acts as a form of dependency injection, decoupling the
// business logic within a handler from the underlying framework services like transport layers
// and message registries.
type HandleContext struct {
	log.Logger                            // Embedded logger for context-aware logging.
	*dispatcher.DispatcherDelivery        // Embedded delivery info, providing access to the raw incoming package.
	actorID                        uint64 // The ID of the actor responsible for handling this message.

	msgLayer              *StatefulMsgLayer         // A reference to the parent stateful message layer.
	ntfSender             *message.NtfSender        // A helper service for sending notifications.
	beforeSendPkgToClient beforeSendPkgToClientFunc // A callback executed before a package is sent.
}

// newHandleContext creates a new, partially initialized HandleContext.
// The context is fully populated by the StatefulMsgLayer before being passed to an actor.
func newHandleContext(aid uint64, dd *dispatcher.DispatcherDelivery, // Changed net.DispatcherDelivery to dispatcher.DispatcherDelivery
	logger log.Logger) *HandleContext {
	return &HandleContext{
		actorID:            aid,
		DispatcherDelivery: dd,
		Logger:             logger,
	}
}

// GetActorID returns the ID of the actor associated with this context.
func (ctx *HandleContext) GetActorID() uint64 {
	return ctx.actorID
}

// SetActorID manually overrides the actor ID in the context. This might be used in
// message handlers that need to forward the request to a different actor.
func (ctx *HandleContext) SetActorID(aid uint64) {
	ctx.actorID = aid
	ctx.Info().Uint64("new_actor_id", aid).Msg("manual override of actor ID in context")
}

// NtfClient is a convenience method for sending a notification to a client.
// It automatically sets the source actor ID to the current context's actor ID
// before forwarding the package to the notification sender service.
func (ctx *HandleContext) NtfClient(pkg transport.TransSendPkg) { // Changed net.TransSendPkg to transport.TransSendPkg
	// Set the source of the notification to the current actor.
	pkg.SetSrcActorID(ctx.GetActorID())

	err := ctx.ntfSender.NtfClient(pkg, ctx.actorID, ctx.msgLayer.csTransport)
	if err != nil {
		ctx.Error().Str("msgid", pkg.PkgHdr.GetMsgID()).Err(err).Msg("failed to send client notification")
	}
}

// NtfServer is a convenience method for sending a notification to another server.
// It automatically sets the source actor ID and uses the server-to-server transport.
func (ctx *HandleContext) NtfServer(pkg transport.TransSendPkg) { // Changed net.TransSendPkg to transport.TransSendPkg
	pkg.SetSrcActorID(ctx.GetActorID())
	if err := ctx.ntfSender.NtfServer(pkg, ctx.msgLayer.ssTransport); err != nil {
		ctx.Error().Err(err).Str("msgid", pkg.PkgHdr.GetMsgID()).Msg("failed to send server notification")
	}
}

// sendBack is the standard method for a handler to send a response to a request.
// It constructs a response package using metadata from the original request and sends
// it back to the originator via the transport's send-back mechanism.
func (ctx *HandleContext) sendBack(code int32, body proto.Message) (err error) {
	if ctx.TransSendBack == nil {
		return fmt.Errorf("cannot send back for MsgID '%s': no send-back function available", ctx.Pkg.PkgHdr.GetMsgID())
	}

	// Create a response package, automatically routing it back to the original sender.
	resPkg, err := message.NewResPkg(ctx.Pkg, ctx.ProtoInfo.GetResMsgID(), code, body, ctx.ResOpts...) // Changed net.NewResPkg to message.NewResPkg
	if err != nil {
		return fmt.Errorf("failed to create response package: %w", err)
	}

	// Execute the pre-send callback if it's registered.
	if ctx.beforeSendPkgToClient != nil {
		if err = ctx.beforeSendPkgToClient(resPkg); err != nil {
			return fmt.Errorf("beforeSendPkgToClient callback failed: %w", err)
		}
	}

	// Send the package via the transport.
	err = ctx.TransSendBack(resPkg)
	return err
}
