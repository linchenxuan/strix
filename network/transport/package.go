// Package transport defines the interfaces and foundational data structures for network
// communication. This file specifies the transport-level package formats for sending
// and receiving data, and includes the logic for their serialization and deserialization.
package transport

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/network/codec"
	"github.com/linchenxuan/strix/network/pb"
	"google.golang.org/protobuf/proto"
)

// MsgCreator defines an interface for a factory that can create message instances from an ID.
// Note: This is a duplicate of the interface in the `message` package and should be consolidated.
type MsgCreator interface {
	CreateMsg(msgID string) (proto.Message, error)
	ContainsMsg(msgID string) bool
}

// TransRecvPkg represents a package as it is received by the transport layer.
// It encapsulates the raw message data and implements a lazy decoding mechanism to optimize
// performance by only deserializing the message body when it is explicitly accessed.
type TransRecvPkg struct {
	RouteHdr *pb.RouteHead   // Routing information for distributed environments.
	PkgHdr   *pb.PackageHead // Metadata for the package, like message ID and actor IDs.
	decoded  bool            // A flag to indicate if the body has been decoded.
	bodyData []byte          // The raw, serialized bytes of the message body.
	creator  MsgCreator      // The factory used to instantiate the message body object.
	body     proto.Message   // The decoded message body, cached after the first access.
}

// SetRouteHdr sets the route header for the package.
func (t *TransRecvPkg) SetRouteHdr(rh *pb.RouteHead) {
	t.RouteHdr = rh
}

// GetRouteHdr returns the route header of the package.
func (t *TransRecvPkg) GetRouteHdr() *pb.RouteHead {
	return t.RouteHdr
}

// GetPkgHdr returns the package metadata header.
func (t *TransRecvPkg) GetPkgHdr() *pb.PackageHead {
	return t.PkgHdr
}

// DecodeBody lazily decodes the raw message body into a protobuf message object.
// On the first call, it uses the MsgCreator to instantiate the correct message type,
// decodes the raw `bodyData` into it, caches the result, and clears the raw data to save memory.
// Subsequent calls return the cached object directly.
func (t *TransRecvPkg) DecodeBody() (proto.Message, error) {
	if t.decoded {
		return t.body, nil
	}

	if t.creator == nil {
		return nil, errors.New("DecodeBody: MsgCreator is not set")
	}

	// Create an empty message instance of the correct type.
	body, err := t.creator.CreateMsg(t.PkgHdr.MsgID)
	if err != nil {
		return nil, err
	}

	// Decode the raw bytes into the message object.
	if err := codec.Decode(body, t.bodyData); err != nil {
		return nil, err
	}

	// Cache the decoded body and mark as decoded.
	t.body = body
	t.decoded = true
	t.bodyData = nil // Release the raw byte slice to free memory.

	return t.body, nil
}

// ConvertToTransSendPkg transforms a received package into a package ready for sending.
// This is useful for forwarding messages. It ensures the body is decoded before creating the new package.
func (t *TransRecvPkg) ConvertToTransSendPkg() (*TransSendPkg, error) {
	body, err := t.DecodeBody()
	if err != nil {
		return nil, err
	}

	return &TransSendPkg{
		RouteHdr: t.RouteHdr,
		PkgHdr:   t.PkgHdr,
		Body:     body,
	}, nil
}

// MarshalLogObj implements the log.ObjectMarshaller interface for structured logging.
// It provides a safe and readable representation of the package for logs.
func (t *TransRecvPkg) MarshalLogObj(e *log.LogEvent) {
	if t.RouteHdr != nil {
		e.Str("route", t.RouteHdr.String())
	}
	if t.PkgHdr != nil {
		e.Str("pkgHdr", t.PkgHdr.String())
	}

	// Attempt to log the body content safely.
	if t.decoded && t.body != nil {
		if msg, err := proto.Marshal(t.body); err == nil {
			e.Str("msg", string(msg))
		}
	}
}

// TransSendPkg represents a package that is ready to be sent over the network.
// It contains a fully constructed message object and its associated headers.
type TransSendPkg struct {
	RouteHdr *pb.RouteHead   // Routing information for directing the package.
	PkgHdr   *pb.PackageHead // Metadata for the package.
	Body     proto.Message   // The protobuf message payload.
}

// SetSrcActorID sets the source actor ID in the package header.
func (t *TransSendPkg) SetSrcActorID(id uint64) {
	if t.PkgHdr != nil {
		t.PkgHdr.SrcActorID = id
	}
}

// GetRouteHdr returns the route header of the package.
func (t *TransSendPkg) GetRouteHdr() *pb.RouteHead {
	return t.RouteHdr
}

// SetRouteHdr sets the route header for the package.
func (t *TransSendPkg) SetRouteHdr(rh *pb.RouteHead) {
	t.RouteHdr = rh
}

// GetPkgHdr returns the package metadata header.
func (t *TransSendPkg) GetPkgHdr() *pb.PackageHead {
	return t.PkgHdr
}

// SetDstActorID sets the destination actor ID in the package header.
func (t *TransSendPkg) SetDstActorID(id uint64) {
	if t.PkgHdr != nil {
		t.PkgHdr.DstActorID = id
	}
}

// MarshalLogObj implements the log.ObjectMarshaller interface for structured logging.
func (t *TransSendPkg) MarshalLogObj(e *log.LogEvent) {
	if t.RouteHdr != nil {
		e.Str("route", t.RouteHdr.String())
	}
	if t.PkgHdr != nil {
		e.Str("pkgHdr", t.PkgHdr.String())
	}
	if t.Body != nil {
		if msg, err := proto.Marshal(t.Body); err == nil {
			e.Str("msg", string(msg))
		}
	}
}

// NewTransRecvPkgWithBody creates a new received package from a pre-decoded message body.
// This is useful for internal message passing or testing where deserialization is not needed.
func NewTransRecvPkgWithBody(routeHdr *pb.RouteHead, hdr *pb.PackageHead, body proto.Message) *TransRecvPkg {
	return &TransRecvPkg{
		RouteHdr: routeHdr,
		PkgHdr:   hdr,
		decoded:  true,
		body:     body,
	}
}

// NewTransRecvPkgWithBodyData creates a new received package from raw byte data.
// This is the standard constructor used by the transport layer when reading from the network.
// The body remains encoded until `DecodeBody` is called.
func NewTransRecvPkgWithBodyData(routeHdr *pb.RouteHead, hdr *pb.PackageHead,
	bodyData []byte, creator MsgCreator) *TransRecvPkg {
	return &TransRecvPkg{
		RouteHdr: routeHdr,
		PkgHdr:   hdr,
		decoded:  false,
		creator:  creator,
		bodyData: bodyData,
	}
}

// EncodeCSMsg serializes a TransSendPkg into its constituent parts according to the wire protocol.
// The protocol format is: [PreHead, PkgHead, PkgBody].
// It returns a slice of byte slices, where each inner slice holds one part of the message.
func EncodeCSMsg(pkg *TransSendPkg) (encodedBuf [][]byte, err error) {
	// Allocate slices for [PreHead, PkgHead, PkgBody, (reserved for tail)]
	datas := make([][]byte, 4)

	datas[1], err = codec.Encode(pkg.PkgHdr, nil)
	if err != nil {
		return nil, err
	}

	datas[2], err = codec.Encode(pkg.Body, nil)
	if err != nil {
		return nil, err
	}

	// Create the PreHead which contains the lengths of the other parts.
	pHdr := &PreHead{
		HdrSize:  uint32(len(datas[1])),
		BodySize: uint32(len(datas[2])),
	}

	datas[0] = EncodePreHead(pHdr)
	return datas, nil
}

// PackCSMsg serializes a package and writes all its parts into a single buffer.
// This is a convenience function for transports that send the message in one write operation.
func PackCSMsg(pkg *TransSendPkg, buf *bytes.Buffer) error {
	datas, err := EncodeCSMsg(pkg)
	if err != nil {
		return err
	}

	for _, data := range datas {
		if _, err := buf.Write(data); err != nil {
			return err
		}
	}

	return nil
}

// PackCSPkg serializes a package, returning the PreHead separately and writing the
// PkgHead and PkgBody to the provided buffer. This is for transports that need to
// handle the length prefix separately from the payload.
func PackCSPkg(pkg *TransSendPkg, buf *bytes.Buffer) (preHeadBuf []byte, err error) {
	datas, err := EncodeCSMsg(pkg)
	if err != nil {
		return nil, err
	}

	// Write PkgHead and PkgBody to the buffer.
	if _, err := buf.Write(datas[1]); err != nil {
		return nil, err
	}
	if _, err := buf.Write(datas[2]); err != nil {
		return nil, err
	}

	return datas[0], nil
}

// decodeCSMsgHead parses the PreHead and PkgHead from a raw byte slice of a full message.
// It returns the parsed headers and a slice pointing to the raw body data.
func decodeCSMsgHead(data []byte) (HeadDecodeResult, error) {
	var result HeadDecodeResult
	if len(data) < PRE_HEAD_SIZE {
		return result, errors.New("data too short for PreHead")
	}

	h, err := DecodePreHead(data)
	if err != nil {
		return result, fmt.Errorf("failed to decode PreHead: %w", err)
	}

	expectedLen := PRE_HEAD_SIZE + h.HdrSize + h.BodySize
	if uint32(len(data)) < expectedLen {
		return result, fmt.Errorf("data length mismatch: expected %d, got %d", expectedLen, len(data))
	}

	result.PreHead = h
	result.PkgHead = &pb.PackageHead{}
	pkgHeadEnd := PRE_HEAD_SIZE + h.HdrSize
	if err = codec.Decode(result.PkgHead, data[PRE_HEAD_SIZE:pkgHeadEnd]); err != nil {
		return result, fmt.Errorf("failed to decode PkgHead: %w", err)
	}
	result.BodyData = data[pkgHeadEnd : pkgHeadEnd+h.BodySize]
	return result, nil
}

// DecodeCSMsg decodes a full client-server message from a byte slice.
// It parses the headers and decodes the body into a message object.
func DecodeCSMsg(data []byte, creator MsgCreator) (*TransRecvPkg, error) {
	if creator == nil {
		return nil, errors.New("MsgCreator is nil")
	}
	result, err := decodeCSMsgHead(data)
	if err != nil {
		return nil, err
	}
	body, err := creator.CreateMsg(result.PkgHead.GetMsgID())
	if err != nil {
		return nil, err
	}

	if err := codec.Decode(body, result.BodyData); err != nil {
		return nil, err
	}
	return NewTransRecvPkgWithBody(nil, result.PkgHead, body), nil
}

// DecodeCSPkg decodes the PkgHead and PkgBody from a byte slice, given a pre-decoded PreHead.
// This is used by transports that read the message from the network in two stages
// (first the length prefix, then the payload).
func DecodeCSPkg(h *PreHead, data []byte, creator MsgCreator) (*TransRecvPkg, error) {
	if creator == nil {
		return nil, errors.New("MsgCreator is nil")
	}
	expectedLen := h.HdrSize + h.BodySize
	if uint32(len(data)) < expectedLen {
		return nil, fmt.Errorf("data length mismatch: expected %d, got %d", expectedLen, len(data))
	}

	var hdr pb.PackageHead
	if err := codec.Decode(&hdr, data[:h.HdrSize]); err != nil {
		return nil, fmt.Errorf("failed to decode PkgHead in DecodeCSPkg: %w", err)
	}

	body, err := creator.CreateMsg(hdr.GetMsgID())
	if err != nil {
		return nil, err
	}

	if err := codec.Decode(body, data[h.HdrSize:h.HdrSize+h.BodySize]); err != nil {
		return nil, err
	}

	return NewTransRecvPkgWithBody(nil, &hdr, body), nil
}

// EncodeSSMsgWithBuffer SSMsgFormat MsgPreHead + RouteHdr + MsgPreHead + PkgHead + PkgBody.
func EncodeSSMsgWithBuffer(pkg *TransSendPkg, encodeBuf *EncoderBuffer) error {
	datas := encodeBuf.Datas
	meshHeadIdx := 1

	var err error
	datas[meshHeadIdx], err = codec.Encode(pkg.RouteHdr, datas[meshHeadIdx])
	if err != nil {
		return err
	}

	datas[meshHeadIdx+2], err = codec.Encode(pkg.PkgHdr, datas[meshHeadIdx+2])
	if err != nil {
		return err
	}

	datas[meshHeadIdx+3], err = codec.Encode(pkg.Body, datas[meshHeadIdx+3])
	if err != nil {
		return err
	}

	h := PreHead{
		HdrSize:  uint32(len(datas[meshHeadIdx+2])),
		BodySize: uint32(len(datas[meshHeadIdx+3])),
	}
	encodeBuf.EncodePreHead(meshHeadIdx+1, h)

	var bodyLen int
	for i := meshHeadIdx + 1; i < _EncodeBufferSize; i++ {
		bodyLen += len(datas[i])
	}
	h.HdrSize = uint32(len(datas[meshHeadIdx]))
	h.BodySize = uint32(bodyLen)
	encodeBuf.EncodePreHead(0, h)

	return err
}

// EncodeSSMsg SSMsgFormat MsgPreHead + RouteHdr + MsgPreHead + PkgHead + PkgBody.
func EncodeSSMsg(pkg TransSendPkg) ([][]byte, error) {
	if pkg.PkgHdr == nil || pkg.RouteHdr == nil {
		return nil, errors.New("encode invalid pkg")
	}

	dataSize := 5
	meshHeadIdx := 1

	datas := make([][]byte, dataSize)
	var err error
	datas[meshHeadIdx], err = codec.Encode(pkg.RouteHdr, datas[meshHeadIdx])
	if err != nil {
		return datas, err
	}

	datas[meshHeadIdx+2], err = codec.Encode(pkg.PkgHdr, datas[meshHeadIdx+2])
	if err != nil {
		return datas, err
	}

	datas[meshHeadIdx+3], err = codec.Encode(pkg.Body, datas[meshHeadIdx+3])
	if err != nil {
		return datas, err
	}

	h := &PreHead{
		HdrSize:  uint32(len(datas[meshHeadIdx+2])),
		BodySize: uint32(len(datas[meshHeadIdx+3])),
	}
	datas[meshHeadIdx+1] = EncodePreHead(h)

	var bodyLen int
	for i := meshHeadIdx + 1; i < dataSize; i++ {
		bodyLen += len(datas[i])
	}
	h.HdrSize = uint32(len(datas[meshHeadIdx]))
	h.BodySize = uint32(bodyLen)
	datas[0] = EncodePreHead(h)
	return datas, err
}
