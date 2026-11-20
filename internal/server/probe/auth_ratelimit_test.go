package probe

import (
	"testing"
	"time"
)

func TestAuthFailureLimiterBlocksAfterThreshold(t *testing.T) {
	var limiter authFailureLimiter
	now := time.Unix(1_700_000_000, 0)
	for attempt := 1; attempt < authFailureMaximum; attempt++ {
		if limiter.fail("203.0.113.7", now) {
			t.Fatalf("attempt %d blocked before threshold", attempt)
		}
	}
	if !limiter.fail("203.0.113.7", now) {
		t.Fatal("threshold attempt did not block the host")
	}
	if !limiter.blocked("203.0.113.7", now) {
		t.Fatal("blocked host is not refused")
	}
	if limiter.blocked("203.0.113.8", now) {
		t.Fatal("a different host was blocked")
	}
	if limiter.blocked("203.0.113.7", now.Add(authBlockDuration)) {
		t.Fatal("block did not expire")
	}
}

func TestAuthFailureLimiterWindowExpires(t *testing.T) {
	var limiter authFailureLimiter
	now := time.Unix(1_700_000_000, 0)
	for attempt := 0; attempt < authFailureMaximum-1; attempt++ {
		limiter.fail("198.51.100.9", now)
	}
	later := now.Add(authFailureWindow)
	if limiter.blocked("198.51.100.9", later) {
		t.Fatal("stale failures blocked the host")
	}
	if limiter.fail("198.51.100.9", later) {
		t.Fatal("failure window was not restarted")
	}
}

func TestAuthFailureLimiterSuccessClearsFailures(t *testing.T) {
	var limiter authFailureLimiter
	now := time.Unix(1_700_000_000, 0)
	for attempt := 0; attempt < authFailureMaximum-1; attempt++ {
		limiter.fail("192.0.2.44", now)
	}
	limiter.reset("192.0.2.44")
	for attempt := 0; attempt < authFailureMaximum-1; attempt++ {
		if limiter.fail("192.0.2.44", now) {
			t.Fatal("failures were not cleared after a successful login")
		}
	}
}

func TestAuthFailureLimiterPruneDropsStaleEntries(t *testing.T) {
	var limiter authFailureLimiter
	now := time.Unix(1_700_000_000, 0)
	limiter.fail("192.0.2.10", now)
	limiter.prune(now.Add(authFailureWindow))
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if len(limiter.entries) != 0 {
		t.Fatalf("stale entries retained: %d", len(limiter.entries))
	}
}
