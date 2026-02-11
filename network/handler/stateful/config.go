// Package stateful provides the implementation for the stateful message processing layer.
// This file defines the configuration structures for the layer's behavior.
package stateful

import "fmt"

// ActorSaveCategory defines the configuration for an actor's state persistence strategies.
// It controls when and how an actor's `Save()` method is automatically triggered.
type ActorSaveCategory struct {
	// AfterHandleSave, if true, triggers a save after the actor handles any message
	// that could potentially modify its state.
	AfterHandleSave bool `mapstructure:"afterHandleSave"`
	// FreqSave, if true, enables periodic saving of the actor's state.
	FreqSave bool `mapstructure:"freqSave"`
	// FreqSaveSecond specifies the interval in seconds for periodic saving when FreqSave is enabled.
	FreqSaveSecond uint32 `mapstructure:"freqSaveSecond"`
}

// StatefulConfig holds all configuration parameters for the stateful message layer.
// These settings govern actor lifecycle, resource management, and performance tuning.
type StatefulConfig struct {
	// SaveCategory configures the automatic persistence behavior for all actors.
	SaveCategory ActorSaveCategory `mapstructure:"saveCategory"`
	// ActorLifeSecond is the idle timeout in seconds. If an actor receives no messages
	// for this duration, it will be gracefully shut down to conserve resources.
	ActorLifeSecond int `mapstructure:"actorLifeSecond"`
	// TickPeriodMillSec defines the interval in milliseconds for the actor's internal
	// tick. The actor's `OnTick()` method is called at this frequency.
	TickPeriodMillSec int `mapstructure:"tickPeriodMillSec"`
	// GrHBTimeoutSec is the timeout in seconds for a graceful shutdown heartbeat or phase.
	GrHBTimeoutSec int `mapstructure:"grHBTimeoutSec"`
	// BroadcastQPS is the queries-per-second limit for broadcast operations to prevent overload.
	BroadcastQPS uint32 `mapstructure:"broadcastQPS"`
	// BroadcastMaxWaitNum is the maximum number of items that can be queued for broadcast.
	BroadcastMaxWaitNum int32 `mapstructure:"broadcastMaxWaitNum"`
	// MaxActorCount is the maximum number of concurrent actors allowed to exist on this server node.
	MaxActorCount int `mapstructure:"maxActorCount"`
	// ActorNotCreateRetCode is the error code returned to a client when a request fails
	// because its target actor could not be created (e.g., due to MaxActorCount limit).
	ActorNotCreateRetCode int32 `mapstructure:"actorNotCreateRetCode"`
	// ActorMsgFilters is a list of message IDs that should be filtered or blocked at this layer.
	ActorMsgFilters []string `mapstructure:"actorMsgFilters"`
	// MigrateFeatSwitch is a feature flag to enable or disable the actor state migration functionality.
	MigrateFeatSwitch bool `mapstructure:"migrateFeatSwitch"`
	// TaskChanSize defines the buffer size of each actor's incoming message channel (mailbox).
	// A smaller size will apply back-pressure more quickly.
	TaskChanSize int `mapstructure:"taskChanSize"`
	// GraceFulTimeoutSecond is the maximum time in seconds to wait for all actors to shut down
	// during a graceful server exit.
	GraceFulTimeoutSecond int `mapstructure:"graceFulTimeoutSecond"`
}

// GetName returns the key name for this configuration section.
func (c *StatefulConfig) GetName() string {
	return "stateful"
}

// Validate checks that the configuration parameters are within valid ranges.
func (c *StatefulConfig) Validate() error {
	if c.TickPeriodMillSec <= 0 {
		return fmt.Errorf("TickPeriodMillSec must be positive")
	}
	if c.GrHBTimeoutSec <= 0 {
		return fmt.Errorf("GrHBTimeoutSec must be positive")
	}
	if c.BroadcastQPS <= 0 {
		return fmt.Errorf("BroadcastQPS must be positive")
	}
	if c.MaxActorCount <= 0 {
		return fmt.Errorf("MaxActorCount must be positive")
	}
	if c.TaskChanSize <= 0 {
		return fmt.Errorf("TaskChanSize must be positive")
	}
	if c.GraceFulTimeoutSecond <= 0 {
		return fmt.Errorf("GraceFulTimeoutSecond must be positive")
	}
	return nil
}
