package monitoring

import (
	"context"
	"errors"
	"testing"
)

type stubVersionObserver struct {
	version string
	channel string
	err     error
}

func (s stubVersionObserver) ObserveVersion(context.Context, string, string) (string, string, error) {
	return s.version, s.channel, s.err
}

func TestCaptureVersionReportsPinnedProbeReachabilityWithoutPostureStore(t *testing.T) {
	svc := &MonitoringService{versionObserver: stubVersionObserver{version: "7.15.3", channel: "stable"}}
	if !svc.captureVersion(context.Background(), "business", "device") {
		t.Fatal("successful version read must be usable as management-plane reachability")
	}
}

func TestCaptureVersionDoesNotInventReachability(t *testing.T) {
	tests := []stubVersionObserver{
		{err: errors.New("dial failed")},
		{version: "", channel: "stable"},
	}
	for _, observer := range tests {
		svc := &MonitoringService{versionObserver: observer}
		if svc.captureVersion(context.Background(), "business", "device") {
			t.Fatal("failed or empty version read must not report reachability")
		}
	}
}
