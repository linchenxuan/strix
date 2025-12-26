package event

import (
	"fmt"
	"sync"
	"time"

	"github.com/linchenxuan/strix/log"
)

// Publisher includes multiple topics.
type Publisher struct {
	lock   sync.RWMutex
	topics map[string]*Topic // Subscriber information.
}

// NewTopic must create a topic before you can initiate a subscription.
func (p *Publisher) NewTopic(topicName string, timeout time.Duration) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	_, ok := p.topics[topicName]
	if ok {
		return fmt.Errorf("topic %s already create", topicName)
	}
	topic := &Topic{
		timeout:     timeout,
		subscribers: []Subscriber{},
	}

	p.topics[topicName] = topic
	return nil
}

// RegisterSubscriber registers a subscriber.
func (p *Publisher) RegisterSubscriber(topicName string, fn Subscriber) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	topic, ok := p.topics[topicName]
	if !ok {
		return fmt.Errorf("topic %s not create", topicName)
	}

	topic.subscribers = append(topic.subscribers, fn)
	log.Info().Str("Topic", topicName).
		Int("num", len(topic.subscribers)).Msg("add subscribers")
	return nil
}

// Publish post content.
func (p *Publisher) Publish(topicName string, i any) error {
	p.lock.RLock()
	defer p.lock.RUnlock()

	topic, ok := p.topics[topicName]
	if !ok {
		return fmt.Errorf("topic:%s not create", topicName)
	}

	log.Info().Str("topic", topicName).Int("subscribers num", len(topic.subscribers)).Msg("publish event")

	var wg sync.WaitGroup

	for _, sub := range topic.subscribers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub(i)
		}()
	}

	wg.Wait()

	return nil
}
