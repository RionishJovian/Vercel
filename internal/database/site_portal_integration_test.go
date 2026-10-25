package database

import (
	"os"
	"testing"

	"mega-magic/internal/model"
)

func applySitePortalSchema(t *testing.T, db *DB) {
	t.Helper()
	sqlBytes, err := os.ReadFile("../../migrations/20261016170000_site_portals.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(sqlBytes)); err != nil {
		t.Fatalf("apply site portal schema: %v", err)
	}
}

func TestDB_SitePortalPublishAndTenantIsolation(t *testing.T) {
	db := requireHarness(t)
	applySitePortalSchema(t, db)
	bizA, bizB := insertBusiness(t, db), insertBusiness(t, db)
	var siteA, siteB string
	if err := db.QueryRow(`INSERT INTO sites (business_id, name) VALUES ($1, 'A site') RETURNING site_id`, bizA).Scan(&siteA); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO sites (business_id, name) VALUES ($1, 'B site') RETURNING site_id`, bizB).Scan(&siteB); err != nil {
		t.Fatal(err)
	}
	repo := NewSitePortalRepository()
	portal := &model.SitePortal{
		SiteID: siteA, BusinessID: bizA,
		DraftConfig: &model.PortalConfig{BusinessID: bizA, BusinessName: "A Wi-Fi", VoucherLoginEnabled: true},
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := WithTenant(tx, bizA, func() error { return repo.UpsertDraftTXN(tx, portal) }); err != nil {
		t.Fatal(err)
	}
	if err := WithTenant(tx, bizA, func() error { return repo.PublishTXN(tx, bizA, siteA) }); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetBySiteTXN(tx, bizA, siteA)
	if err != nil || got == nil || got.PublishedRevision == 0 || got.PublishedConfig.BusinessName != "A Wi-Fi" {
		t.Fatalf("published site portal missing: got=%+v err=%v", got, err)
	}
	foreign, err := repo.GetBySiteTXN(tx, bizB, siteA)
	if err != nil {
		t.Fatal(err)
	}
	if foreign != nil {
		t.Fatal("business B resolved business A site portal")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	// A later draft changes only draft state; guests continue seeing the last
	// published revision until the explicit publish operation runs.
	portal.DraftConfig.BusinessName = "A Wi-Fi Draft"
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := WithTenant(tx, bizA, func() error { return repo.UpsertDraftTXN(tx, portal) }); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetBySiteTXN(tx, bizA, siteA)
	if err != nil || got == nil || got.PublishedConfig.BusinessName != "A Wi-Fi" || got.DraftConfig.BusinessName != "A Wi-Fi Draft" {
		t.Fatalf("draft/published revisions crossed: got=%+v err=%v", got, err)
	}
	_ = siteB
	_ = tx.Rollback()
}
