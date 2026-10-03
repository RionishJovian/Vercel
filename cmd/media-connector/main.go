package main

import (
	"context"
	"flag"
	"fmt"
	"golang.org/x/term"
	"mega-magic/internal/media/connector"
	"mega-magic/internal/media/jellyfin"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func secret(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	b, e := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if e != nil {
		return "", fmt.Errorf("secure terminal entry unavailable; use a normal terminal")
	}
	value := strings.TrimSpace(string(b))
	for i := range b {
		b[i] = 0
	}
	return value, nil
}
func run() error {
	cloud := flag.String("cloud", "https://mikromagic.com", "cloud origin")
	local := flag.String("jellyfin", "http://127.0.0.1:8096", "local Jellyfin address")
	writes := flag.Bool("allow-library-changes", false, "permit confirmed library additions and scans (no deletes)")
	flag.Parse()
	// Secrets are prompted locally and held in memory; no command-line flags,
	// credential files, browser storage or log output.
	code, e := secret("Paste the webapp pairing code (hidden): ")
	if e != nil {
		return e
	}
	key, e := secret("Enter a Jellyfin API key locally (hidden): ")
	if e != nil {
		return e
	}
	if key == "" {
		return fmt.Errorf("a Jellyfin API key is required; do not enter your account password")
	}
	client, e := jellyfin.NewLocal(*local, key)
	if e != nil {
		return e
	}
	defer client.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	// Prove local admin API access before consuming the single-use pairing code.
	if _, e = client.Libraries(ctx); e != nil {
		return e
	}
	runner, e := connector.New(*cloud, client, *writes)
	if e != nil {
		return e
	}
	defer runner.Close()
	if e = runner.Pair(ctx, code); e != nil {
		return e
	}
	fmt.Println("Magic Media connected for this session. Keep this terminal open. Ctrl-C disconnects; pair again after restarting.")
	// Independent bounded workers keep long guest streams from blocking owner
	// inventory jobs. Transient guest failures do not stop the paired session.
	for worker := 0; worker < 4; worker++ {
		go func() {
			for ctx.Err() == nil {
				_ = runner.RelayStep(ctx)
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
			}
		}()
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if e = runner.Step(ctx); e != nil {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("connector stopped: %v; check the dashboard before retrying any library change", e)
			}
		}
	}
}
