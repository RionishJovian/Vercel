package routeros

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// limitedRunner gives each command the app's limit, so a command RouterOS holds fails here the way it does in production.
type limitedRunner struct {
	r     CommandRunner
	limit time.Duration
}

func (t limitedRunner) Run(ctx context.Context, cmd string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, t.limit)
	defer cancel()
	return t.r.Run(ctx, cmd)
}

// RouterOS 7.24.6 finished a 31-file folder copy and then held the command for 60 seconds, so reinstalling the guest page failed.
// Needs a CHR whose profile mm-labnet has been installed on several times. It installs twice more on that profile.
func TestCHRLabPortalReinstallOnAGrownFolder(t *testing.T) {
	target := os.Getenv("MM_CHR_SSH")
	if target == "" {
		t.Skip("set MM_CHR_SSH to run against a disposable CHR")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	r := chrRunner{target}
	l := chrLab{t, r, ctx}
	a := NewDeviceApplier(capabilitySource{}, 10*time.Second, func(SSHConfig) CommandRunner { return limitedRunner{r, 10 * time.Second} }, DefaultInterface, 0, 0)
	const biz, dev = "biz-1", "dev-1"
	files := func(folder string) int {
		n, err := strconv.Atoi(l.put(`/file print count-only where name~"^` + folder + `/"`))
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	active := func() string { return l.put(`:put [/ip hotspot profile get [find name=mm-labnet] html-directory]`) }
	for round := 1; round <= 2; round++ {
		before := active()
		sourceFiles := files(before)
		req := PortalDeployment{Profile: "mm-labnet", Version: fmt.Sprintf("%016x", time.Now().UnixNano()), Design: &PortalLoginDesign{BusinessName: "Lab", WelcomeText: "Welcome", PrimaryHex: "#146b52", GlassTintHex: "#146b52"}}
		plan, err := a.PreviewPortal(ctx, biz, dev, req)
		if err != nil {
			t.Fatalf("round %d preview: %v", round, err)
		}
		req.Version, req.Confirmation = plan.Version, plan.Confirmation
		start := time.Now()
		res, err := a.ApplyPortal(ctx, biz, dev, req)
		if err != nil || !res.Verified {
			t.Fatalf("round %d apply from %s (%d files): verified=%v %v", round, before, sourceFiles, res.Verified, err)
		}
		now := active()
		if now == before || !strings.HasPrefix(plan.File, now+"/") {
			t.Fatalf("round %d: profile on %q, plan %q", round, now, plan.File)
		}
		backups := l.put(`/file print count-only where name~"^` + now + `/login-backup-"`)
		if backups != "1" {
			t.Errorf("round %d: %s backups in the new folder, want this install's only", round, backups)
		}
		if got := files(before); got != sourceFiles {
			t.Errorf("round %d: the old folder went from %d to %d files", round, sourceFiles, got)
		}
		t.Logf("round %d: %s (%d files) -> %s (%d files) in %s", round, before, sourceFiles, now, files(now), time.Since(start).Round(time.Second))
	}
}
