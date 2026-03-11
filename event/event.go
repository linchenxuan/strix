package event

import "time"

// Subscriber 定义了事件订阅者的处理函数。
type Subscriber func(any)

// NewTopic 在默认事件管理器上创建一个 topic。
func NewTopic(topicName string, timeout time.Duration) error {
	return defaultManager.NewTopic(topicName, timeout)
}

// RegisterSubscriber 在默认事件管理器上为指定 topic 注册订阅者。
func RegisterSubscriber(topicName string, fn Subscriber) error {
	return defaultManager.RegisterSubscriber(topicName, fn)
}

// Publish 在默认事件管理器上向指定 topic 发布事件。
func Publish(topicName string, payload any) error {
	return defaultManager.Publish(topicName, payload)
}
