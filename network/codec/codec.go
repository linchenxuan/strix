// Package codec provides a flexible mechanism for serializing and deserializing network messages.
// It defines a core Codec interface and provides a default implementation that uses
// protobuf for encoding and JSON for decoding, allowing for custom codecs to be plugged in.
package codec

import (
	"encoding/json"
	"errors"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var (
	// errCodecNotInit is returned when an operation is attempted before a Codec
	// has been initialized or set.
	errCodecNotInit = errors.New("codec not init")

	// _codec holds the singleton instance of the Codec used by the package-level
	// Encode and Decode functions. It defaults to DefaultCodec.
	_codec Codec = &DefaultCodec{}
)

// Codec defines the contract for message serialization and deserialization.
// This interface allows different encoding strategies to be used interchangeably
// throughout the network layer.
type Codec interface {
	// Encode takes a protobuf message and marshals it into a byte slice.
	// It is designed to be efficient by appending the encoded data to an existing
	// byte slice `b`, reducing memory allocations.
	Encode(m protoreflect.ProtoMessage, b []byte) ([]byte, error)

	// Decode takes a byte slice and unmarshals it into a target data structure `a`.
	// The target `a` is passed as an `any` type, allowing for flexibility in the
	// type of data being decoded (e.g., structs, maps).
	Decode(a any, b []byte) error
}

// Encode uses the globally configured codec to marshal a protobuf message.
// This function acts as a convenient wrapper around the active codec's Encode method.
// It returns an error if no codec has been set.
func Encode(m protoreflect.ProtoMessage, b []byte) ([]byte, error) {
	if _codec == nil {
		return nil, errCodecNotInit
	}
	return _codec.Encode(m, b)
}

// Decode uses the globally configured codec to unmarshal a byte slice into a
// target data structure.
// This function acts as a convenient wrapper around the active codec's Decode method.
// It returns an error if no codec has been set.
func Decode(a any, b []byte) error {
	if _codec == nil {
		return errCodecNotInit
	}
	return _codec.Decode(a, b)
}

// SetCodec replaces the default global codec with a custom implementation.
// This should be called during application initialization to configure the desired
// serialization strategy. It is not safe for concurrent use.
func SetCodec(c Codec) {
	_codec = c
}

// DefaultCodec provides a basic implementation of the Codec interface.
// It uses the standard protobuf marshaler for encoding and the standard JSON unmarshaler
// for decoding. Note the asymmetry: this is suitable for scenarios where outgoing messages
// are strictly protobuf objects, while incoming data might be in a more generic JSON format.
type DefaultCodec struct{}

// Encode marshals a protobuf message using the official proto.MarshalOptions.
// It appends the result to the provided byte slice `b` to optimize memory usage.
func (c *DefaultCodec) Encode(m protoreflect.ProtoMessage, b []byte) ([]byte, error) {
	return proto.MarshalOptions{}.MarshalAppend(b, m)
}

// Decode unmarshals a JSON-encoded byte slice into the provided interface `a`.
// It uses the standard `json.Unmarshal` for this purpose.
func (c *DefaultCodec) Decode(a any, b []byte) error {
	return json.Unmarshal(b, a)
}
