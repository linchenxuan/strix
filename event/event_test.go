package event

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPublishDeliversPayloadToSubscriber(t *testing.T) {
	resetManagerForTest()

	err := NewTopic("demo", 50*time.Millisecond)
	assert.NoError(t, err)

	received := make(chan string, 1)
	err = RegisterSubscriber("demo", func(v any) {
		received <- v.(string)
	})
	assert.NoError(t, err)

	err = Publish("demo", "payload")
	assert.NoError(t, err)
	assert.Equal(t, "payload", <-received)
}

func TestNewTopicReturnsDuplicateError(t *testing.T) {
	resetManagerForTest()

	assert.NoError(t, NewTopic("demo", 50*time.Millisecond))

	err := NewTopic("demo", 50*time.Millisecond)
	assert.ErrorIs(t, err, ErrDuplicateTopic)
}

func TestRegisterSubscriberReturnsTopicNotFound(t *testing.T) {
	resetManagerForTest()

	err := RegisterSubscriber("missing", func(any) {})
	assert.ErrorIs(t, err, ErrTopicNotFound)
}

func TestPublishReturnsTopicNotFound(t *testing.T) {
	resetManagerForTest()

	err := Publish("missing", nil)
	assert.ErrorIs(t, err, ErrTopicNotFound)
}

func TestPublishReturnsTimeoutWhenSubscriberBlocks(t *testing.T) {
	resetManagerForTest()

	assert.NoError(t, NewTopic("demo", 10*time.Millisecond))
	assert.NoError(t, RegisterSubscriber("demo", func(any) {
		time.Sleep(30 * time.Millisecond)
	}))

	err := Publish("demo", "payload")
	assert.ErrorIs(t, err, ErrPublishTimeout)
}

func TestNewTopicReturnsInvalidTimeout(t *testing.T) {
	resetManagerForTest()

	err := NewTopic("demo", maxTopicTimeout+time.Millisecond)
	assert.ErrorIs(t, err, ErrInvalidTopicTimeout)
}

func TestNewTopicReturnsInvalidTimeoutForZero(t *testing.T) {
	resetManagerForTest()

	err := NewTopic("demo", 0)
	assert.ErrorIs(t, err, ErrInvalidTopicTimeout)
}

func TestRegisterSubscriberReturnsSubscriberLimit(t *testing.T) {
	resetManagerForTest()
	defaultManager.maxSubscribers = 1

	assert.NoError(t, NewTopic("demo", 50*time.Millisecond))
	assert.NoError(t, RegisterSubscriber("demo", func(any) {}))

	err := RegisterSubscriber("demo", func(any) {})
	assert.ErrorIs(t, err, ErrSubscriberLimit)
}

func TestRegisterSubscriberReturnsInvalidSubscriber(t *testing.T) {
	resetManagerForTest()

	assert.NoError(t, NewTopic("demo", 50*time.Millisecond))

	err := RegisterSubscriber("demo", nil)
	assert.ErrorIs(t, err, ErrInvalidSubscriber)
}

func TestPublishRecoversSubscriberPanic(t *testing.T) {
	resetManagerForTest()

	assert.NoError(t, NewTopic("demo", 50*time.Millisecond))
	assert.NoError(t, RegisterSubscriber("demo", func(any) {
		panic("boom")
	}))

	done := make(chan struct{}, 1)
	assert.NoError(t, RegisterSubscriber("demo", func(any) {
		done <- struct{}{}
	}))

	err := Publish("demo", nil)
	assert.NoError(t, err)
	<-done
}

func TestPublishTimeoutDoesNotCancelSubscriber(t *testing.T) {
	resetManagerForTest()

	assert.NoError(t, NewTopic("demo", 10*time.Millisecond))

	done := make(chan struct{}, 1)
	assert.NoError(t, RegisterSubscriber("demo", func(any) {
		time.Sleep(30 * time.Millisecond)
		done <- struct{}{}
	}))

	err := Publish("demo", nil)
	assert.ErrorIs(t, err, ErrPublishTimeout)
	<-done
}
