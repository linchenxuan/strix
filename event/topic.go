package event

import "time"

// Strix framework provides several default topics.
const (
	// ReloadConfig update process configuration
	ReloadConfig = "ReloadConfig"
)

// Topic subscription list for a single topic.
type Topic struct {
	timeout     time.Duration // Publish timeout.
	subscribers []Subscriber  // Subscription queue.
}
