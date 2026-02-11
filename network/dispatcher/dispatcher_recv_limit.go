// Package dispatcher provides the central message routing and processing engine for the Strix
// network stack. This file contains implementations of rate limiters used to control the
// message processing rate, protecting the server from traffic overload.
package dispatcher

import (
	"context"
	"sync/atomic"

	"go.uber.org/ratelimit"
	"golang.org/x/time/rate"
)

// DispatcherRecvLimiter implements a token bucket-based rate limiter for incoming messages.
// It is used as a filter in the dispatcher chain to enforce a maximum message processing rate.
// The token bucket algorithm allows for short bursts of traffic that exceed the steady-state
// limit, which can be useful for accommodating natural variations in request flow.
// This implementation is thread-safe for configuration reloads.
type DispatcherRecvLimiter struct {
	// limiter holds an atomic pointer to the underlying rate limiter instance.
	// Using an atomic pointer allows the limiter's configuration (rate and burst)
	// to be hot-reloaded concurrently without data races.
	limiter atomic.Pointer[rate.Limiter]
}

// NewTokenRecvLimiter creates a new DispatcherRecvLimiter.
// It initializes a token bucket rate limiter with a specified steady-state rate and burst size.
//
// - limit: The number of events allowed per second.
// - burst: The maximum number of tokens that can be accumulated, representing the allowed burst size.
func NewTokenRecvLimiter(limit int, burst int) *DispatcherRecvLimiter {
	limiter := rate.NewLimiter(rate.Limit(limit), burst)
	if limiter == nil {
		// This should theoretically not happen with valid inputs.
		return nil
	}
	self := &DispatcherRecvLimiter{}
	self.limiter.Store(limiter)
	return self
}

// Take blocks until a token is available from the bucket, or until the context is canceled.
// This call effectively enforces the rate limit. It is called by the `recvLimiterFilter`.
func (l *DispatcherRecvLimiter) Take() error {
	// Wait for a token to become available.
	return l.limiter.Load().Wait(context.Background())
}

// Reload safely updates the rate and burst size of the limiter at runtime.
// It creates a new limiter instance and atomically swaps the pointer, ensuring that
// concurrent calls to Take() will use either the old or the new limiter, but never a
// corrupted state.
func (l *DispatcherRecvLimiter) Reload(limit int, burst int) {
	newLimiter := rate.NewLimiter(rate.Limit(limit), burst)
	if newLimiter == nil {
		return
	}
	l.limiter.Store(newLimiter)
}

// recvLimiterFilter is a DispatcherFilter that applies the rate limit.
// It calls the limiter's Take() method, which will block if the rate of incoming
// messages exceeds the configured limit. Once a token is acquired, it passes
// control to the next filter in the chain.
func (l *DispatcherRecvLimiter) recvLimiterFilter(d *DispatcherDelivery, f DispatcherFilterHandleFunc) error {
	if err := l.Take(); err != nil {
		return err // This could happen if the context is canceled.
	}
	return f(d)
}

// FunnelRecvLimiter is an alternative rate limiter implementation based on the leaky bucket algorithm,
// using Uber's `ratelimit` package. A leaky bucket provides a more constant, smooth output rate,
// as it does not allow for bursting in the same way a token bucket does.
// This implementation is kept as a potential alternative but is not the default used by the Dispatcher.
type FunnelRecvLimiter struct {
	// limiter holds an atomic pointer to the underlying leaky bucket rate limiter.
	// This allows for concurrent-safe hot-reloading of the rate limit.
	limiter atomic.Pointer[ratelimit.Limiter]
}

// NewFunnelRecvLimiter creates a new FunnelRecvLimiter.
//
// - limit: The maximum number of events allowed per second (the rate at which the bucket leaks).
func NewFunnelRecvLimiter(limit int) *FunnelRecvLimiter {
	limiter := ratelimit.New(limit)
	if limiter == nil {
		return nil
	}
	self := &FunnelRecvLimiter{}
	self.limiter.Store(&limiter)
	return self
}

// Take blocks until a request is allowed to pass according to the leaky bucket's rate.
func (l *FunnelRecvLimiter) Take() {
	(*l.limiter.Load()).Take()
}

// Reload safely updates the rate of the leaky bucket limiter at runtime.
// It creates a new limiter instance and atomically swaps it with the old one.
func (l *FunnelRecvLimiter) Reload(limit int) {
	newLimiter := ratelimit.New(limit)
	if newLimiter == nil {
		return
	}
	l.limiter.Store(&newLimiter)
}
