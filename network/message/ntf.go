// Package message defines the core data structures and interfaces for network messages.
// This file implements a helper for sending notification messages, encapsulating validation
// and routing logic.
package message

import (
	"errors"
	"fmt"
	"github.com/linchenxuan/strix/network/transport"
)

// Aliasing types from transport package
type (
	TransSendPkg = transport.TransSendPkg
	CSTransport  = transport.CSTransport
	SSTransport  = transport.SSTransport
)

// NtfSender is a helper service that provides methods for sending notification packages.
// It centralizes validation logic, ensuring that a message is a valid notification
// of the correct type (e.g., client-to-server vs. server-to-server) before passing it
// to the underlying transport.
type NtfSender struct {
	// No longer needs a MessageManager field as functions are package-level
}

// NewNtfSender creates a new NtfSender instance.
// Message lookups now use package-level functions.
func NewNtfSender() *NtfSender {
	return &NtfSender{}
}

// NtfClient sends a notification package to a specific client actor.
// It validates that the message is a registered Client-Server notification, sets the
// destination actor ID, and uses the provided CSTransport to send the package.
func (s *NtfSender) NtfClient(pkg TransSendPkg, aid uint64, cs CSTransport) error {
	if pkg.PkgHdr == nil {
		return errors.New("NtfClient: notification package header is nil")
	}
	if cs == nil {
		return fmt.Errorf("NtfClient: CSTransport is nil for MsgID '%s'", pkg.PkgHdr.GetMsgID())
	}

	msgID := pkg.PkgHdr.GetMsgID()
	info, ok := GetProtoInfo(msgID)
	if !ok || info == nil {
		return fmt.Errorf("NtfClient: no proto info found for MsgID '%s'", msgID)
	}

	if !IsCSNtfMsg(msgID) {
		return fmt.Errorf("NtfClient: MsgID '%s' is not a registered client-server notification", msgID)
	}

	// If a specific actor ID is provided, set it as the destination.
	if aid > 0 {
		pkg.PkgHdr.DstActorID = aid
	}

	return cs.SendToClient(pkg)
}

// NtfServer sends a notification package to another server.
// It validates that the message is a registered notification type and uses the
// provided SSTransport to send the package.
func (s *NtfSender) NtfServer(pkg TransSendPkg, ss SSTransport) error {
	if pkg.PkgHdr == nil {
		return errors.New("NtfServer: notification package header is nil")
	}

	msgID := pkg.PkgHdr.GetMsgID()
	info, ok := GetProtoInfo(msgID)
	if !ok || info == nil {
		return fmt.Errorf("NtfServer: no proto info found for MsgID '%s'", msgID)
	}

	// Any notification type is valid for server-to-server communication.
	if !IsNtfMsg(msgID) {
		return fmt.Errorf("NtfServer: MsgID '%s' is not a registered notification type", msgID)
	}

	return ss.SendToServer(&pkg)
}
