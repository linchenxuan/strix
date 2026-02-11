// Package dispatcher provides the central message routing and processing engine for the Strix
// network stack. It acts as a hub that receives messages from various transport layers,
// applies a chain of filters (such as rate limiting and custom logic), and then dispatches
// the messages to the appropriate application-level message handlers.
package dispatcher

import (
	"errors"
	"fmt"
	"sync"

	"github.com/linchenxuan/strix/network/handler"
	"github.com/linchenxuan/strix/network/message"
	"github.com/linchenxuan/strix/network/pb" // Import pb for GetPkgHdr return type
	"github.com/linchenxuan/strix/network/transport"
	//"github.com/linchenxuan/strix/tracing" // Removed unused import
	"google.golang.org/protobuf/proto" // Import proto for DecodeBody return type
)

// DispatcherDelivery is a data transfer object that carries a message through the dispatcher pipeline.
// It embeds the transport-level delivery information and augments it with protocol metadata and
// options for crafting a response. This struct is the primary unit of work within the dispatcher.
type DispatcherDelivery struct {
	*transport.TransportDelivery                            // Embedded transport delivery containing the raw network package.
	ProtoInfo                    *message.MsgProtoInfo      // Protocol metadata for the message, looked up from the package-level message registry.
	ResOpts                      []transport.TransPkgOption // Optional parameters that can be used when constructing a response message.
}

// Ensure DispatcherDelivery implements the handler.Delivery interface.
var _ handler.Delivery = (*DispatcherDelivery)(nil)

// GetPkgHdr returns the package header from the embedded TransportDelivery.
func (dd *DispatcherDelivery) GetPkgHdr() *pb.PackageHead {
	if dd.TransportDelivery != nil && dd.TransportDelivery.Pkg != nil {
		return dd.TransportDelivery.Pkg.GetPkgHdr()
	}
	return nil
}

// GetProtoInfo returns the protocol metadata.
func (dd *DispatcherDelivery) GetProtoInfo() *message.MsgProtoInfo {
	return dd.ProtoInfo
}

// GetSrcEntityID returns the logical entity ID of the sender from the route header.
func (dd *DispatcherDelivery) GetSrcEntityID() uint32 {
	if dd.TransportDelivery != nil && dd.TransportDelivery.Pkg != nil && dd.TransportDelivery.Pkg.RouteHdr != nil {
		return dd.TransportDelivery.Pkg.RouteHdr.GetSrcEntityID()
	}
	return 0
}

// GetReqSrcActorID returns the ID of the actor that originated the request.
func (dd *DispatcherDelivery) GetReqSrcActorID() uint64 {
	if dd.TransportDelivery != nil && dd.TransportDelivery.Pkg != nil && dd.TransportDelivery.Pkg.PkgHdr != nil {
		return dd.TransportDelivery.Pkg.PkgHdr.GetSrcActorID()
	}
	return 0
}

// GetReqDstActorID returns the ID of the actor that is the intended destination of the request.
func (dd *DispatcherDelivery) GetReqDstActorID() uint64 {
	if dd.TransportDelivery != nil && dd.TransportDelivery.Pkg != nil && dd.TransportDelivery.Pkg.PkgHdr != nil {
		return dd.TransportDelivery.Pkg.PkgHdr.GetDstActorID()
	}
	return 0
}

// GetSrcClientVersion returns the version of the client that sent the message.
func (dd *DispatcherDelivery) GetSrcClientVersion() int64 {
	if dd.TransportDelivery != nil && dd.TransportDelivery.Pkg != nil && dd.TransportDelivery.Pkg.PkgHdr != nil {
		return dd.TransportDelivery.Pkg.PkgHdr.GetSrcCliVersion()
	}
	return 0
}

// DecodeBody decodes the message body from the embedded TransportDelivery.
func (dd *DispatcherDelivery) DecodeBody() (proto.Message, error) {
	if dd.TransportDelivery != nil && dd.TransportDelivery.Pkg != nil {
		return dd.TransportDelivery.Pkg.DecodeBody()
	}
	return nil, errors.New("cannot decode body: TransportDelivery or Pkg is nil")
}

// MsgFilterPluginCfg holds the configuration for the message filtering functionality.
// It specifies which message IDs should be explicitly filtered (e.g., blocked or logged).
type MsgFilterPluginCfg struct {
	// MsgFilter is a list of message ID strings that the filter will act upon.
	MsgFilter []string `mapstructure:"msgFilter"`
}

// GetName returns the configuration key for the message filter settings.
func (c *MsgFilterPluginCfg) GetName() string {
	return "msg_filter"
}

// Validate checks if the configuration is valid. For this struct, no validation is currently needed.
func (c *MsgFilterPluginCfg) Validate() error {
	return nil
}

// DispatcherConfig holds all configurable parameters for the Dispatcher.
// It includes settings for rate limiting and message filtering.
type DispatcherConfig struct {
	// RecvRateLimit is the maximum number of messages to process per second. This uses a token bucket algorithm.
	RecvRateLimit int `mapstructure:"recvRateLimit"`
	// TokenBurst defines the burst capacity of the token bucket, allowing for short-term spikes in traffic.
	TokenBurst int `mapstructure:"tokenBurst"`
	// MsgFilter holds the configuration for the message filtering plugin.
	MsgFilter MsgFilterPluginCfg `mapstructure:"msgFilter"`
}

// GetName returns the configuration key for the dispatcher settings.
func (c *DispatcherConfig) GetName() string {
	return "dispatcher"
}

// Validate checks if the dispatcher configuration parameters are within acceptable ranges.
func (c *DispatcherConfig) Validate() error {
	if c.RecvRateLimit <= 0 {
		return fmt.Errorf("RecvRateLimit must be positive")
	}
	if c.TokenBurst <= 0 {
		return fmt.Errorf("TokenBurst must be positive")
	}
	if c.RecvRateLimit > 1000000 {
		return fmt.Errorf("RecvRateLimit cannot exceed 1,000,000 messages per second")
	}
	if c.TokenBurst > c.RecvRateLimit*10 {
		return fmt.Errorf("TokenBurst cannot exceed 10 times RecvRateLimit")
	}
	return nil
}

// Dispatcher is the central message processing hub. It orchestrates the flow of messages
// from the transport layer to the appropriate application message handlers. Its key
// responsibilities include:
// - Applying a chain of filters to incoming messages (e.g., rate limiting).
// - Integrating with metrics and tracing systems for observability.
// - Resolving message protocol information.
// - Routing messages to the correct MsgLayerReceiver based on protocol metadata.
// - Supporting dynamic configuration updates (hot-reloading).
type Dispatcher struct {
	msglayers    map[message.MsgLayerType]handler.MsgLayerReceiver // Maps message layer types to their registered handlers.
	transports   []transport.Transport                             // A list of network transports (e.g., TCP, WebSocket) it serves.
	recvLimiter  *DispatcherRecvLimiter                            // The rate limiter for incoming messages.
	filters      DispatcherFilterChain                             // The chain of filters to be applied to each message.
	msgFilterMap map[string]struct{}                               // A fast-lookup map for message IDs that need to be filtered.

	config *DispatcherConfig // The current configuration for the dispatcher.
	lock   sync.RWMutex      // A mutex to protect the configuration during hot-reloading.
}

// NewDispatcher creates and initializes a new Dispatcher instance.
// It sets up the rate limiter and message filters based on the provided configuration.
func NewDispatcher(cfg *DispatcherConfig, trans []transport.Transport) (*Dispatcher, error) {
	if cfg == nil {
		cfg = &DispatcherConfig{
			RecvRateLimit: 10000, // Default rate limit
			TokenBurst:    1000,  // Default token burst
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid dispatcher configuration: %w", err)
	}

	d := &Dispatcher{
		msglayers:    make(map[message.MsgLayerType]handler.MsgLayerReceiver),
		transports:   trans,
		recvLimiter:  NewTokenRecvLimiter(cfg.RecvRateLimit, cfg.TokenBurst),
		msgFilterMap: make(map[string]struct{}),

		config: cfg,
		lock:   sync.RWMutex{},
	}

	d.reloadMsgFilterCfg(&cfg.MsgFilter)

	// The filter chain is processed in the order filters are added.
	d.filters = append(d.filters, d.msgFilter)
	d.filters = append(d.filters, d.recvLimiter.recvLimiterFilter)

	return d, nil
}

// RegisterMsglayer registers a message handler for a specific message layer type.
// Each layer type (e.g., Stateless, Stateful) can have one handler. This method is not
// safe for concurrent calls and should be called during initialization.
func (d *Dispatcher) RegisterMsglayer(t message.MsgLayerType, m handler.MsgLayerReceiver) error {
	if m == nil {
		return errors.New("RegisterMsglayer: receiver is nil")
	}
	if t <= message.MsgLayerType_None || t >= message.MsgLayerType_Max {
		return errors.New("RegisterMsglayer: invalid message layer type")
	}

	if _, ok := d.msglayers[t]; ok {
		return errors.New("RegisterMsglayer: a receiver for this layer type is already registered")
	}
	d.msglayers[t] = m
	return nil
}

// handleTransportMsgImpl is the final step in the dispatcher's filter chain.
// It selects the appropriate message layer handler and invokes it.
func (d *Dispatcher) handleTransportMsgImpl(dd *DispatcherDelivery) error {
	receiver := d.chooseMsgLayerReceiver(dd)
	if receiver == nil {
		// This can happen if a message is received for a layer that has no registered handler.
		return fmt.Errorf("no message layer receiver found for message ID '%s' and layer type '%v'",
			dd.GetProtoInfo().MsgID, dd.GetProtoInfo().MsgLayerType)
	}

	return receiver.OnRecvDispatcherPkg(dd)
}

// chooseMsgLayerReceiver selects the registered MsgLayerReceiver based on the protocol
// information of the message.
func (d *Dispatcher) chooseMsgLayerReceiver(dd *DispatcherDelivery) handler.MsgLayerReceiver {
	if dd.GetProtoInfo() == nil {
		return nil
	}
	// The zero value for MsgLayerType is MsgLayerType_None, so this check is safe.
	return d.msglayers[message.MsgLayerType(dd.GetProtoInfo().MsgLayerType)]
}
