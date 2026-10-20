package database

import (
	"os"
	"strings"
	"testing"

	"mega-magic/internal/model"
)

// The committed repository and handlers read/write portal_configs.media_host
// and media_port (PortalConfigRepository, portal deployment, guest approvals),
// and migrations/20261004130000_portal_media_server.sql upgrades existing
// databases. These fixtures prove both halves of that contract: the fresh
// init.sql baseline carries the columns and their CHECK constraints, and the
// tracked migration upgrades a pre-media database while preserving every
// existing portal setting.

const preMediaColumns = `    media_host text,
    media_port text NOT NULL DEFAULT '8096',
`

const preMediaPortConstraint = `    CONSTRAINT portal_configs_media_port_check
        CHECK (media_port ~ '^[0-9]{1,5}$' AND media_port::integer BETWEEN 1 AND 65535),
`

const preMediaHostConstraint = `    CONSTRAINT portal_configs_media_host_check
        CHECK (media_host IS NULL OR (
            media_host ~ '^([0-9]{1,3}\.){3}[0-9]{1,3}$'
            AND split_part(media_host, '.', 1)::integer BETWEEN 1 AND 255
            AND split_part(media_host, '.', 2)::integer BETWEEN 0 AND 255
            AND split_part(media_host, '.', 3)::integer BETWEEN 0 AND 255
            AND split_part(media_host, '.', 4)::integer BETWEEN 1 AND 254
            AND split_part(media_host, '.', 1)::integer < 224
            AND (split_part(media_host, '.', 1)::integer = 10
                OR (split_part(media_host, '.', 1)::integer = 192 AND split_part(media_host, '.', 2)::integer = 168)
                OR (split_part(media_host, '.', 1)::integer = 172 AND split_part(media_host, '.', 2)::integer BETWEEN 16 AND 31))
        )),
`

// preMediaBaseline strips the portal-media columns and CHECK constraints from
// the committed fresh baseline, recreating the schema an existing database
// had before the media-server feature shipped.
func preMediaBaseline(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../database/init.sql")
	if err != nil {
		t.Fatalf("read fresh baseline schema: %v", err)
	}
	baseline := string(raw)
	for _, cut := range []string{preMediaColumns, preMediaPortConstraint, preMediaHostConstraint} {
		if strings.Count(baseline, cut) != 1 {
			t.Fatalf("portal-media block not found exactly once in init.sql: %q", cut)
		}
		baseline = strings.Replace(baseline, cut, "", 1)
	}
	return baseline
}

func loadPortalMediaMigration(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../migrations/20261004130000_portal_media_server.sql")
	if err != nil {
		t.Fatalf("read portal-media migration: %v", err)
	}
	return string(raw)
}

func assertPortalCheckRejected(t *testing.T, db *DB, statement string) {
	t.Helper()
	if _, err := db.Exec(statement); err == nil {
		t.Fatalf("portal media CHECK did not reject: %s", statement)
	} else if !strings.Contains(err.Error(), "check constraint") {
		t.Fatalf("expected CHECK constraint rejection, got: %v\n%s", err, statement)
	}
}

func TestPortalMediaMigrationUpgradesExistingDatabase(t *testing.T) {
	db, cleanup := requireRoleMigrationDB(t)
	defer cleanup()

	if _, err := db.Exec(preMediaBaseline(t)); err != nil {
		t.Fatalf("load pre-media baseline schema: %v", err)
	}
	biz := insertBusiness(t, db)

	// Portal settings that existed BEFORE the media feature, with distinctive
	// values whose preservation the upgrade must prove.
	if _, err := db.Exec(`
INSERT INTO portal_configs
    (business_id, business_name, welcome_text, terms_text, primary_hex, glass_tint_hex,
     consent_required, consent_text, bank_name, bank_account, bank_holder,
     guest_grace_minutes, guest_grace_data_mb, guest_plans,
     voucher_login_enabled, mobile_payment_enabled, updated_at)
VALUES ($1::uuid, 'Pre-Media Wi-Fi', 'Welcome wording', 'Terms wording', '#123456', '#abcdef',
        true, 'Consent text', 'KBZ Bank', '12345678', 'Owner A',
        15, 75, '[{"key":"basic","label":"Basic","minutes":60,"amount":1500,"data_mb":512}]'::jsonb,
        true, false, now())`, biz); err != nil {
		t.Fatalf("seed pre-media portal settings: %v", err)
	}

	if _, err := db.Exec(loadPortalMediaMigration(t)); err != nil {
		t.Fatalf("portal-media migration failed on an upgraded database: %v", err)
	}

	// Defaults land without touching any existing setting.
	var mediaHostIsNull bool
	var mediaPort string
	if err := db.QueryRow(`SELECT media_host IS NULL, media_port FROM portal_configs WHERE business_id = $1::uuid`, biz).Scan(&mediaHostIsNull, &mediaPort); err != nil {
		t.Fatalf("read upgraded media defaults: %v", err)
	}
	if !mediaHostIsNull || mediaPort != "8096" {
		t.Fatalf("upgraded defaults = (null=%t, port=%s), want (true, 8096)", mediaHostIsNull, mediaPort)
	}

	// Every pre-existing portal setting survives the upgrade byte-for-byte.
	var name, welcome, terms, hex, tint, consent string
	var consentRequired bool
	var bankName, bankAccount, bankHolder string
	var graceMinutes, graceData int
	var plansEqual bool
	var voucherEnabled bool
	if err := db.QueryRow(`
SELECT business_name, welcome_text, terms_text, primary_hex, glass_tint_hex, consent_required, coalesce(consent_text,''),
       coalesce(bank_name,''), coalesce(bank_account,''), coalesce(bank_holder,''),
       guest_grace_minutes, guest_grace_data_mb,
       guest_plans = '[{"key":"basic","label":"Basic","minutes":60,"amount":1500,"data_mb":512}]'::jsonb,
       voucher_login_enabled
  FROM portal_configs WHERE business_id = $1::uuid`, biz).Scan(
		&name, &welcome, &terms, &hex, &tint, &consentRequired, &consent,
		&bankName, &bankAccount, &bankHolder,
		&graceMinutes, &graceData, &plansEqual, &voucherEnabled); err != nil {
		t.Fatalf("read preserved portal settings: %v", err)
	}
	for _, check := range []struct {
		name string
		ok   bool
	}{
		{"business_name", name == "Pre-Media Wi-Fi"},
		{"welcome_text", welcome == "Welcome wording"},
		{"terms_text", terms == "Terms wording"},
		{"primary_hex", hex == "#123456"},
		{"glass_tint_hex", tint == "#abcdef"},
		{"consent_required", consentRequired},
		{"consent_text", consent == "Consent text"},
		{"bank_name", bankName == "KBZ Bank"},
		{"bank_account", bankAccount == "12345678"},
		{"bank_holder", bankHolder == "Owner A"},
		{"guest_grace_minutes", graceMinutes == 15},
		{"guest_grace_data_mb", graceData == 75},
		{"guest_plans", plansEqual},
		{"voucher_login_enabled", voucherEnabled},
	} {
		if !check.ok {
			t.Fatalf("portal setting %s changed during the media upgrade", check.name)
		}
	}

	// The real repository round-trips media values after the upgrade.
	repo := NewPortalConfigRepository(db)
	cfg, err := repo.GetByBusiness(biz)
	if err != nil {
		t.Fatalf("repo read after upgrade: %v", err)
	}
	if cfg.MediaHost != "" || cfg.MediaPort != "8096" || cfg.BusinessName != "Pre-Media Wi-Fi" {
		t.Fatalf("repo read after upgrade = (%q, %q, %q)", cfg.MediaHost, cfg.MediaPort, cfg.BusinessName)
	}
	cfg.MediaHost = "10.88.20.5"
	cfg.MediaPort = "8920"
	if err := repo.Upsert(cfg); err != nil {
		t.Fatalf("repo upsert with media values after upgrade: %v", err)
	}
	cfg, err = repo.GetByBusiness(biz)
	if err != nil {
		t.Fatalf("repo re-read after upgrade: %v", err)
	}
	if cfg.MediaHost != "10.88.20.5" || cfg.MediaPort != "8920" || cfg.WelcomeText != "Welcome wording" {
		t.Fatalf("media round-trip lost settings: host=%q port=%q welcome=%q", cfg.MediaHost, cfg.MediaPort, cfg.WelcomeText)
	}

	// Storage-layer guards match the fresh baseline exactly.
	assertPortalCheckRejected(t, db, `UPDATE portal_configs SET media_host='8.8.8.8' WHERE business_id='`+biz+`'`)
	assertPortalCheckRejected(t, db, `UPDATE portal_configs SET media_host='172.32.0.1' WHERE business_id='`+biz+`'`)
	assertPortalCheckRejected(t, db, `UPDATE portal_configs SET media_port='0' WHERE business_id='`+biz+`'`)
	assertPortalCheckRejected(t, db, `UPDATE portal_configs SET media_port='70000' WHERE business_id='`+biz+`'`)
	if _, err := db.Exec(`UPDATE portal_configs SET media_host='192.168.88.10', media_port='8096' WHERE business_id='` + biz + `'`); err != nil {
		t.Fatalf("private media target rejected after upgrade: %v", err)
	}
	// The migration is idempotent: a second run must not disturb anything.
	if _, err := db.Exec(loadPortalMediaMigration(t)); err != nil {
		t.Fatalf("portal-media migration is not idempotent: %v", err)
	}
}

func TestPortalMediaColumnsExistOnFreshSchema(t *testing.T) {
	db, cleanup := requireRoleMigrationDB(t)
	defer cleanup()

	raw, err := os.ReadFile("../../database/init.sql")
	if err != nil {
		t.Fatalf("read fresh baseline schema: %v", err)
	}
	if _, err := db.Exec(string(raw)); err != nil {
		t.Fatalf("fresh baseline schema must load cleanly: %v", err)
	}

	for _, check := range []struct{ column, want string }{
		{"media_host", "text"},
		{"media_port", "text"},
	} {
		var dataType string
		if err := db.QueryRow(`
SELECT data_type FROM information_schema.columns
 WHERE table_name='portal_configs' AND column_name=$1`, check.column).Scan(&dataType); err != nil {
			t.Fatalf("fresh schema missing portal_configs.%s: %v", check.column, err)
		}
		if dataType != check.want {
			t.Fatalf("portal_configs.%s type = %s, want %s", check.column, dataType, check.want)
		}
	}

	// The real repository round-trips media values on a fresh database too.
	biz := insertBusiness(t, db)
	repo := NewPortalConfigRepository(db)
	if err := repo.Upsert(&model.PortalConfig{
		BusinessID: biz, BusinessName: "Fresh Wi-Fi", ConsentRequired: true,
		MediaHost: "192.168.88.10", MediaPort: "8096",
		VoucherLoginEnabled: true,
	}); err != nil {
		t.Fatalf("repo upsert on fresh schema: %v", err)
	}
	cfg, err := repo.GetByBusiness(biz)
	if err != nil {
		t.Fatalf("repo read on fresh schema: %v", err)
	}
	if cfg.MediaHost != "192.168.88.10" || cfg.MediaPort != "8096" {
		t.Fatalf("fresh-schema media round-trip = (%q, %q)", cfg.MediaHost, cfg.MediaPort)
	}

	// The fresh baseline enforces the same guards as the migration.
	assertPortalCheckRejected(t, db, `UPDATE portal_configs SET media_host='203.0.113.5' WHERE business_id='`+biz+`'`)
	assertPortalCheckRejected(t, db, `UPDATE portal_configs SET media_port='89a0' WHERE business_id='`+biz+`'`)
	if _, err := db.Exec(`UPDATE portal_configs SET media_port='8920' WHERE business_id='` + biz + `'`); err != nil {
		t.Fatalf("valid media port rejected on fresh schema: %v", err)
	}
}
