// Package dispatcher provides the central message routing and processing engine for the Strix
// network stack. This file specifically defines the filter (middleware) mechanism that allows
// for pre-processing of messages before they reach their final handler.
package dispatcher

import (
	"errors"

	"github.com/linchenxuan/strix/network/message"
	"github.com/linchenxuan/strix/network/pb"
	"github.com/linchenxuan/strix/network/transport"
)

// DispatcherFilterHandleFunc defines the signature for the final handler function in a filter chain.
// After a message has passed through all registered filters, this function is called to perform
// the core message processing.
type DispatcherFilterHandleFunc func(dd *DispatcherDelivery) error

// DispatcherFilter defines the contract for a filter in the dispatcher's processing pipeline.
// A filter is a function that intercepts a message, performs some logic (e.g., logging,
// authentication, rate-limiting), and then calls the next function in the chain.
// This implements the chain-of-responsibility or middleware pattern.
type DispatcherFilter func(dd *DispatcherDelivery, next DispatcherFilterHandleFunc) error

// DispatcherFilterChain is a slice of DispatcherFilters that represents the ordered
// processing pipeline for incoming messages.
type DispatcherFilterChain []DispatcherFilter

// Handle executes the chain of filters for a given message. It starts with the first filter
// and passes a function that, when called, will execute the rest of the chain.
// This is achieved through recursion. If the chain is empty, it directly calls the final
// handler `f`.
func (fc DispatcherFilterChain) Handle(dd *DispatcherDelivery, f DispatcherFilterHandleFunc) error {
	if len(fc) == 0 {
		return f(dd)
	}
	// Execute the first filter, passing a closure that will handle the rest of the chain.
	return fc[0](dd, func(dd *DispatcherDelivery) error {
		return fc[1:].Handle(dd, f)
	})
}

// reloadMsgFilterCfg atomically updates the internal message filter map from a configuration object.
// This is used to support hot-reloading of filter rules. Note that this method itself is not
// concurrently safe; it must be called from within a write-locked section.
func (dp *Dispatcher) reloadMsgFilterCfg(cfg *MsgFilterPluginCfg) {
	// Create a new map to avoid modifying the old one while it might be in use.
	newFilterMap := make(map[string]struct{})
	for _, msgName := range cfg.MsgFilter {
		newFilterMap[msgName] = struct{}{}
	}
	dp.msgFilterMap = newFilterMap
}

// msgFilter is a built-in filter implementation that blocks messages based on their ID.
// It checks if a message's ID is present in the dispatcher's `msgFilterMap`. If it is,
// the filter chain is short-circuited. For request-type messages, an empty response is
// sent back to the originator.
func (dp *Dispatcher) msgFilter(d *DispatcherDelivery, f DispatcherFilterHandleFunc) error {
	if d.Pkg.PkgHdr == nil {
		return errors.New("dispatcher filter: received delivery with nil PkgHdr")
	}

	hdr := d.Pkg.PkgHdr
	msgID := hdr.GetMsgID()

	// Check if the message ID is in the filter map.
	if _, ok := dp.msgFilterMap[msgID]; !ok {
		// If not filtered, proceed to the next handler in the chain.
		return f(d)
	}

	// The message is filtered. For requests, we should send a default response back.
	pi, ok := message.GetProtoInfo(msgID)
	if !ok {
		// Log this? Or maybe just drop. Returning an error might be too noisy.
		// For now, returning an error to indicate a configuration mismatch.
		return errors.New("dispatcher filter: message " + msgID + " is filtered but has no proto info")
	}

	// Only send a response if the filtered message was a request that expects a response.
	if !pi.IsReq() {
		return nil // Message filtered and dropped, chain stops here.
	}

	// Construct and send an empty response.
	resPkg := &transport.TransSendPkg{
		PkgHdr: &pb.PackageHead{
			MsgID:      pi.ResMsgID,
			DstActorID: hdr.GetSrcActorID(),
			SrcActorID: hdr.GetDstActorID(),
			SvrPkgSeq:  hdr.GetSvrPkgSeq(),
			CliPkgSeq:  hdr.GetCliPkgSeq(),
		},
	}

	if d.TransSendBack != nil {
		return d.TransSendBack(resPkg)
	}

	// If no send-back function is available, we just drop the message.
	return nil
}
