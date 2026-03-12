package tracing

import "sync/atomic"

// SamplerDecision 采样决策。
type SamplerDecision uint8

const (
	// Drop 不采样。
	Drop SamplerDecision = 0
	// Sample 采样。
	Sample SamplerDecision = 1
)

// Sampler 采样器接口。
type Sampler interface {
	ShouldSample(opCode uint16) SamplerDecision
}

// RateSampler 按固定比例采样。
// rate=1 全量采样，rate=100 即 1% 采样，rate=0 不采样。
type RateSampler struct {
	rate    uint32         // 采样率分母，每 rate 次采样 1 次。
	counter atomic.Uint64  // 无锁调用计数。
}

// NewRateSampler 创建一个固定比例采样器。
func NewRateSampler(rate uint32) *RateSampler {
	return &RateSampler{rate: rate}
}

// ShouldSample 判断是否采样。
func (s *RateSampler) ShouldSample(_ uint16) SamplerDecision {
	if s.rate == 0 {
		return Drop
	}
	if s.rate == 1 {
		return Sample
	}
	c := s.counter.Add(1)
	if c%uint64(s.rate) == 0 {
		return Sample
	}
	return Drop
}

// AlwaysSampler 始终采样。
type AlwaysSampler struct{}

// ShouldSample 始终返回 Sample。
func (AlwaysSampler) ShouldSample(_ uint16) SamplerDecision {
	return Sample
}

// NeverSampler 从不采样。
type NeverSampler struct{}

// ShouldSample 始终返回 Drop。
func (NeverSampler) ShouldSample(_ uint16) SamplerDecision {
	return Drop
}
