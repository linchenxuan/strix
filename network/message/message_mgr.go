// Package message defines the core data structures and interfaces for network messages.
// This file implements the MessageManager, which acts as a central registry for all
// message protocol definitions within the framework.
package message

import (
	"errors"

	"google.golang.org/protobuf/proto"
)

// MessageManager serves as a central registry for message protocol information.
// It maps message IDs to their corresponding metadata (MsgProtoInfo), which includes
// factory functions, handlers, and other properties. This allows the framework to
// dynamically handle various message types without hard-coding their details.
var propInfoMap = make(map[string]*MsgProtoInfo)

// RegisterMsgInfo registers the metadata for a message type.
// This is typically the first step in registration, often called from generated code,
// and it provides information like the message's factory function and type.
// If a registration for the same message ID already exists, this method will preserve
// the previously registered handler and layer type, merging the old and new info.
func RegisterMsgInfo(pi *MsgProtoInfo) {
	if pi == nil || len(pi.MsgID) == 0 || pi.New == nil {
		// Silently ignore invalid registrations. Consider adding logging here.
		return
	}

	// Preserve handler if already registered. This supports a two-phase registration
	// where handlers are attached separately from the core message info.
	if p, ok := propInfoMap[pi.MsgID]; ok {
		pi.MsgHandle = p.MsgHandle
		pi.MsgLayerType = p.MsgLayerType
	}
	propInfoMap[pi.MsgID] = pi
}

// RegisterMsgHandle registers the business logic handler and application layer for a message.
// This is typically the second step in registration, called from application code to
// associate a handler with a message type. If no metadata has been registered for the
// message ID yet, a minimal MsgProtoInfo is created.
func RegisterMsgHandle(msgid string, handle any, msgLayerType MsgLayerType) {
	if len(msgid) == 0 {
		return
	}

	if p, ok := propInfoMap[msgid]; ok {
		// If info already exists, just update the handler and layer.
		p.MsgHandle = handle
		p.MsgLayerType = msgLayerType
		return
	}

	// If no info exists, create a partial entry. This assumes RegisterMsgInfo will be called later.
	propInfoMap[msgid] = &MsgProtoInfo{
		MsgHandle:    handle,
		MsgLayerType: msgLayerType,
	}
}

// GetProtoInfo retrieves the complete protocol metadata for a given message ID.
// It returns the MsgProtoInfo and a boolean indicating if the lookup was successful.
func GetProtoInfo(msgID string) (*MsgProtoInfo, bool) {
	protoInfo, ok := propInfoMap[msgID]
	return protoInfo, ok
}

// CreateMsg creates a new, empty protobuf message instance for the given message ID.
// This method fulfills the MsgCreator interface, allowing the MessageManager to act as a
// message factory for the network layer.
func CreateMsg(msgID string) (proto.Message, error) {
	info, ok := GetProtoInfo(msgID)
	if !ok {
		return nil, errors.New("CreateMsg: message ID not found: " + msgID)
	}
	if info.New == nil {
		return nil, errors.New("CreateMsg: message factory function is not registered for " + msgID)
	}
	return info.New(), nil
}

// ContainsMsg checks if metadata for a given message ID is present in the manager.
// This method also fulfills the MsgCreator interface.
func ContainsMsg(msgID string) bool {
	_, ok := GetProtoInfo(msgID)
	return ok
}

// IsRequestMsg checks if the given message ID corresponds to a request message.
func IsRequestMsg(msgID string) bool {
	if info, ok := GetProtoInfo(msgID); ok {
		return info.MsgReqType == MRTReq
	}
	return false
}

// IsNtfMsg checks if the given message ID corresponds to a notification message.
func IsNtfMsg(msgID string) bool {
	if info, ok := GetProtoInfo(msgID); ok {
		return info.MsgReqType == MRTNtf
	}
	return false
}

// IsSSRequestMsg checks if the given message ID corresponds to a server-to-server request.
func IsSSRequestMsg(msgID string) bool {
	if info, ok := GetProtoInfo(msgID); ok {
		return info.IsSSReq()
	}
	return false
}

// IsSSResMsg checks if the given message ID corresponds to a server-to-server response.
func IsSSResMsg(msgID string) bool {
	if info, ok := GetProtoInfo(msgID); ok {
		return info.IsSSRes()
	}
	return false
}

// IsCSNtfMsg checks if the given message ID corresponds to a client-to-server notification.
func IsCSNtfMsg(msgID string) bool {
	if info, ok := GetProtoInfo(msgID); ok {
		return info.MsgReqType == MRTNtf && info.IsCS
	}
	return false
}

// GetAllMsgList returns a slice of all message IDs that satisfy the given predicate function.
// This provides a flexible way to query the registry for messages with specific properties.
// Example: GetAllMsgList(func(pi *MsgProtoInfo) bool { return pi.IsCS })
func GetAllMsgList(checkFunc func(protoInfo *MsgProtoInfo) bool) []string {
	// Pre-allocating with a reasonable capacity can improve performance.
	msgList := make([]string, 0, len(propInfoMap)/2)
	for msgID, protoInfo := range propInfoMap {
		if checkFunc(protoInfo) {
			msgList = append(msgList, msgID)
		}
	}
	return msgList
}
