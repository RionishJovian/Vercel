package routeros

import (
	"context"
	"errors"
	"testing"
)

type singleTargetSource struct {
	target *DeviceTarget
	err    error
}

func (s singleTargetSource) TargetForDevice(context.Context, string, string) (*DeviceTarget, error) {
	return s.target, s.err
}

func TestCIDRTargetSourceAllowsOnlyExplicitNetworks(t *testing.T) {
	valid := pwTarget("dev", "10.77.0.4:22", "admin", "secret")
	policy, err := NewCIDRTargetSource(singleTargetSource{target: valid}, []string{"10.77.0.0/24", "fd42::/64"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := policy.TargetForDevice(context.Background(), "biz", "dev")
	if err != nil {
		t.Fatalf("allowed target rejected: %v", err)
	}
	if got != valid {
		t.Fatal("policy did not preserve the resolved target")
	}

	policy.next = singleTargetSource{target: pwTarget("dev", "10.88.0.4:22", "admin", "secret")}
	if _, err := policy.TargetForDevice(context.Background(), "biz", "dev"); !errors.Is(err, ErrTargetNotAllowed) {
		t.Fatalf("outside target error = %v, want ErrTargetNotAllowed", err)
	}

	policy.next = singleTargetSource{target: pwTarget("dev", "router.internal:22", "admin", "secret")}
	if _, err := policy.TargetForDevice(context.Background(), "biz", "dev"); !errors.Is(err, ErrTargetNotAllowed) {
		t.Fatalf("hostname target error = %v, want ErrTargetNotAllowed", err)
	}
}

func TestCIDRTargetSourceIPv6(t *testing.T) {
	target := pwTarget("dev", "[fd42::4]:22", "admin", "secret")
	policy, err := NewCIDRTargetSource(singleTargetSource{target: target}, []string{"fd42::/64"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.TargetForDevice(context.Background(), "biz", "dev"); err != nil {
		t.Fatalf("allowed IPv6 target rejected: %v", err)
	}
}

func TestCIDRTargetSourceRejectsUnsafeConfiguration(t *testing.T) {
	source := singleTargetSource{target: pwTarget("dev", "10.0.0.1:22", "admin", "secret")}
	for name, cidrs := range map[string][]string{
		"empty":              nil,
		"invalid":            {"not-a-cidr"},
		"ipv4 default route": {"0.0.0.0/0"},
		"ipv6 default route": {"::/0"},
		"noncanonical":       {"10.77.0.4/24"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewCIDRTargetSource(source, cidrs); err == nil {
				t.Fatal("unsafe target policy configuration was accepted")
			}
		})
	}
	if _, err := NewCIDRTargetSource(nil, []string{"10.0.0.0/8"}); err == nil {
		t.Fatal("nil target source was accepted")
	}
}

func TestCIDRTargetSourcePreservesResolverErrors(t *testing.T) {
	policy, err := NewCIDRTargetSource(singleTargetSource{err: ErrNoTarget}, []string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.TargetForDevice(context.Background(), "biz", "dev"); !errors.Is(err, ErrNoTarget) {
		t.Fatalf("resolver error = %v, want ErrNoTarget", err)
	}
}
