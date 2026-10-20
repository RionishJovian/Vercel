package routeros

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestQuickConfigRecoveryAfterCancellation(t *testing.T) {
	type contextKey struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "tenant-context"))
	defer cancel()
	state := &quickConfigRunner{identity: "old-router", dns: "1.1.1.1"}
	interrupted, recoveryCommands := false, 0
	runner := commandRunnerFunc(func(runCtx context.Context, command string) (string, error) {
		if err := runCtx.Err(); err != nil {
			return "", err
		}
		if interrupted {
			recoveryCommands++
			deadline, ok := runCtx.Deadline()
			if !ok || time.Until(deadline) > 30*time.Second {
				t.Fatal("recovery must have a bounded deadline")
			}
			if runCtx.Value(contextKey{}) != "tenant-context" {
				t.Fatal("recovery lost request context values")
			}
		}
		if !interrupted && strings.HasPrefix(command, "/ip dns set ") {
			interrupted = true
			cancel()
			return "", context.Canceled
		}
		return state.Run(runCtx, command)
	})
	result, err := quickConfigApplier(runner).ApplyQuickConfig(ctx, "biz-1", "dev-1", QuickConfig{
		Identity: "new-router", DNSServers: []string{"9.9.9.9"},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("original cancellation must remain detectable: %v", err)
	}
	if !result.RolledBack || result.Verified || state.identity != "old-router" || state.dns != "1.1.1.1" || recoveryCommands < 4 {
		t.Fatalf("cancelled request did not restore and read back settings: result=%+v state=%+v recovery commands=%d", result, state, recoveryCommands)
	}
}

func TestQuickConfigRecoveryRequiresReadback(t *testing.T) {
	for _, mode := range []string{"write-failure", "read-failure", "mismatch"} {
		t.Run(mode, func(t *testing.T) {
			state := &quickConfigRunner{identity: "old-router", dns: "1.1.1.1"}
			writeErr := errors.New("original DNS write failure")
			recovering := false
			runner := commandRunnerFunc(func(ctx context.Context, command string) (string, error) {
				if !recovering && strings.HasPrefix(command, "/ip dns set ") {
					recovering = true
					return "", writeErr
				}
				if recovering {
					switch {
					case mode == "write-failure" && strings.HasPrefix(command, "/system identity set "):
						return "", errors.New("recovery connection lost")
					case mode == "read-failure" && command == "/system identity print":
						return "", errors.New("recovery read failed")
					case mode == "mismatch" && strings.HasPrefix(command, "/system identity set "):
						return "", nil // The command succeeds but the old identity is not restored.
					}
				}
				return state.Run(ctx, command)
			})
			result, err := quickConfigApplier(runner).ApplyQuickConfig(context.Background(), "biz-1", "dev-1", QuickConfig{
				Identity: "new-router", DNSServers: []string{"9.9.9.9"},
			})
			if !errors.Is(err, writeErr) || !strings.Contains(err.Error(), "rollback") {
				t.Fatalf("must retain original failure and report rollback failure: %v", err)
			}
			if result.RolledBack || result.Verified || strings.Contains(err.Error(), "previous settings restored") {
				t.Fatalf("unverified recovery must not claim restoration: result=%+v err=%v", result, err)
			}
		})
	}
}

func TestQuickConfigVerificationMismatchReportsRecovery(t *testing.T) {
	for _, restore := range []bool{true, false} {
		t.Run(map[bool]string{true: "restored", false: "not-restored"}[restore], func(t *testing.T) {
			state := &quickConfigRunner{identity: "old-router", dns: "1.1.1.1"}
			identityWrites := 0
			runner := commandRunnerFunc(func(ctx context.Context, command string) (string, error) {
				if strings.HasPrefix(command, "/system identity set ") {
					identityWrites++
					if identityWrites == 1 || !restore {
						return "", nil
					}
				}
				if !restore && identityWrites > 1 && strings.HasPrefix(command, "/ip dns set ") {
					return "", errors.New("rollback write failed")
				}
				return state.Run(ctx, command)
			})
			result, err := quickConfigApplier(runner).ApplyQuickConfig(context.Background(), "biz-1", "dev-1", QuickConfig{
				Identity: "new-router", DNSServers: []string{"9.9.9.9"},
			})
			if err == nil || !strings.Contains(err.Error(), "verification mismatch") || result.Verified || result.RolledBack != restore {
				t.Fatalf("incorrect verification outcome: result=%+v err=%v", result, err)
			}
			if strings.Contains(err.Error(), "previous settings restored") != restore {
				t.Fatalf("restoration message must match readback: %v", err)
			}
		})
	}
}
