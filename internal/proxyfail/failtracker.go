// Package proxyfail tracks proxies that repeatedly fail at runtime and keeps
// them out of proxy selection for a cooldown period.
//
// TODO: move FailTracker and BanChecker into github.com/azzimoda/go-tg-proxy
// (proxy.Service), exposing Ban/Unban there. Until then the ban is implemented
// locally by wrapping the pool checker through the public WithChecker option.
package proxyfail

import (
	"sync"
	"time"
)

// FailTracker records proxies that repeatedly failed during sends and remembers
// which one the bot currently runs on (set by the bot builder). Bans expire
// lazily: a proxy banned now is considered banned until now+cooldown.
type FailTracker struct {
	mu       sync.Mutex
	active   string
	banned   map[string]time.Time
	cooldown time.Duration
}

// NewFailTracker returns a FailTracker whose bans last for cooldown.
func NewFailTracker(cooldown time.Duration) *FailTracker {
	if cooldown <= 0 {
		cooldown = 5 * time.Minute
	}
	return &FailTracker{
		banned:   make(map[string]time.Time),
		cooldown: cooldown,
	}
}

// SetActive remembers the proxy the bot is currently built with.
func (f *FailTracker) SetActive(proxy string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active = proxy
}

// Active returns the proxy the bot is currently built with.
func (f *FailTracker) Active() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active
}

// Ban marks proxy as failed for the cooldown period and returns whether it was
// not already banned.
func (f *FailTracker) Ban(proxy string) bool {
	if proxy == "" {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if until, ok := f.banned[proxy]; ok && time.Now().Before(until) {
		return false
	}
	f.banned[proxy] = time.Now().Add(f.cooldown)
	return true
}

// IsBanned reports whether proxy is currently banned.
func (f *FailTracker) IsBanned(proxy string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	until, ok := f.banned[proxy]
	if !ok {
		return false
	}
	if time.Now().Before(until) {
		return true
	}
	delete(f.banned, proxy)
	return false
}

// Reset removes a ban and forgets the active proxy.
func (f *FailTracker) Reset(proxy string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.active == proxy {
		f.active = ""
	}
	delete(f.banned, proxy)
}
