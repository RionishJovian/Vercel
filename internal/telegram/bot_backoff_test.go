package telegram

import (
	"testing"
	"time"
)

func TestPollBackoffGrowsAndStopsAtThirtySeconds(t *testing.T) {
	want := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second}
	for i, w := range want {
		if got := pollBackoff(i + 1); got != w {
			t.Errorf("failure %d: wait %v, want %v", i+1, got, w)
		}
	}
	if pollBackoff(0) != 500*time.Millisecond {
		t.Error("zero failures still waits the first step")
	}
}
