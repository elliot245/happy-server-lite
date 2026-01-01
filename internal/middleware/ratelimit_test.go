package middleware

import (
	"testing"
	"time"
)

func TestRateLimiter_AllowAndDeny(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := now
	rl := NewRateLimiterWithNow(2, time.Minute, func() time.Time { return clock })

	if !rl.Allow("ip") {
		t.Fatalf("expected allow")
	}
	if !rl.Allow("ip") {
		t.Fatalf("expected allow")
	}
	if rl.Allow("ip") {
		t.Fatalf("expected deny")
	}

	clock = clock.Add(time.Minute + time.Second)
	if !rl.Allow("ip") {
		t.Fatalf("expected allow after window")
	}
}
