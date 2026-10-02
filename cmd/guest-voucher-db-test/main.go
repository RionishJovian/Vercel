// Runs only the disposable guest-voucher harness, with credentials passed to the
// child environment rather than printed or written to a shell command.
package main

import (
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"mega-magic/internal/config"
)

func main() {
	path := flag.String("config", "config.yaml", "configuration read for the disposable DB harness")
	flag.Parse()
	cfg, err := config.Load(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "test configuration unavailable")
		os.Exit(1)
	}
	db := cfg.Database
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(db.Host, strconv.Itoa(db.Port)), User: url.UserPassword(db.User, db.Password), Path: "/" + db.Name}
	q := u.Query()
	q.Set("sslmode", db.SSLMode)
	u.RawQuery = q.Encode()
	cmd := exec.Command("go", "test", "./internal/database", "./internal/server", "-run", "^TestDB_(GuestVoucherDurableClaim|GuestAccessRecovery|GuestRecoveryAmbiguousCompletion|GuestRecoveryRestartFence|GuestProvisionOrchestration|GuestVoucherSchemaReadiness|SubscriptionTransitionTrials|SubscriptionBusinessRenewal|SubscriptionAuthenticatedHTTP)$", "-count=1", "-v")
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "MM_TEST_DB_URL=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "MM_TEST_DB_URL="+u.String())
	out, err := cmd.CombinedOutput()
	text := string(out)
	for _, secret := range []string{u.String(), db.Password, url.QueryEscape(db.Password)} {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
	}
	fmt.Print(text)
	if !strings.Contains(text, "--- PASS: TestDB_GuestRecoveryRestartFence") {
		os.Exit(1)
	}
	if err != nil || strings.Contains(text, "--- SKIP") || !strings.Contains(text, "--- PASS: TestDB_GuestVoucherDurableClaim") || !strings.Contains(text, "--- PASS: TestDB_GuestAccessRecovery") || !strings.Contains(text, "--- PASS: TestDB_GuestRecoveryAmbiguousCompletion") || !strings.Contains(text, "--- PASS: TestDB_GuestProvisionOrchestration") || !strings.Contains(text, "--- PASS: TestDB_GuestVoucherSchemaReadiness") || !strings.Contains(text, "--- PASS: TestDB_SubscriptionTransitionTrials") || !strings.Contains(text, "--- PASS: TestDB_SubscriptionBusinessRenewal") || !strings.Contains(text, "--- PASS: TestDB_SubscriptionAuthenticatedHTTP") {
		os.Exit(1)
	}
}
