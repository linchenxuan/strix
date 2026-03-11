package event

import "errors"

var (
	// ErrTopicNotFound 表示目标 topic 不存在。
	ErrTopicNotFound = errors.New("topic not found")
	// ErrDuplicateTopic 表示 topic 名称重复创建。
	ErrDuplicateTopic = errors.New("duplicate topic")
	// ErrInvalidTopicTimeout 表示 topic 超时时间超过支持上限。
	ErrInvalidTopicTimeout = errors.New("invalid topic timeout")
	// ErrSubscriberLimit 表示 topic 的订阅者数量达到上限。
	ErrSubscriberLimit = errors.New("subscriber limit reached")
	// ErrPublishTimeout 表示事件发布在超时时间内未完成。
	ErrPublishTimeout = errors.New("publish timeout")
	// ErrInvalidSubscriber 表示注册了无效的订阅者。
	ErrInvalidSubscriber = errors.New("invalid subscriber")
)
