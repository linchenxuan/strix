package event

import (
	"fmt"
	"sync"
	"time"

	"github.com/linchenxuan/strix/log"
)

// maxTopicTimeout 是单个 topic 允许配置的最大发布超时时间。
const maxTopicTimeout = time.Second

// topic 保存单个主题的订阅者和发布超时配置。
type topic struct {
	timeout     time.Duration
	subscribers []Subscriber
}

// manager 负责管理 topic 和订阅关系。
type manager struct {
	topics         map[string]*topic
	maxSubscribers int
	mu             sync.RWMutex
}

var defaultManager = newManager()

// newManager 创建并返回一个新的事件管理器。
func newManager() *manager {
	return &manager{
		topics:         make(map[string]*topic),
		maxSubscribers: 1024,
	}
}

// NewTopic 创建一个新的 topic，并记录发布超时配置。
func (m *manager) NewTopic(topicName string, timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.topics[topicName]; exists {
		return fmt.Errorf("%w: topic %q already exists", ErrDuplicateTopic, topicName)
	}
	if timeout <= 0 || timeout > maxTopicTimeout {
		return fmt.Errorf("%w: topic %q timeout %s is outside range (0, %s]", ErrInvalidTopicTimeout, topicName, timeout, maxTopicTimeout)
	}

	m.topics[topicName] = &topic{
		timeout:     timeout,
		subscribers: make([]Subscriber, 0),
	}
	return nil
}

// RegisterSubscriber 为指定 topic 注册一个订阅者。
func (m *manager) RegisterSubscriber(topicName string, fn Subscriber) error {
	if fn == nil {
		return fmt.Errorf("%w: topic %q", ErrInvalidSubscriber, topicName)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	topic, ok := m.topics[topicName]
	if !ok {
		return fmt.Errorf("%w: topic %q", ErrTopicNotFound, topicName)
	}
	if len(topic.subscribers) >= m.maxSubscribers {
		return fmt.Errorf("%w: topic %q limit %d", ErrSubscriberLimit, topicName, m.maxSubscribers)
	}

	topic.subscribers = append(topic.subscribers, fn)
	return nil
}

// Publish 向指定 topic 的所有订阅者并发发布事件，并等待所有订阅者完成的最长时间。
// 当发布超时时，Publish 返回 ErrPublishTimeout，但不会主动中断已经启动的订阅者，
// 这些订阅者仍会继续执行直到自行结束，因此 timeout 不提供取消执行的语义。
func (m *manager) Publish(topicName string, payload any) error {
	m.mu.RLock()
	topic, ok := m.topics[topicName]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("%w: topic %q", ErrTopicNotFound, topicName)
	}
	subscribers := append([]Subscriber(nil), topic.subscribers...)
	timeout := topic.timeout
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, subscriber := range subscribers {
		wg.Add(1)
		go func(sub Subscriber) {
			defer wg.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Error().
						Str("topic", topicName).
						Any("panic", recovered).
						Msg("event subscriber panic")
				}
			}()
			sub(payload)
		}(subscriber)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("%w: topic %q", ErrPublishTimeout, topicName)
	}
}
