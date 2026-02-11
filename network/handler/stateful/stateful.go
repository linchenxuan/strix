// Package stateful provides the stateful message processing layer implementation for the Strix framework.
// It manages actors with persistent state and handles message routing, actor lifecycle,
// and state persistence in a distributed game server environment.
package stateful

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/metrics"
	"github.com/linchenxuan/strix/network/dispatcher"
	"github.com/linchenxuan/strix/network/handler"
	"github.com/linchenxuan/strix/network/message"
	"github.com/linchenxuan/strix/network/transport"
	"github.com/linchenxuan/strix/tracing"
	"google.golang.org/protobuf/proto"
)

// MsgHandle defines the function signature for a stateful message handler.
// It takes the message context, the target actor, and the message body as input,
// and returns a response message and a status code.
type MsgHandle func(ctx *HandleContext, actor Actor, body proto.Message) (res proto.Message, code int32)

// MigrateMsgHandle defines the function signature for handling messages related to actor state migration.
// It processes messages during actor state migration between servers.
type MigrateMsgHandle func(ctx *HandleContext, body proto.Message) (res proto.Message, code int32)

// StatefulMsgLayer is the top-level implementation of the stateful message processing layer.
// It is responsible for receiving messages from the dispatcher, routing them to the correct
// stateful actor, and managing the lifecycle of those actors via the actorMgr.
type StatefulMsgLayer struct {
	*StatefulConfig                       // Embedded configuration for the stateful layer.
	csTransport     transport.CSTransport // The transport for client-to-server communication.
	ssTransport     transport.SSTransport // The transport for server-to-server communication.
	actorMgr        *actorMgr             // The manager responsible for all actor lifecycles.

	lock sync.RWMutex // A lock to protect the configuration during hot-reloads.

	_ handler.MsgLayer `json:"-"` // Assert that StatefulMsgLayer implements handler.MsgLayer
}

// GetConfigName returns the name of the configuration section this listener is interested in.
func (layer *StatefulMsgLayer) GetConfigName() string {
	return "stateful"
}

// NewMsgLayer creates a new stateful message layer instance.
func NewMsgLayer(cfg *StatefulConfig, creator ActorCreator, cs transport.CSTransport, ss transport.SSTransport) (*StatefulMsgLayer, error) {
	if cfg == nil {
		return nil, errors.New("StatefulConfig cannot be nil; use NewMsgLayerWithConfigManager for dynamic configuration")
	}

	layer := &StatefulMsgLayer{
		StatefulConfig: cfg,
		csTransport:    cs,
		ssTransport:    ss,
		actorMgr:       newActorMgr(creator),

		lock: sync.RWMutex{},
	}
	layer.actorMgr.msgLayer = layer // Provide back-reference for config access.
	return layer, nil
}

// Init initializes the stateful message layer. This method is part of the handler.MsgLayer interface.
func (layer *StatefulMsgLayer) Init() error {
	// Currently no specific initialization logic is needed here.
	return nil
}

// Shutdown gracefully shuts down the stateful message layer by telling the actor manager
// to terminate all active actors. This method is part of the handler.MsgLayer interface.
func (layer *StatefulMsgLayer) Shutdown() {
	layer.actorMgr.shutdown()
}

// IsActorExist checks if an actor with the given ID is currently active in the manager.
func (layer *StatefulMsgLayer) IsActorExist(aid uint64) bool {
	_, ok := layer.actorMgr.getActorRuntime(aid)
	return ok
}

// GetActorCount returns the total number of currently active actors.
func (layer *StatefulMsgLayer) GetActorCount() int {
	return layer.actorMgr.actorCount()
}

// GetMaxActorCount returns the configured maximum number of actors for this node.
func (layer *StatefulMsgLayer) GetMaxActorCount() int {
	return layer.MaxActorCount
}

// GetMigrateFeatSwitch returns true if the actor state migration feature is enabled.
func (layer *StatefulMsgLayer) GetMigrateFeatSwitch() bool {
	return layer.MigrateFeatSwitch
}

// NtfActorExit signals all active actors to begin their graceful shutdown process.
func (layer *StatefulMsgLayer) NtfActorExit() {
	layer.actorMgr.ntfActorExit()
}

// DispatchCachedPkg dispatches a message to an already active actor.
// This is an optimization to avoid the actor creation check if the caller knows the actor exists.
func (layer *StatefulMsgLayer) DispatchCachedPkg(aid uint64, delivery handler.Delivery) error {
	ar, ok := layer.actorMgr.getActorRuntime(aid)
	if !ok {
		return fmt.Errorf("actor %d not found for cached dispatch", aid)
	}
	// Type assert delivery to concrete dispatcher.DispatcherDelivery for newHandleContext
	concreteDelivery, ok := delivery.(*dispatcher.DispatcherDelivery)
	if !ok {
		return fmt.Errorf("invalid delivery type for DispatchCachedPkg")
	}
	hCtx := newHandleContext(aid, concreteDelivery, ar.actor)
	hCtx.msgLayer = layer
	hCtx.ntfSender = message.NewNtfSender()
	return ar.postPkg(hCtx)
}

// OnRecvDispatcherPkg is the main entry point for messages routed to the stateful layer.
// It implements the handler.MsgLayerReceiver interface.
func (layer *StatefulMsgLayer) OnRecvDispatcherPkg(delivery handler.Delivery) error {
	if err := layer.handleDispacherPkg(delivery); err != nil {
		// Log the error but do not return it, as the error has been handled
		// (e.g., by sending an error response or dropping the message).
		log.Error().Err(err).Msg("failed to handle dispatcher package")
	}
	return nil
}

// handleDispacherPkg is the internal handler that routes a message to the correct actor.
func (layer *StatefulMsgLayer) handleDispacherPkg(delivery handler.Delivery) (err error) {
	// Type assert delivery to concrete dispatcher.DispatcherDelivery to access Pkg field
	concreteDelivery, ok := delivery.(*dispatcher.DispatcherDelivery)
	if !ok {
		return fmt.Errorf("invalid delivery type for handleDispacherPkg")
	}
	if concreteDelivery.Pkg.PkgHdr.DstActorID == 0 {
		return errors.New("message for stateful layer has no destination actor ID")
	}
	return layer.handleActorPkg(concreteDelivery.Pkg.PkgHdr.DstActorID, delivery)
}

// handleActorPkg finds or creates the target actor and posts the message to its mailbox.
// This is the hand-off point where the message moves from the dispatcher's thread to the
// actor's dedicated goroutine for serialized processing.
func (layer *StatefulMsgLayer) handleActorPkg(aid uint64, delivery handler.Delivery) (err error) {
	ctx := context.Background()
	spanName := fmt.Sprintf("stateful.actor.dispatch.%d", aid)
	ctx, span := tracing.StartSpanFromContext(ctx, spanName)
	defer span.End()

	span.SetTag("component", "stateful_layer")
	span.SetTag("actor.id", aid)
	span.SetTag("message.id", delivery.GetPkgHdr().GetMsgID())

	startTime := time.Now()
	metrics.IncrCounterWithGroup("net.stateful", "actor_message_received_total", 1)
	defer metrics.RecordStopwatchWithGroup("net.stateful", "actor_dispatch_time", startTime)

	// Get or create the actor's runtime.
	ar, err := layer.tryGetActorRuntime(aid, delivery)
	if err != nil {
		metrics.IncrCounterWithDimGroup("net.stateful", "actor_dispatch_error_total", 1, map[string]string{"reason": "get_or_create_failed"})
		span.SetTag("error", true)
		span.LogKV("event", "actor_get_or_create_failed", "error.message", err.Error())
		return err
	}

	// Type assert delivery to concrete dispatcher.DispatcherDelivery for newHandleContext
	concreteDelivery, ok := delivery.(*dispatcher.DispatcherDelivery)
	if !ok {
		return fmt.Errorf("invalid delivery type for handleActorPkg")
	}

	// Create the context for this specific message handling.
	hCtx := newHandleContext(aid, concreteDelivery, ar.actor)
	hCtx.msgLayer = layer
	hCtx.ntfSender = message.NewNtfSender()

	// Post the message to the actor's mailbox. This is a non-blocking hand-off.
	if err = ar.postPkg(hCtx); err != nil {
		metrics.IncrCounterWithDimGroup("net.stateful", "actor_dispatch_error_total", 1, map[string]string{"reason": "mailbox_full"})
		span.SetTag("error", true)
		span.LogKV("event", "actor_mailbox_full", "error.message", err.Error())
	} else {
		span.LogKV("event", "message_posted_to_mailbox")
	}

	return err
}

// tryGetActorRuntime attempts to get an existing actor runtime. If the actor does not
// exist, it proceeds to create it.
func (layer *StatefulMsgLayer) tryGetActorRuntime(aid uint64, delivery handler.Delivery) (*actorRuntime, error) {
	// First, try a read-locked check for performance.
	if a, ok := layer.actorMgr.getActorRuntime(aid); ok {
		return a, nil
	}

	// If not found, decode the body before attempting creation. This is a business logic
	// choice to ensure we don't create actors for malformed requests.
	if _, err := delivery.DecodeBody(); err != nil {
		return nil, fmt.Errorf("failed to decode message body for actor %d: %w", aid, err)
	}

	// Now, attempt to create the actor. This involves a write lock.
	return layer.actorMgr.createActor(aid)
}
