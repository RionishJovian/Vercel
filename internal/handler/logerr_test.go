package handler

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"mega-magic/internal/provisioner/routeros"
)

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func TestLogIfErrOnlyWritesWhenThereIsAnError(t *testing.T) {
	buf := captureLog(t)
	logIfErr(nil, "nothing to say %d", 1)
	if buf.Len() != 0 {
		t.Fatalf("a nil error must not log, got %q", buf.String())
	}
	logIfErr(errors.New("boom"), "router change %q on %s not recorded", "restore", "dev-1")
	if got := buf.String(); !strings.Contains(got, `router change "restore" on dev-1 not recorded: boom`) {
		t.Fatalf("log line missing what and why: %q", got)
	}
}

type flakyRouter struct {
	failFirst, calls int
}

func (f *flakyRouter) ConfigureGuestGrace(context.Context, string, string, routeros.GuestGrace) error {
	return nil
}
func (f *flakyRouter) DecideGuestAccess(context.Context, string, string, string, string, bool, int, int, int, int) error {
	f.calls++
	if f.calls <= f.failFirst {
		return errors.New("router unreachable")
	}
	return nil
}

func TestUndoGuestGraceRetriesThenSaysSoInTheLog(t *testing.T) {
	old := graceUndoPause
	graceUndoPause = time.Millisecond
	t.Cleanup(func() { graceUndoPause = old })

	buf := captureLog(t)
	r := &flakyRouter{failFirst: 2}
	h := &GuestWifiHandler{graceRouter: r}
	h.undoGuestGrace("biz", "dev", "req-1", "AA:BB:CC:DD:EE:01")
	if r.calls != 3 || buf.Len() != 0 {
		t.Fatalf("recovers on the third try without a log line: calls=%d log=%q", r.calls, buf.String())
	}

	r = &flakyRouter{failFirst: 99}
	h.graceRouter = r
	h.undoGuestGrace("biz", "dev", "req-2", "AA:BB:CC:DD:EE:02")
	if r.calls != 3 || !strings.Contains(buf.String(), "req-2") || !strings.Contains(buf.String(), "remove it on the router by hand") {
		t.Fatalf("gives up after three tries and names the request: calls=%d log=%q", r.calls, buf.String())
	}
}
