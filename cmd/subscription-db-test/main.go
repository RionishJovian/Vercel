// Runs subscription tests only in cryptographically named disposable databases.
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
	path := flag.String("config", "config.yaml", "protected test connection configuration")
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
	cmd := exec.Command("go", "test", "./internal/database", "./internal/server", "-run", "^TestDB_(SubscriptionTransitionTrials|SubscriptionBusinessRenewal|SubscriptionAuthenticatedHTTP)$", "-count=1", "-v")
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "MM_TEST_DB_URL=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "MM_TEST_DB_URL="+u.String())
	out, err := cmd.CombinedOutput()
	report := string(out)
	for _, secret := range []string{u.String(), db.Password, url.QueryEscape(db.Password)} {
		if secret != "" {
			report = strings.ReplaceAll(report, secret, "[redacted]")
		}
	}
	fmt.Print(report)
	if err != nil || strings.Contains(report, "--- SKIP") {
		os.Exit(1)
	}
	for _, name := range []string{"SubscriptionTransitionTrials", "SubscriptionBusinessRenewal", "SubscriptionAuthenticatedHTTP"} {
		if !strings.Contains(report, "--- PASS: TestDB_"+name) {
			os.Exit(1)
		}
	}
}
