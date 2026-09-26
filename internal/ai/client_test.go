package ai

import (
	"context"
	"testing"

	"mega-magic/internal/model"
)

func f64(v float64) *float64 { return &v }
func i(v int) *int           { return &v }

func TestRuleFallback_latencySpike(t *testing.T) {
	c := NewRuleFallbackClient()
	f, err := c.Analyze(context.Background(), model.TelemetrySample{LatencyMs: i(401)})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if f == nil || f.Rule != "latency_spike" || f.Severity != "critical" {
		t.Fatalf("latency>400 => critical latency_spike, got %+v", f)
	}
}

func TestRuleFallback_cpuHigh(t *testing.T) {
	c := NewRuleFallbackClient()
	f, err := c.Analyze(context.Background(), model.TelemetrySample{CPUPercent: f64(86)})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if f == nil || f.Rule != "cpu_high" || f.Severity != "warn" {
		t.Fatalf("cpu>85 => warn cpu_high, got %+v", f)
	}
}

func TestRuleFallback_healthy(t *testing.T) {
	c := NewRuleFallbackClient()
	f, err := c.Analyze(context.Background(), model.TelemetrySample{CPUPercent: f64(10), LatencyMs: i(20)})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if f != nil {
		t.Fatalf("healthy sample => nil finding, got %+v", f)
	}
}

func TestRuleFallback_belowThresholds(t *testing.T) {
	c := NewRuleFallbackClient()
	// latency 250 is below the 400 threshold -> healthy.
	f, err := c.Analyze(context.Background(), model.TelemetrySample{LatencyMs: i(250), CPUPercent: f64(50)})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if f != nil {
		t.Fatalf("latency 250 should be healthy, got %+v", f)
	}
}

func TestRuleFallback_nilFields(t *testing.T) {
	c := NewRuleFallbackClient()
	f, err := c.Analyze(context.Background(), model.TelemetrySample{})
	if err != nil || f != nil {
		t.Fatalf("nil fields => healthy, got %+v / %v", f, err)
	}
}
