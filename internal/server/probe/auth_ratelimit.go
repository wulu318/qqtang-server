package probe

import (
	"sync"
	"time"
)

// Authentication is a challenge-response exchange, so a brute-force attempt is
// cheap for the client and expensive for the server (PBKDF2 per proof). The
// limiter bounds both: a host that keeps failing proofs is refused before any
// password parameters are derived or any nonce is issued.
const (
	authFailureWindow  = 5 * time.Minute
	authFailureMaximum = 10
	authBlockDuration  = 10 * time.Minute
)

type authFailureEntry struct {
	WindowStart  time.Time
	Failures     int
	BlockedUntil time.Time
}

// authFailureLimiter is keyed by the observed remote host (not by UIN) so an
// attacker cannot lock a specific account by guessing its UIN.
type authFailureLimiter struct {
	mu      sync.Mutex
	entries map[string]authFailureEntry
}

func (limiter *authFailureLimiter) blocked(host string, now time.Time) bool {
	if host == "" {
		return false
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	entry, found := limiter.entries[host]
	if !found {
		return false
	}
	if entry.BlockedUntil.IsZero() {
		return false
	}
	if now.Before(entry.BlockedUntil) {
		return true
	}
	limiter.deleteLocked(host)
	return false
}

// fail records one rejected proof and reports whether the host is now blocked.
func (limiter *authFailureLimiter) fail(host string, now time.Time) bool {
	if host == "" {
		return false
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if limiter.entries == nil {
		limiter.entries = make(map[string]authFailureEntry)
	}
	entry := limiter.entries[host]
	if entry.WindowStart.IsZero() || now.Sub(entry.WindowStart) >= authFailureWindow {
		entry = authFailureEntry{WindowStart: now}
	}
	entry.Failures++
	if entry.Failures >= authFailureMaximum {
		entry.BlockedUntil = now.Add(authBlockDuration)
	}
	limiter.entries[host] = entry
	if len(limiter.entries) > 4096 {
		limiter.pruneLocked(now)
	}
	return entry.Failures >= authFailureMaximum
}

func (limiter *authFailureLimiter) reset(host string) {
	if host == "" {
		return
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	limiter.deleteLocked(host)
}

// prune drops expired windows so a long-lived server does not retain one entry
// per address that ever mistyped a password.
func (limiter *authFailureLimiter) prune(now time.Time) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	limiter.pruneLocked(now)
}

func (limiter *authFailureLimiter) pruneLocked(now time.Time) {
	for host, entry := range limiter.entries {
		expired := !entry.BlockedUntil.IsZero() && !now.Before(entry.BlockedUntil)
		stale := entry.BlockedUntil.IsZero() && now.Sub(entry.WindowStart) >= authFailureWindow
		if expired || stale {
			delete(limiter.entries, host)
		}
	}
}

func (limiter *authFailureLimiter) deleteLocked(host string) {
	if limiter.entries == nil {
		return
	}
	delete(limiter.entries, host)
}
