// Command voucher-code-protect moves existing voucher codes to the protected form (sealed copy plus blind index).
//
// It is a dry run unless -apply is passed, never prints a code or a key, and reads the key from the same
// configuration the API uses (security.voucher_code_key).
//
//	voucher-code-protect                          show how many vouchers are in each form
//	voucher-code-protect -apply                   give every plaintext voucher a verified sealed copy (plaintext kept)
//	voucher-code-protect -apply -clear-plaintext  then remove the plaintext, row by row, after re-verifying each one
//
// Run -clear-plaintext only after every API instance runs this version with the key: an older instance cannot read a
// voucher whose plaintext is gone. Back up the key first; see docs/VOUCHER-CODE-PROTECTION.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/vouchercode"
)

func main() {
	configPath := flag.String("config", "config.yaml", "configuration file (the same one the API uses)")
	apply := flag.Bool("apply", false, "write the sealed copies; without it nothing changes")
	clear := flag.Bool("clear-plaintext", false, "with -apply: remove plaintext codes that have a verified sealed copy")
	batch := flag.Int("batch", 500, "rows per transaction")
	timeout := flag.Duration("timeout", 10*time.Minute, "overall time limit")
	flag.Parse()
	if err := run(*configPath, *apply, *clear, *batch, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, "voucher-code-protect:", err)
		os.Exit(1)
	}
}

func run(configPath string, apply, clear bool, batch int, timeout time.Duration) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	key := strings.TrimSpace(cfg.Security.VoucherCodeKey)
	if key == "" {
		return fmt.Errorf("security.voucher_code_key is not set in %s", configPath)
	}
	protector, err := vouchercode.New(key)
	if err != nil {
		return fmt.Errorf("security.voucher_code_key: %w", err)
	}
	db, err := database.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	r, err := db.ProtectVoucherCodes(ctx, protector, apply, clear, batch)
	if err != nil {
		return err
	}
	mode := "DRY RUN (nothing changed)"
	if r.Applied {
		mode = "APPLIED"
	}
	fmt.Printf("%s\n  vouchers visible to this connection:  %d\n  plaintext only (legacy):               %d\n  sealed, plaintext still present:       %d\n  protected only (plaintext cleared):    %d\n  sealed by this run:                    %d\n  plaintext cleared by this run:         %d\n  left untouched (sealed copy mismatch): %d\n",
		mode, r.Visible, r.PlaintextOnly, r.Both, r.ProtectedOnly, r.Sealed, r.Cleared, r.Mismatched)
	if r.Visible == 0 {
		fmt.Println("Zero vouchers are visible. If the database has vouchers, this connection is row-restricted; run it with the service role used by rotate-credential-key.")
	}
	if r.Mismatched > 0 {
		return fmt.Errorf("%d vouchers have a sealed copy that does not match their plaintext; they were left unchanged, investigate before retrying", r.Mismatched)
	}
	return nil
}
