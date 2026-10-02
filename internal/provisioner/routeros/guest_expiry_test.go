package routeros

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestPaidGuestOneMinuteDeadline(t *testing.T) {
	for _, date := range []string{"2026-10-31", "oct/31/2026"} {
		count := "0"
		var adds int
		r := commandRunnerFunc(func(_ context.Context, cmd string) (string, error) {
			switch {
			case strings.Contains(cmd, "print count-only"):
				return count, nil
			case cmd == "/system clock print":
				return "date: " + date + "\ntime: 23:59:30", nil
			case strings.Contains(cmd, "scheduler add"):
				adds++
				want := "start-date=2026-11-01"
				if strings.Contains(date, "/") {
					want = "start-date=nov/01/2026"
				}
				if !strings.Contains(cmd, want+" start-time=00:00:30") || !strings.Contains(cmd, "hotspot user disable") || !strings.Contains(cmd, "hotspot active remove") || !strings.Contains(cmd, "mega-magic:paid:") {
					t.Fatalf("invalid scoped expiry: %s", cmd)
				}
				count = "1"
				return "", nil
			default:
				return "", fmt.Errorf("unexpected command")
			}
		})
		for i := 0; i < 2; i++ {
			if err := schedulePaidGuestExpiry(context.Background(), r, graceTestID, routerOSQuoted("AA:BB:CC:DD:EE:FE"), "mega-magic:paid:"+graceTestID, 1); err != nil {
				t.Fatal(err)
			}
		}
		if adds != 1 {
			t.Fatal("retry renewed validity")
		}
	}
}
