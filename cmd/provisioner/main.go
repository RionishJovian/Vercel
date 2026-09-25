// Command provisioner implements the dry-run half of the router recovery
// provisioner (DESIGN-provisioner.md section 1).
//
// Usage:
//
//	go run ./cmd/provisioner --dry-run --peer <uuid> [--config <path>] [--peer-addr host:port]
//
// It validates the provisioning config file, checks connectivity to the peer,
// and reports the recovery changes that WOULD be applied. It never writes to
// production: live apply (sections 2-4) is not implemented here and the binary
// refuses to run without --dry-run.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"mega-magic/internal/provisioner"
)

func main() {
	var (
		dryRun   = flag.Bool("dry-run", false, "validate + probe + simulate only; never writes to production (required)")
		peer     = flag.String("peer", "", "peer identifier (uuid) to inspect (required)")
		cfgFile  = flag.String("config", "provisioning/MagicHub_Provisioning_v1.1.rsc", "provisioning config artifact to validate")
		peerAddr = flag.String("peer-addr", "", "management endpoint host[:port] for the connectivity check")
		timeout  = flag.Duration("timeout", 5*time.Second, "connectivity probe timeout")
		asJSON   = flag.Bool("json", false, "emit the report as JSON")
	)
	flag.Parse()

	if err := run(*dryRun, *peer, *cfgFile, *peerAddr, *timeout, *asJSON); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(dryRun bool, peer, cfgFile, peerAddr string, timeout time.Duration, asJSON bool) error {
	if !dryRun {
		return fmt.Errorf("live apply is not implemented; refusing to run without --dry-run")
	}
	if peer == "" {
		return fmt.Errorf("--peer is required")
	}

	// Choose a read-only prober. When no endpoint can be resolved for the peer
	// id we report connectivity as unchecked rather than inventing a result.
	var probe provisioner.Prober
	if endpoint, ok := provisioner.ResolveEndpoint(peer, peerAddr); ok {
		probe = provisioner.TCPProber{Endpoint: endpoint, Timeout: timeout}
	} else {
		probe = provisioner.StaticProber{
			Obs:  provisioner.Observation{State: provisioner.StateUnknown},
			Conn: provisioner.UncheckedConnectivity(peer),
		}
	}

	p := provisioner.New(probe)
	report, err := p.DryRunAction(context.Background(), peer, cfgFile)
	if err != nil {
		return err
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	printReport(report)
	return nil
}

func printReport(r *provisioner.Report) {
	fmt.Printf("Provisioner dry-run — peer %s\n", r.PeerID)
	fmt.Printf("  config:       %s (%s)\n", r.ConfigPath, validLabel(r.ConfigValid))
	fmt.Printf("  connectivity: %s\n", connectivityLabel(r.Connectivity))
	fmt.Println("  proposed changes (simulation only; nothing is applied):")
	if len(r.Changes) == 0 {
		fmt.Println("    (none)")
	}
	for i, c := range r.Changes {
		fmt.Printf("    %d. %-20s target=%s\n", i+1, c.Action, c.Target)
		fmt.Printf("       %s -> %s (%s, risk=%s)\n", c.Current, c.Proposed, c.Reason, c.Risk)
	}
	if len(r.Issues) > 0 {
		fmt.Println("  config issues:")
		for _, is := range r.Issues {
			fmt.Printf("    [%s] %s: %s\n", is.Severity, is.Code, is.Detail)
		}
	}
	fmt.Printf("  summary: %s\n", r.Summary)
	fmt.Println("  dry-run: no production writes performed")
}

func validLabel(ok bool) string {
	if ok {
		return "valid"
	}
	return "INVALID"
}

func connectivityLabel(c provisioner.Connectivity) string {
	switch {
	case !c.Checked:
		return "not checked (" + c.Detail + ")"
	case c.Reachable:
		return "reachable via " + c.Endpoint
	default:
		return "unreachable via " + c.Endpoint + " (" + c.Detail + ")"
	}
}
