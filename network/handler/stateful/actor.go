// Package stateful provides the implementation for the stateful message processing layer.
// This file defines the core Actor model, which represents a single-threaded, stateful
// entity (e.g., a player, a room), and the `actorRuntime` that manages its lifecycle.
package stateful

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/metrics"
	"github.com/linchenxuan/strix/network/message"
	"github.com/linchenxuan/strix/network/pb"
	"github.com/linchenxuan/strix/network/transport"
	"google.golang.org/protobuf/proto"
)

// ActorExitFunc is a callback function type that is executed when an actor exits.
// It's used for performing custom cleanup tasks.
type ActorExitFunc func()

// Actor is the interface that all stateful entities must implement. An Actor is a fundamental
// unit of concurrency that encapsulates state and behavior. It processes messages serially,
// eliminating the need for locks within the actor's implementation.
type Actor interface {
	log.Logger // Embeds a logger for convenient, context-aware logging.

	// Init is called once when the actor is created and before it starts processing messages.
	// Use this for any necessary setup.
	Init() error
	// OnTick is called periodically by the actor's runtime. It is used for time-based logic,
	// such as updating game state or checking conditions.
	OnTick() error
	// OnGracefulExit is called when the actor is about to be shut down. This is the place
	// to perform cleanup, such as saving final state or releasing resources.
	OnGracefulExit()
	// OnMigrateFailRecover is called if a state migration for this actor fails. The actor
	// should implement recovery logic to restore a consistent state.
	OnMigrateFailRecover() error
	// Save persists the actor's current state to a backing store.
	Save()
	// ShowChange is used for debugging and diagnostics, returning a representation of the actor's state changes.
	ShowChange() (proto.Message, string)
	// SetActorExitFunc registers a callback function to be invoked when the actor exits.
	SetActorExitFunc(ActorExitFunc)
	// EncodeMigrateData serializes the actor's state for migration to another server.
	EncodeMigrateData() ([]byte, error)
	// GetUpdateInternMS returns the actor-specific update interval in milliseconds.
	GetUpdateInternMS() int
}

// ActorCreator defines a factory function signature for creating new Actor instances.
type ActorCreator func(uid uint64) (Actor, error)

// ActorBase is a convenience base struct that provides a default implementation for some
// methods of the Actor interface, reducing boilerplate for new actor types.
type ActorBase struct {
	exitFunc ActorExitFunc
}

// SetActorExitFunc registers the callback function to be invoked when ExitActor is called.
func (a *ActorBase) SetActorExitFunc(f ActorExitFunc) { a.exitFunc = f }

// ExitActor triggers the registered exit function. This is typically called by the
// application logic to programmatically shut down the actor.
func (a *ActorBase) ExitActor() {
	if a.exitFunc != nil {
		a.exitFunc()
	}
}

// actorRuntime is the internal supervisor and execution environment for an Actor instance.
// It is responsible for the actor's entire lifecycle, including running its event loop,
// processing messages serially from a channel, handling ticks, and managing graceful shutdown.
// This structure ensures that all interactions with the actor's state are single-threaded.
type actorRuntime struct {
	actor           Actor               // The application-defined actor instance.
	actorID         uint64              // The unique ID of this actor.
	lastActive      int64               // UNIX timestamp of the last activity, used for idle timeout.
	lastFreqSaveSec int64               // UNIX timestamp of the last periodic save.
	closed          atomic.Bool         // An atomic flag indicating if the actor's loop is shutting down.
	pkgCtxChan      chan *HandleContext // The actor's "mailbox": a channel for incoming messages.
	migrateChan     chan bool           // A channel for signaling state migration.
	cancel          context.CancelFunc  // A function to cancel the actor's context and trigger shutdown.
	ctx             context.Context     // The context governing the actor's lifecycle.

	msgLayer *StatefulMsgLayer // A reference back to the owning message layer.
}

// GetActor returns the underlying application-defined Actor instance.
func (a *actorRuntime) GetActor() Actor {
	return a.actor
}

// newActorRuntime creates a new runtime environment for a given actor.
func newActorRuntime(msgLayer *StatefulMsgLayer, actorID uint64, actor Actor) *actorRuntime {
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now().Unix()
	ar := &actorRuntime{
		actorID:         actorID,
		actor:           actor,
		pkgCtxChan:      make(chan *HandleContext, msgLayer.TaskChanSize),
		migrateChan:     make(chan bool, 1),
		lastActive:      now,
		lastFreqSaveSec: now,
		ctx:             ctx,
		cancel:          cancel,
		msgLayer:        msgLayer,
	}
	return ar
}

// Cancel triggers the shutdown of the actor's event loop by canceling its context.
func (a *actorRuntime) Cancel() {
	if a != nil && a.cancel != nil {
		a.actor.Debug().Msg("actor context canceled")
		a.cancel()
	}
}

// initActor calls the user-defined Init method on the actor.
func (a *actorRuntime) initActor() error {
	return a.actor.Init()
}

// exitActor initiates a graceful shutdown of the actor by canceling its context.
func (a *actorRuntime) exitActor() {
	a.actor.Info().Msg("actor exit requested")
	a.cancel()
}

// postPkg delivers a message to the actor by placing it in its mailbox channel.
// It is the only way to send messages to an actor and ensures serialized processing.
// If the actor is closed, it drops notifications and returns an error for requests.
// If the channel is full, it returns an error, providing back-pressure.
func (a *actorRuntime) postPkg(hCtx *HandleContext) error {
	if a.closed.Load() {
		msgType := "notification"
		if hCtx.ProtoInfo.IsReq() {
			msgType = "request"
		}
		metrics.IncrCounterWithDimGroup("net.stateful", "actor_message_dropped_total", 1, map[string]string{"reason": "actor_closed", "message_type": msgType})

		if hCtx.ProtoInfo.IsReq() {
			hCtx.Error().Msg("actor is closed, cannot process request")
			// In a real implementation, a proper error response should be sent back.
			return errors.New("actor is closed")
		}
		// For notifications, we can just drop them.
		hCtx.Info().Msg("actor is closed, dropping notification")
		return nil
	}

	select {
	case a.pkgCtxChan <- hCtx:
		metrics.UpdateGaugeWithGroup("net.stateful", "actor_queue_length", metrics.Value(len(a.pkgCtxChan)))
		return nil
	default:
		// Mailbox is full.
		msgType := "notification"
		if hCtx.ProtoInfo.IsReq() {
			msgType = "request"
		}
		metrics.IncrCounterWithDimGroup("net.stateful", "actor_message_dropped_total", 1, map[string]string{"reason": "queue_full", "message_type": msgType})
		return fmt.Errorf("actor %d: mailbox channel is full", a.actorID)
	}
}

// dealLeftTask processes all remaining messages in the mailbox during a graceful shutdown.
// This ensures that no messages are lost when the actor is terminated.
func (a *actorRuntime) dealLeftTask() {
	if len(a.pkgCtxChan) == 0 {
		return
	}
	a.actor.Info().Int("count", len(a.pkgCtxChan)).Msg("processing remaining tasks on exit")
	for {
		select {
		case ctx := <-a.pkgCtxChan:
			if err := a.handlePkg(ctx); err != nil {
				ctx.Error().Err(err).Msg("failed to handle remaining package on exit")
			}
		default:
			// The channel is empty, all tasks are processed.
			return
		}
	}
}

// runLoop starts and manages the actor's main event processing loop.
// It initializes the actor once, then drives mailbox/tick/shutdown on a single select loop.
func (a *actorRuntime) runLoop() {
	a.actor.Info().Msg("stateful actor task loop started")
	defer a.actor.Info().Msg("stateful actor task loop exited")

	if err := a.initActor(); err != nil {
		a.actor.Error().Err(err).Msg("actor initialization failed")
		return
	}

	ticker := time.NewTicker(time.Millisecond * time.Duration(a.msgLayer.TickPeriodMillSec))
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			a.closed.Store(true)
			a.dealLeftTask()
			a.actor.OnGracefulExit()
			return
		case hCtx := <-a.pkgCtxChan:
			a.lastActive = time.Now().Unix()
			if err := a.handlePkg(hCtx); err != nil {
				hCtx.Error().Err(err).Msg("failed to handle package")
			}
		case <-ticker.C:
			a.tick()
		}
	}
}

// tick performs periodic actions for the actor. This includes invoking the actor's
// OnTick method, triggering periodic state saves, and checking for inactivity to
// time out idle actors.
func (a *actorRuntime) tick() {
	startTime := time.Now()
	metrics.IncrCounterWithGroup("net.stateful", "actor_tick_total", 1)

	// Execute user-defined tick logic.
	if err := a.actor.OnTick(); err != nil {
		a.actor.Error().Err(err).Msg("error during actor OnTick")
		metrics.IncrCounterWithGroup("net.stateful", "actor_tick_error_total", 1)
	}
	metrics.RecordStopwatchWithGroup("net.stateful", "actor_tick_process_time", startTime)

	now := time.Now().Unix()
	cfg := a.msgLayer.StatefulConfig

	// Handle periodic saving.
	if cfg.SaveCategory.FreqSaveSecond > 0 && now >= a.lastFreqSaveSec+int64(cfg.SaveCategory.FreqSaveSecond) {
		saveStartTime := time.Now()
		a.actor.Save()
		metrics.RecordStopwatchWithGroup("net.stateful", "actor_save_time", saveStartTime)
		a.lastFreqSaveSec = now
	}

	// Handle idle timeout.
	if cfg.ActorLifeSecond > 0 && now >= a.lastActive+int64(cfg.ActorLifeSecond) {
		a.actor.Info().Int64("idle_seconds", now-a.lastActive).Msg("actor timed out due to inactivity")
		metrics.IncrCounterWithGroup("net.stateful", "actor_timeout_total", 1)
		a.exitActor()
	}
}

// handlePkg is the core logic for processing a single message. It decodes the message body,
// finds the appropriate handler function from the message's protocol info, and executes it.
// For request-type messages, it also handles sending the response back.
func (a *actorRuntime) handlePkg(hCtx *HandleContext) (err error) {
	startTime := time.Now()
	msgID := hCtx.ProtoInfo.MsgID
	isReq := hCtx.ProtoInfo.IsReq()

	msgTypeStr := "notification"
	if isReq {
		msgTypeStr = "request"
	}
	dimensions := map[string]string{"msg_id": msgID, "message_type": msgTypeStr}
	metrics.IncrCounterWithDimGroup("net.stateful", "actor_message_process_total", 1, dimensions)

	defer func() {
		// This defer block ensures metrics are recorded even if panics occur,
		// though a proper panic recovery mechanism would be more robust.
		metrics.RecordStopwatchWithDimGroup("net.stateful", "actor_message_process_time", startTime, dimensions)
		if err != nil {
			metrics.IncrCounterWithDimGroup("net.stateful", "actor_message_error_total", 1, dimensions)
		}
	}()

	body, err := hCtx.Pkg.DecodeBody()
	if err != nil {
		dimensions["error_type"] = "decode"
		return err
	}

	handle, ok := hCtx.ProtoInfo.GetMsgHandle().(MsgHandle)
	if !ok || handle == nil {
		dimensions["error_type"] = "no_handler"
		return errors.New("message handler not found or has incorrect type")
	}

	hCtx.beforeSendPkgToClient = a.beforeSendPkgToClient

	// Execute the business logic handler.
	res, code := handle(hCtx, a.actor, body)

	// If it was a request, send the response.
	if isReq {
		if err = hCtx.sendBack(code, res); err != nil {
			dimensions["error_type"] = "send_back"
			// The original error is overwritten here; might be better to wrap it.
		}
	}

	if code != int32(pb.EAsuraRetCode_AsuraRetCodeOK) {
		dimensions["error_type"] = "business"
		dimensions["ret_code"] = fmt.Sprintf("%d", code)
		hCtx.Warn().Str("MsgID", msgID).Int32("RetCode", code).Msg("handler returned non-OK status")
	} else {
		metrics.IncrCounterWithDimGroup("net.stateful", "actor_message_success_total", 1, dimensions)
	}

	return err
}

// beforeSendPkgToClient is a callback executed just before a response/notification is sent.
// It can trigger a state save based on the message type and configuration, ensuring that
// state is persisted after certain operations.
func (a *actorRuntime) beforeSendPkgToClient(resPkg *transport.TransSendPkg) error {
	pi, ok := message.GetProtoInfo(resPkg.PkgHdr.GetMsgID())
	if !ok {
		return fmt.Errorf("unknown message ID '%s' for actor %d", resPkg.PkgHdr.GetMsgID(), a.actorID)
	}

	// Trigger save if configured to do so after handling responses or notifications.
	if (pi.IsRes() || pi.IsNtf()) && a.msgLayer.SaveCategory.AfterHandleSave {
		a.actor.Save()
	}
	return nil
}
