// Package stateful provides the implementation for the stateful message processing layer.
// This file defines the actorMgr, which is responsible for managing the lifecycle of
// all actor instances within the system.
package stateful

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/linchenxuan/strix/log"
	"github.com/linchenxuan/strix/metrics"
)

// actorMgr is the central supervisor for all actors in the stateful layer.
// It handles the creation, retrieval, and graceful shutdown of actors in a thread-safe manner.
// It ensures that actor instances are created on-demand and cleaned up when they exit.
type actorMgr struct {
	creator  ActorCreator             // The factory function used to create new Actor instances.
	actorMap map[uint64]*actorRuntime // A map from actor ID to its runtime, holding all active actors.
	lock     sync.RWMutex             // A lock to protect concurrent access to the actorMap.
	closed   int32                    // An atomic flag indicating if the manager is shutting down.
	msgLayer *StatefulMsgLayer        // A reference to the parent message layer for accessing configuration.
}

// _gracefulExitTick defines the polling interval for checking actor shutdown status.
const _gracefulExitTick = time.Millisecond * 10

// newActorMgr creates a new actor manager.
func newActorMgr(creator ActorCreator) *actorMgr {
	return &actorMgr{
		creator:  creator,
		actorMap: make(map[uint64]*actorRuntime),
	}
}

// actorCount returns the current number of active actors managed by this manager.
func (mgr *actorMgr) actorCount() int {
	mgr.lock.RLock()
	defer mgr.lock.RUnlock()
	return len(mgr.actorMap)
}

// getActorRuntime retrieves an actor's runtime by its ID in a thread-safe manner.
// It returns the runtime and a boolean indicating if the actor was found.
func (mgr *actorMgr) getActorRuntime(id uint64) (*actorRuntime, bool) {
	mgr.lock.RLock()
	defer mgr.lock.RUnlock()
	a, ok := mgr.actorMap[id]
	return a, ok
}

// createActor is a thread-safe "get-or-create" method for actors.
// If an actor with the given ID already exists, it is returned. Otherwise, a new actor
// is created, its runtime is initialized, and its event loop is started in a new goroutine.
func (mgr *actorMgr) createActor(aid uint64) (*actorRuntime, error) {
	mgr.lock.Lock()
	defer mgr.lock.Unlock()

	metrics.IncrCounterWithGroup("net.stateful", "actor_create_attempt_total", 1)

	// First, check if the actor already exists.
	if a, ok := mgr.actorMap[aid]; ok {
		metrics.IncrCounterWithDimGroup("net.stateful", "actor_create_attempt_total", 1, map[string]string{"result": "already_exists"})
		return a, nil
	}

	// If not, check if the manager is shutting down.
	if atomic.LoadInt32(&mgr.closed) != 0 {
		metrics.IncrCounterWithDimGroup("net.stateful", "actor_create_failed_total", 1, map[string]string{"reason": "manager_closed"})
		return nil, fmt.Errorf("cannot create actor %d: manager is closed", aid)
	}

	// Enforce the maximum actor count limit.
	if len(mgr.actorMap) >= mgr.msgLayer.MaxActorCount {
		metrics.IncrCounterWithDimGroup("net.stateful", "actor_create_failed_total", 1, map[string]string{"reason": "actor_limit_exceeded"})
		return nil, fmt.Errorf("cannot create actor %d: actor limit of %d reached", aid, mgr.msgLayer.MaxActorCount)
	}

	// Create the new actor using the provided factory function.
	actor, err := mgr.creator(aid)
	if err != nil {
		metrics.IncrCounterWithDimGroup("net.stateful", "actor_create_failed_total", 1, map[string]string{"reason": "creator_error"})
		return nil, fmt.Errorf("failed to create actor %d: %w", aid, err)
	}

	// Create and register the actor's runtime.
	ar := newActorRuntime(mgr.msgLayer, aid, actor)
	actor.SetActorExitFunc(ar.exitActor)
	mgr.actorMap[aid] = ar

	metrics.UpdateGaugeWithGroup("net.stateful", "active_actors_count", metrics.Value(len(mgr.actorMap)))
	metrics.IncrCounterWithGroup("net.stateful", "actor_created_total", 1)

	// Start the actor's event loop in a new goroutine.
	go func() {
		// This defer ensures the actor is removed from the map when its loop exits, preventing memory leaks.
		defer func() {
			mgr.lock.Lock()
			delete(mgr.actorMap, aid)
			mgr.lock.Unlock()
			metrics.UpdateGaugeWithGroup("net.stateful", "active_actors_count", metrics.Value(len(mgr.actorMap)))
			metrics.IncrCounterWithGroup("net.stateful", "actor_destroyed_total", 1)
		}()
		ar.runLoop()
	}()

	return ar, nil
}

// deleteActor requests the graceful shutdown of an actor by its ID.
// It finds the actor's runtime and cancels its context, which triggers the exit sequence.
func (mgr *actorMgr) deleteActor(id uint64) {
	if a, ok := mgr.getActorRuntime(id); ok {
		a.Cancel()
	} else {
		log.Warn().Uint64("aid", id).Msg("attempted to delete an actor that does not exist or has already exited")
	}
}

// getAllActors returns a slice containing all currently active actor runtimes.
// This is useful for operations that need to iterate over all actors, like broadcasting.
func (mgr *actorMgr) getAllActors() []*actorRuntime {
	mgr.lock.RLock()
	defer mgr.lock.RUnlock()
	ret := make([]*actorRuntime, 0, len(mgr.actorMap))
	for _, v := range mgr.actorMap {
		ret = append(ret, v)
	}
	return ret
}

// getAllActorsAID returns a slice of all active actor IDs.
func (mgr *actorMgr) getAllActorsAID() []uint64 {
	mgr.lock.RLock()
	defer mgr.lock.RUnlock()
	ret := make([]uint64, 0, len(mgr.actorMap))
	for aid := range mgr.actorMap {
		ret = append(ret, aid)
	}
	return ret
}

// shutdown orchestrates the graceful shutdown of all actors managed by the manager.
// It notifies all actors to exit and then waits for them to terminate, with a configurable timeout.
func (mgr *actorMgr) shutdown() {
	log.Info().Msg("actor manager shutdown sequence started")
	defer log.Info().Msg("actor manager shutdown sequence finished")

	// Signal all actors to begin their graceful exit.
	mgr.ntfActorExit()

	startTime := time.Now()
	timeout := time.Duration(mgr.msgLayer.GraceFulTimeoutSecond) * time.Second
	ticker := time.NewTicker(_gracefulExitTick)
	defer ticker.Stop()

	// Poll until all actors have exited or the timeout is reached.
	for {
		select {
		case <-ticker.C:
			if mgr.actorCount() == 0 {
				log.Info().Msg("all actors have exited gracefully")
				return
			}
			if time.Since(startTime) > timeout {
				log.Error().Int("remaining", mgr.actorCount()).Msg("graceful shutdown timed out; some actors did not exit")
				return
			}
		}
	}
}

// ntfActorExit notifies all running actors to begin their graceful shutdown process.
// It sets the `closed` flag to prevent new actors from being created during shutdown.
func (mgr *actorMgr) ntfActorExit() {
	// Atomically set the closed flag to prevent new actor creations.
	atomic.StoreInt32(&mgr.closed, 1)

	actorsToExit := mgr.getAllActors()
	log.Info().Int("count", len(actorsToExit)).Msg("notifying all actors to exit")

	for _, a := range actorsToExit {
		a.exitActor()
	}
}
