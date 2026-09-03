package proxy

import (
	"sync"
	"time"
)

// contractRateLimiter enforces the per-minute call ceiling stored on a data product
// contract scope (dw_contract_scopes.rate_limit, documented as "계약별 분당 호출량 제한").
// Windows are fixed and aligned to the wall-clock minute so Retry-After and the reset
// header are exact. State lives in this process: with more than one replica each
// instance enforces the ceiling against the traffic it serves.
//
// The zero value is ready to use.
type contractRateLimiter struct {
	mu      sync.Mutex
	windows map[string]contractRateWindow
}

type contractRateWindow struct {
	start time.Time
	count int
}

// rateLimiterMaxKeys bounds the tracked contract count before stale windows are pruned.
const rateLimiterMaxKeys = 512

// allow records one call against contractKey and reports whether it stayed inside the
// limit, how many calls the current window has consumed, and when that window resets.
// A non-positive limit means unlimited. Rejected calls do not consume the window.
func (l *contractRateLimiter) allow(contractKey string, limit int, now time.Time) (bool, int, time.Time) {
	start := now.UTC().Truncate(time.Minute)
	reset := start.Add(time.Minute)
	if limit <= 0 || contractKey == "" {
		return true, 0, reset
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.windows == nil {
		l.windows = map[string]contractRateWindow{}
	}
	window := l.windows[contractKey]
	if !window.start.Equal(start) {
		window = contractRateWindow{start: start}
		if len(l.windows) > rateLimiterMaxKeys {
			l.pruneLocked(start)
		}
	}
	if window.count >= limit {
		l.windows[contractKey] = window
		return false, window.count, reset
	}
	window.count++
	l.windows[contractKey] = window
	return true, window.count, reset
}

// pruneLocked drops windows that closed before the current one. Callers hold l.mu.
func (l *contractRateLimiter) pruneLocked(current time.Time) {
	for key, window := range l.windows {
		if window.start.Before(current) {
			delete(l.windows, key)
		}
	}
}
