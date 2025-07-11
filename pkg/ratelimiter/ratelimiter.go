package ratelimiter

import (
	"context"
	"time"
)

// RateLimiter enforces a strict rate limit on operations.
type RateLimiter struct {
	ticker  *time.Ticker
	ctx     context.Context
	first   chan struct{} // Channel to signal the first request, allowing it to pass immediately.
	stopped bool
}

// New creates a new RateLimiter.
func New(rate time.Duration, ctx context.Context) *RateLimiter {
	rl := &RateLimiter{
		ticker: time.NewTicker(rate),
		ctx:    ctx,
		first:  make(chan struct{}, 1),
	}
	// Pre-fill the channel so the first Wait() call returns immediately.
	rl.first <- struct{}{}
	return rl
}

// Wait blocks until the next token is available from the ticker, or until the context is done.
func (r *RateLimiter) Wait() error {
	// The first request will consume from the pre-filled `first` channel and return instantly.
	// Subsequent requests will find the channel empty and block on the ticker.
	select {
	case <-r.first:
		return nil
	default:
	}

	select {
	case <-r.ticker.C:
		return nil
	case <-r.ctx.Done():
		r.stopped = true
		return r.ctx.Err()
	}
}

// Stop releases resources used by the RateLimiter.
func (r *RateLimiter) Stop() {
	if !r.stopped {
		r.ticker.Stop()
		r.stopped = true
	}
}
