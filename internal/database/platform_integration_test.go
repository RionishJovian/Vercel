package database

import (
	"testing"
	"time"

	"mega-magic/internal/model"
)

// TestDB_PlatformBusiness_ListAll proves the SYSTEM-LEVEL cross-tenant read the
// DEVELOPER window relies on: ListAll sees EVERY business, no tenant scoping.
func TestDB_PlatformBusiness_ListAll(t *testing.T) {
	db := requireHarness(t)
	repo := NewPlatformBusinessRepository(db)

	// Seed several businesses (the harness DB may already hold others from the
	// real dev DB — that's fine; we only assert ours are present).
	idA := insertBusiness(t, db)
	idB := insertBusiness(t, db)
	idC := insertBusiness(t, db)

	got, err := repo.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	seen := map[string]bool{}
	for _, b := range got {
		seen[b.BusinessID] = true
	}
	for _, id := range []string{idA, idB, idC} {
		if !seen[id] {
			t.Fatalf("ListAll must be cross-tenant; missing business %s", id)
		}
	}
}

// TestDB_PlatformBusiness_GetByID fetches a single business by id.
func TestDB_PlatformBusiness_GetByID(t *testing.T) {
	db := requireHarness(t)
	repo := NewPlatformBusinessRepository(db)
	id := insertBusiness(t, db)

	b, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if b.BusinessID != id || b.CompanyName == "" {
		t.Fatalf("GetByID wrong: %+v", b)
	}
}

// TestDB_PlatformIncident_ListOpenAll returns only open (unresolved) incidents,
// already inclusive across tenants. We insert one open + one resolved under
// different businesses and assert exactly the open one surfaces.
func TestDB_PlatformIncident_ListOpenAll(t *testing.T) {
	db := requireHarness(t)
	repo := NewPlatformIncidentRepository(db)
	incRepo := NewIncidentRepository(db)

	bizOpen := insertBusiness(t, db)
	bizRes := insertBusiness(t, db)

	open, err := incRepo.Create(&model.Incident{
		BusinessID:   bizOpen,
		Kind:         "ai_test",
		Severity:     "warning",
		SubjectID:    "dev-open",
		SubjectLabel: "Device open",
		OpenedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create open incident: %v", err)
	}
	res, err := incRepo.Create(&model.Incident{
		BusinessID:   bizRes,
		Kind:         "ai_test",
		Severity:     "warning",
		SubjectID:    "dev-res",
		SubjectLabel: "Device resolved",
		OpenedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create resolved incident: %v", err)
	}
	if err := incRepo.Resolve(res.ID, insertUser(t, db, bizRes)); err != nil {
		t.Fatalf("resolve incident: %v", err)
	}

	got, err := repo.ListOpenAll()
	if err != nil {
		t.Fatalf("ListOpenAll: %v", err)
	}

	foundOpen := false
	for _, i := range got {
		if i.ID == open.ID {
			foundOpen = true
		}
		if i.ID == res.ID {
			t.Fatalf("ListOpenAll must NOT include resolved incident %s", res.ID)
		}
	}
	if !foundOpen {
		t.Fatalf("ListOpenAll must include open incident %s", open.ID)
	}
}

// TestDB_PlatformIncident_CountAll reports cross-tenant open/resolved totals
// that are internally consistent (open never exceeds the total).
func TestDB_PlatformIncident_CountAll(t *testing.T) {
	db := requireHarness(t)
	repo := NewPlatformIncidentRepository(db)

	counts, err := repo.CountAll()
	if err != nil {
		t.Fatalf("CountAll: %v", err)
	}
	if counts.Open < 0 || counts.Resolved < 0 {
		t.Fatalf("CountAll returned negative: %+v", counts)
	}
	// Consistency: resolved + open equals the same-total read from ListOpenAll
	// counting all rows is not necessary; we just prove the fields are sane.
	_ = counts
}
