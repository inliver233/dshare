package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	mu         sync.Mutex
	windows    map[string][]time.Time
	inflight   map[int64]int
	lastPruned time.Time
}

type DenyReason string

const (
	DenyNone       DenyReason = ""
	DenyConcurrent DenyReason = "concurrent"
	DenyWindow     DenyReason = "window"
)

func New() *Limiter {
	return &Limiter{
		windows:  map[string][]time.Time{},
		inflight: map[int64]int{},
	}
}

func (l *Limiter) Allow(userID int64, maxConcurrent int, rules ...Rule) (ReleaseFunc, bool, time.Duration) {
	release, ok, retryAfter, _ := l.AllowDetailed(userID, maxConcurrent, rules...)
	return release, ok, retryAfter
}

func (l *Limiter) AllowDetailed(userID int64, maxConcurrent int, rules ...Rule) (ReleaseFunc, bool, time.Duration, DenyReason) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastPruned) > time.Minute {
		l.pruneLocked(now)
	}
	if maxConcurrent > 0 && l.inflight[userID] >= maxConcurrent {
		return nil, false, time.Second, DenyConcurrent
	}
	var retryAfter time.Duration
	for _, rule := range rules {
		if rule.Limit <= 0 || rule.Window <= 0 {
			continue
		}
		key := rule.key(userID)
		items := trimWindow(l.windows[key], now.Add(-rule.Window))
		l.windows[key] = items
		if len(items) >= rule.Limit {
			wait := rule.Window - now.Sub(items[0])
			if wait < time.Second {
				wait = time.Second
			}
			if retryAfter == 0 || wait > retryAfter {
				retryAfter = wait
			}
		}
	}
	if retryAfter > 0 {
		return nil, false, retryAfter, DenyWindow
	}
	for _, rule := range rules {
		if rule.Limit <= 0 || rule.Window <= 0 {
			continue
		}
		key := rule.key(userID)
		l.windows[key] = append(l.windows[key], now)
	}
	if maxConcurrent > 0 {
		l.inflight[userID]++
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			if l.inflight[userID] > 0 {
				l.inflight[userID]--
			}
			l.mu.Unlock()
		})
	}, true, 0, DenyNone
}

type ReleaseFunc func()

type Rule struct {
	Name   string
	Limit  int
	Window time.Duration
}

func (r Rule) key(userID int64) string {
	return r.Name + ":" + stringID(userID)
}

func (l *Limiter) pruneLocked(now time.Time) {
	cutoff := now.Add(-25 * time.Hour)
	for key, values := range l.windows {
		values = trimWindow(values, cutoff)
		if len(values) == 0 {
			delete(l.windows, key)
		} else {
			l.windows[key] = values
		}
	}
	l.lastPruned = now
}

func trimWindow(values []time.Time, cutoff time.Time) []time.Time {
	idx := 0
	for idx < len(values) && values[idx].Before(cutoff) {
		idx++
	}
	if idx == 0 {
		return values
	}
	return append([]time.Time(nil), values[idx:]...)
}

func stringID(id int64) string {
	if id == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	n := id
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
