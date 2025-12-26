package event

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func newTestPublisher() *Publisher {
	return &Publisher{
		topics: make(map[string]*Topic),
	}
}

func TestNewTopic(t *testing.T) {
	p := newTestPublisher()
	topicName := "test-topic"

	// 1. 成功创建
	err := p.NewTopic(topicName, time.Second)
	assert.NoError(t, err)

	_, ok := p.topics[topicName]
	assert.True(t, ok, "topic should be created")

	// 2. 创建已存在的主题
	err = p.NewTopic(topicName, time.Second)
	assert.Error(t, err, "should return error when topic already exists")
}

func TestRegisterSubscriber(t *testing.T) {
	p := newTestPublisher()
	topicName := "test-topic"
	nonExistentTopic := "non-existent-topic"

	// 1. 为不存在的主题注册
	err := p.RegisterSubscriber(nonExistentTopic, func(param any) {})
	assert.Error(t, err, "should return error when topic does not exist")

	// 2. 成功注册
	_ = p.NewTopic(topicName, time.Second)
	err = p.RegisterSubscriber(topicName, func(param any) {})
	assert.NoError(t, err)
	assert.Len(t, p.topics[topicName].subscribers, 1, "subscriber should be added")
}

func TestPublish(t *testing.T) {
	p := newTestPublisher()
	topicName := "test-topic"
	nonExistentTopic := "non-existent-topic"
	message := "hello world"

	// 1. 向不存在的主题发布
	err := p.Publish(nonExistentTopic, message)
	assert.Error(t, err, "should return error when publishing to a non-existent topic")

	// 2. 成功发布
	_ = p.NewTopic(topicName, time.Second)

	received := make(map[int]string)
	var mu sync.Mutex

	subscriber1 := func(param any) {
		mu.Lock()
		received[1] = param.(string)
		mu.Unlock()
	}

	subscriber2 := func(param any) {
		mu.Lock()
		received[2] = param.(string)
		mu.Unlock()
	}

	_ = p.RegisterSubscriber(topicName, subscriber1)
	_ = p.RegisterSubscriber(topicName, subscriber2)

	err = p.Publish(topicName, message)
	assert.NoError(t, err)

	mu.Lock()
	assert.Equal(t, message, received[1], "subscriber 1 should receive the message")
	assert.Equal(t, message, received[2], "subscriber 2 should receive the message")
	mu.Unlock()
}
