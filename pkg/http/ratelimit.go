package http

import (
	"context"
	"math/rand/v2"
	"strconv"
	"time"

	nethttp "net/http"

	"github.com/leaktk/leaktk/pkg/logger"
)

const (
	maxRPS = float64(16.0)
	minRPS = float64(1.0 / 1024.0)
)

type RateLimit struct {
	curRPS float64
}

func parseRetryAfter(retryAfter string) (bool, time.Duration) {
	if retryAfter == "" {
		return false, 0
	}
	if seconds, err := strconv.Atoi(retryAfter); err == nil {
		return true, time.Duration(seconds) * time.Second
	}
	if t, err := time.Parse(nethttp.TimeFormat, retryAfter); err == nil {
		return true, time.Until(t)
	}
	return false, 0
}

func waitDuration(ctx context.Context, d time.Duration) error {
	logger.Debug("waiting duration: milliseconds=%v", d.Milliseconds())
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// Limit request frequencies based on response info
func (r *RateLimit) Limit(ctx context.Context, resp *nethttp.Response) error {
	// Ensure curRPS is set (start it off at max requests per second)
	if r.curRPS == 0 {
		r.curRPS = maxRPS
	}

	switch resp.StatusCode {
	case 429:
		// Use Retry-After when present else fall back on AIMD algo
		if s := resp.Header.Get("Retry-After"); len(s) > 0 {
			logger.Debug("Retry-After header set: value=%q url=%q", s, resp.Request.URL)
			if ok, retryAfter := parseRetryAfter(s); ok {
				// Set it back to max and wait the retry-after period
				r.curRPS = maxRPS
				return waitDuration(ctx, retryAfter)
			}
		}
		// Multiplicative Decrease: Cut the rate by 50%
		r.curRPS = max(minRPS, r.curRPS*0.5)
	case 200:
		// Additive Increase: Gain ~1.0 RPS for every second of successful resps
		r.curRPS = min(maxRPS, r.curRPS+(1.0/r.curRPS))
	}

	// jitter range: [0, 0.25)
	jitter := rand.Float64() * 0.25 // #nosec G404
	delay := time.Duration((1.0/r.curRPS)+jitter) * time.Second
	return waitDuration(ctx, delay)
}
