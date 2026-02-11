// Package transport defines the interfaces and foundational data structures for network
// communication. This file specifies the structure and handling of `PreHead`, a fixed-size
// length prefix used in the wire protocol for framing network messages.
package transport

import (
	"encoding/binary"
	"errors"

	"github.com/linchenxuan/strix/network/pb"
)

// PRE_HEAD_SIZE is the fixed size in bytes of the PreHead structure.
// It is typically placed at the very beginning of a network message to indicate
// the lengths of the subsequent message header and body.
const PRE_HEAD_SIZE = 12 // HdrSize (4 bytes) + BodySize (4 bytes) + (reserved/unused 4 bytes)

// PreHead defines a fixed-size header used for framing network messages.
// It contains the size information for the actual message header and message body,
// enabling the receiver to efficiently read the complete message from the stream.
type PreHead struct {
	HdrSize  uint32 // The size of the message's protobuf header (PkgHead) in bytes.
	BodySize uint32 // The size of the message's protobuf body in bytes.
	// Note: 4 bytes might be reserved or unused in the current 12-byte structure
	//       (8 bytes for HdrSize+BodySize, 4 bytes padding to reach 12).
	TailSize uint32
}

// EncodePreHead serializes a PreHead struct into a byte slice.
// It uses little-endian encoding to store the HdrSize and BodySize into the buffer.
func EncodePreHead(hdr *PreHead) []byte {
	buf := make([]byte, PRE_HEAD_SIZE)
	binary.LittleEndian.PutUint32(buf[0:4], hdr.HdrSize)
	binary.LittleEndian.PutUint32(buf[4:8], hdr.BodySize)
	// The remaining 4 bytes (buf[8:12]) are currently unused but reserved.
	return buf
}

// DecodePreHead deserializes a byte slice back into a PreHead struct.
// It expects a byte slice of at least PRE_HEAD_SIZE and uses little-endian encoding.
// It also performs a basic validation to ensure HdrSize is not zero, as an empty header is typically invalid.
func DecodePreHead(buf []byte) (*PreHead, error) {
	if len(buf) < PRE_HEAD_SIZE {
		return nil, errors.New("buffer too small to decode PreHead")
	}
	hdr := &PreHead{
		HdrSize:  binary.LittleEndian.Uint32(buf[0:4]),
		BodySize: binary.LittleEndian.Uint32(buf[4:8]),
	}
	if hdr.HdrSize == 0 {
		return nil, errors.New("decoded PreHead indicates zero header size, which is invalid")
	}
	return hdr, nil
}

// HeadDecodeResult is a container struct to hold the various parts of a message
// after its initial header decoding phase. It's used internally during message parsing.
type HeadDecodeResult struct {
	PreHead  *PreHead        // The parsed fixed-size length prefix.
	RouteHdr *pb.RouteHead   // The parsed routing header (optional, if included in PkgHead or separately).
	PkgHead  *pb.PackageHead // The parsed package metadata header.
	BodyData []byte          // A byte slice pointing to the raw, unparsed message body data.
}
