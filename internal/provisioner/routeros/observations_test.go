package routeros

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestObservationsFixedReadOnlyMenusAndPartialFailure(t *testing.T) {
	calls := 0
	r := commandRunnerFunc(func(_ context.Context, cmd string) (string, error) {
		calls++
		for _, bad := range []string{" add ", " set ", " remove ", " disable ", " enable ", "password", "secret", "reset-counters"} {
			if strings.Contains(cmd, bad) {
				t.Fatalf("unsafe observation: %s", cmd)
			}
		}
		if strings.Contains(cmd, "/ip arp") {
			return "id: *A\naddress: 10.0.0.2\nmac-address: 02:00:00:00:00:01\ninterface: bridge\n", nil
		}
		return "", fmt.Errorf("source unavailable")
	})
	result, e := readObservations(context.Background(), r)
	if e != nil || calls != len(observationOrder)+1 || !result.Available["arp"] || result.Available["hotspot_sessions"] || len(result.Objects["arp"]) != 1 || result.WriteEnabled || result.ObservedAt == "" {
		t.Fatalf("%+v %v", result, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls = 0
	if _, e = readObservations(ctx, r); e == nil || calls != 0 {
		t.Fatal("cancellation not respected")
	}
}
