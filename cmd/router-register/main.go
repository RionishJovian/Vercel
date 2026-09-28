// Command router-register records a device's SSH management target (address,
// port, username and encrypted secret) so the per-device transport and the
// security-posture capture can actually reach it.
//
// # WHY THIS EXISTS
//
// internal/database.SetDeviceCredentials is the ONLY writer of a device's SSH
// credentials — but until this command, nothing in the shipped code called it.
// The encrypted credential store therefore existed with no way to be populated,
// which meant posture capture could never resolve a single device even once it
// was switched on. This is that missing writer.
//
// The secret is read from a FILE rather than a flag so it cannot leak through
// the process list (ps) or shell history.
//
// USAGE
//
//	# Register an EXISTING device
//	go run ./cmd/router-register \
//	  --business 4a437be9-... --device 141b83ff-... \
//	  --address 118.27.148.183 --port 2222 \
//	  --user admin --password-file /root/routeros-test/admin.pw
//
//	# Create the device first, then register it
//	go run ./cmd/router-register \
//	  --business 4a437be9-... --create \
//	  --name "CHR lab router" --model "CHR 7.15.3" \
//	  --address 118.27.148.183 --port 2222 \
//	  --user admin --password-file /root/routeros-test/admin.pw
//
// The secret is never printed: the command reports only whether a ciphertext is
// present, plus the non-secret fingerprint, and it resolves the stored target
// through the real DeviceCredentialSource and prints its REDACTED form.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/secretbox"
)

func main() {
	var (
		configPath = flag.String("config", "config.yaml", "path to config.yaml (supplies database + credential_key)")
		business   = flag.String("business", "", "business_id (uuid) that owns the device (required)")
		device     = flag.String("device", "", "device_id (uuid) to update; omit with --create")
		create     = flag.Bool("create", false, "create the device row if --device is not given")
		name       = flag.String("name", "", "hardware_name for a created device (required with --create)")
		model      = flag.String("model", "", "optional device_model for a created device")
		layer      = flag.String("layer", "MAIN_GATEWAY", "deployment_layer for a created device")
		address    = flag.String("address", "", "management address the app dials (required)")
		port       = flag.Int("port", 22, "SSH port")
		user       = flag.String("user", "", "SSH username (required)")
		pwFile     = flag.String("password-file", "", "file containing the SSH password (never a flag)")
		keyFile    = flag.String("key-file", "", "file containing an SSH private key instead of a password")
		fingerprnt = flag.String("fingerprint", "", "optional SSH host key fingerprint (only meaningful with --key-file)")
		asJSON     = flag.Bool("json", false, "emit the result as JSON")
		timeout    = flag.Duration("timeout", 15*time.Second, "database operation timeout")
	)
	flag.Parse()

	if err := run(runOpts{
		configPath: *configPath, business: *business, device: *device, create: *create,
		name: *name, model: *model, layer: *layer, address: *address, port: *port,
		user: *user, pwFile: *pwFile, keyFile: *keyFile, fingerprint: *fingerprnt,
		asJSON: *asJSON, timeout: *timeout,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

type runOpts struct {
	configPath, business, device       string
	create                             bool
	name, model, layer                 string
	address                            string
	port                               int
	user, pwFile, keyFile, fingerprint string
	asJSON                             bool
	timeout                            time.Duration
}

func run(o runOpts) error {
	if o.business == "" {
		return errors.New("--business is required")
	}
	if o.address == "" {
		return errors.New("--address is required")
	}
	if o.user == "" {
		return errors.New("--user is required")
	}
	if o.device == "" && !o.create {
		return errors.New("give --device <uuid>, or --create to make a new one")
	}
	if o.create && o.name == "" {
		return errors.New("--name is required with --create")
	}

	// Exactly one secret source, read from a file so it never reaches argv.
	switch {
	case o.pwFile != "" && o.keyFile != "":
		return errors.New("give only one of --password-file / --key-file")
	case o.pwFile == "" && o.keyFile == "":
		return errors.New("one of --password-file / --key-file is required")
	}
	secretFile, kind := o.pwFile, "password"
	if o.keyFile != "" {
		secretFile, kind = o.keyFile, "key"
	}
	raw, err := os.ReadFile(secretFile)
	if err != nil {
		return fmt.Errorf("read secret file: %w", err)
	}
	// Trim only the trailing newline an editor/pipeline adds; an SSH key keeps
	// its internal newlines.
	secret := strings.TrimRight(string(raw), "\r\n")
	if strings.TrimSpace(secret) == "" {
		return fmt.Errorf("secret file %s is empty", secretFile)
	}

	cfg, err := config.Load(o.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	box, err := secretbox.New(cfg.Provisioner.Applier.CredentialKey)
	if err != nil {
		if errors.Is(err, secretbox.ErrNoKey) {
			return errors.New("no credential key configured — set provisioner.applier.credential_key in config.yaml (base64 or hex, 32 bytes) before registering devices")
		}
		return fmt.Errorf("credential key: %w", err)
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()

	deviceID := o.device
	if deviceID == "" {
		// Refuse to create a duplicate. Registering the same router twice would
		// split its identity and posture history across two device rows.
		var existing string
		err := db.QueryRowContext(ctx,
			`SELECT device_id FROM devices WHERE business_id = $1 AND hardware_name = $2 LIMIT 1`,
			o.business, o.name).Scan(&existing)
		switch {
		case err == nil:
			return fmt.Errorf("a device named %q already exists in this business (%s) — pass --device %s to update it instead of --create", o.name, existing, existing)
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("check for an existing device: %w", err)
		}

		// Create the device row. RLS is bypassed for this superuser role, so the
		// business_id predicate below is what scopes it — and there is nothing to
		// scope against but the operator-supplied tenant.
		err = db.QueryRowContext(ctx,
			`INSERT INTO devices (business_id, hardware_name, device_model, deployment_layer, vendor, category, transport)
			 VALUES ($1, $2, NULLIF($3,''), $4, 'mikrotik', 'router', 'direct')
			 RETURNING device_id`,
			o.business, o.name, o.model, o.layer).Scan(&deviceID)
		if err != nil {
			return fmt.Errorf("create device: %w", err)
		}
	}

	src := database.NewDeviceCredentialSource(db, box)
	if err := src.SetDeviceCredentials(ctx, o.business, deviceID, o.address, o.user, kind, secret, o.port, o.fingerprint); err != nil {
		return fmt.Errorf("store credentials: %w", err)
	}

	// Read back WITHOUT selecting the ciphertext — the row's own truth, plus the
	// redacted target the transport would actually dial.
	var (
		hardwareName, host, sshUser, sshKind string
		sshPort                              int
		hasCipher, hasFinger                 bool
	)
	// Column name must match the schema exactly: init.sql and
	// device_credentials.go both use ssh_host_key_fingerprint.
	err = db.QueryRowContext(ctx, `
		SELECT hardware_name,
		       COALESCE(host(management_ip), ''),
		       COALESCE(ssh_username, ''),
		       COALESCE(ssh_secret_kind, ''),
		       COALESCE(ssh_port, 0),
		       (ssh_secret_ciphertext IS NOT NULL),
		       (ssh_host_key_fingerprint IS NOT NULL)
		  FROM devices
		 WHERE business_id = $1 AND device_id = $2`,
		o.business, deviceID,
	).Scan(&hardwareName, &host, &sshUser, &sshKind, &sshPort, &hasCipher, &hasFinger)
	if err != nil {
		return fmt.Errorf("read back device: %w", err)
	}

	target, err := src.TargetForDevice(ctx, o.business, deviceID)
	if err != nil {
		return fmt.Errorf("resolve target (credentials stored but not resolvable): %w", err)
	}

	out := map[string]any{
		"device_id":         deviceID,
		"business_id":       o.business,
		"hardware_name":     hardwareName,
		"management_ip":     host,
		"ssh_port":          sshPort,
		"ssh_username":      sshUser,
		"ssh_secret_kind":   sshKind,
		"ciphertext_stored": hasCipher,
		"fingerprint_set":   hasFinger,
		// DeviceTarget.String() redacts the secret — safe to show.
		"resolved_target": fmt.Sprintf("%v", target),
	}
	if o.asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	fmt.Printf("registered device %s\n", deviceID)
	fmt.Printf("  business     : %s\n", o.business)
	fmt.Printf("  hardware     : %s\n", hardwareName)
	fmt.Printf("  target       : %s\n", fmt.Sprintf("%v", target))
	fmt.Printf("  ciphertext   : stored=%v\n", hasCipher)
	fmt.Printf("  fingerprint  : set=%v\n", hasFinger)
	return nil
}
