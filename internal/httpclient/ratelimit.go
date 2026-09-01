package httpclient

import (
	"context"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/leaktk/leaktk/pkg/logger"
)

const (
	maxRPS = float64(16.0)         // 16 requests per second max
	minRPS = float64(1.0 / 1024.0) // ~ 17 min wait
)

type RateLimit struct {
	hostLimits map[string]*hostLimit
	m          sync.Mutex
}

var (
	rateLimitOnce sync.Once
	rateLimit     *RateLimit
)

type hostLimit struct {
	rps   float64   // allowed requests per second
	after time.Time // time after which the next request is allowed
}

func NewRateLimit() *RateLimit {
	rateLimitOnce.Do(func() {
		rateLimit = &RateLimit{
			hostLimits: make(map[string]*hostLimit, 1),
		}
	})

	return rateLimit
}

func parseRetryAfter(retryAfter string) (time.Duration, bool) {
	if retryAfter == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(retryAfter); err == nil {
		return time.Duration(seconds) * time.Second, true
	}
	if t, err := time.Parse(http.TimeFormat, retryAfter); err == nil {
		return time.Until(t), true
	}
	return 0, false
}

func rpsToDelay(rps float64) time.Duration {
	// jitter range: [0, 0.25)
	jitter := rand.Float64() * 0.25 // #nosec G404
	return time.Duration((1.0/rps)+jitter) * time.Second
}

func (r *RateLimit) loadHostLimits(host string) *hostLimit {
	hl, ok := r.hostLimits[host]
	if !ok {
		hl = &hostLimit{rps: maxRPS}
		r.hostLimits[host] = hl
	}
	return hl
}

func (r *RateLimit) Wait(ctx context.Context, req *http.Request) error {
	// Calc delay between now and the time after which the next request is allowed
	r.m.Lock()
	delay := time.Until(r.loadHostLimits(req.URL.Host).after)
	r.m.Unlock()

	if delay < 0 {
		return nil
	}

	logger.Debug("waiting duration: milliseconds=%v", delay.Milliseconds())

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func (r *RateLimit) Update(resp *http.Response) {
	r.m.Lock()
	defer r.m.Unlock()

	// Get a pointer to the current host limits
	hl := r.loadHostLimits(resp.Request.URL.Host)

	switch resp.StatusCode {
	case 429:
		// Multiplicative Decrease: Cut the rate by 50% (by default unless Retry-After is set)
		hl.rps = max(minRPS, hl.rps*0.5)

		// Use Retry-After when present else fall back on AIMD algo
		if s := resp.Header.Get("Retry-After"); len(s) > 0 {
			logger.Debug("Retry-After header set: value=%q url=%q", s, resp.Request.URL)
			if retryAfter, ok := parseRetryAfter(s); ok {
				hl.rps = maxRPS
				hl.after = time.Now().Add(retryAfter)
				return
			} else {
				logger.Error("could not parse Retry-After header, falling back calculated rate limit")

			}
		}
	case 200:
		// Additive Increase: Gain ~1.0 RPS for every second of successful resps
		hl.rps = min(maxRPS, hl.rps+(1.0/hl.rps))
	}

	// Set the time after which the next request is allowed
	hl.after = time.Now().Add(rpsToDelay(hl.rps))
}
