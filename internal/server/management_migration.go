package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"mega-magic/internal/hub"
	"mega-magic/internal/managementmigration"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/secretbox"
	"net"
	"os"
	"strings"
	"time"
)

type managementMoveService struct {
	db      *database.DB
	store   database.ManagementMigrationStore
	source  routeros.TargetSource
	allowed []string
	hub     *hub.Client
}
type moveHub struct {
	client *hub.Client
	id     hub.Identity
}

func (h moveHub) run(ctx context.Context, j *managementmigration.Job, action string) error {
	out, e := h.client.ManagementMigration(ctx, h.id, action, hub.ManagementMove{OperationID: j.ID, OldAddress: j.OldAddress + "/32", NewAddress: j.NewAddress + "/32", RouterPublicKey: j.RouterKey})
	if e != nil {
		return e
	}
	want := map[string]string{"stage": "staged", "commit": "committed", "finalize": "completed", "abort": "rolled_back"}[action]
	if out.Phase != want {
		return fmt.Errorf("Hub migration checkpoint not verified")
	}
	return nil
}
func (h moveHub) Stage(c context.Context, j *managementmigration.Job) error {
	return h.run(c, j, "stage")
}
func (h moveHub) Commit(c context.Context, j *managementmigration.Job) error {
	return h.run(c, j, "commit")
}
func (h moveHub) Finalize(c context.Context, j *managementmigration.Job) error {
	return h.run(c, j, "finalize")
}
func (h moveHub) Abort(c context.Context, j *managementmigration.Job) error {
	return h.run(c, j, "abort")
}
func (s *managementMoveService) Status(ctx context.Context, biz, device string) (*managementmigration.Job, error) {
	j, e := s.store.Current(ctx, biz, device)
	if e == sql.ErrNoRows {
		return nil, nil
	}
	return j, e
}
func moveUUID() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func (s *managementMoveService) Preview(ctx context.Context, biz, device, user string) (*managementmigration.Job, error) {
	unlock, e := s.store.Lease(ctx, biz, device)
	if e != nil {
		return nil, e
	}
	defer unlock()
	current, e := s.Status(ctx, biz, device)
	if e != nil {
		return nil, e
	}
	if current != nil && current.Phase != "completed" && current.Phase != "rolled_back" {
		if current.Phase == "planned" && current.RequestedBy == user {
			return current, nil
		}
		return nil, fmt.Errorf("recover the existing migration first")
	}
	peer, e := s.hub.InspectRouter(ctx, hub.Identity{TenantID: biz, RouterID: device, RequestedBy: user})
	if e != nil {
		return nil, e
	}
	if peer.State != "active" || peer.LastHandshakeAt == nil || time.Since(*peer.LastHandshakeAt) < 0 || time.Since(*peer.LastHandshakeAt) > 5*time.Minute {
		return nil, managementmigration.Explain("hub_offline", "A recent Magic Hub handshake is required. Connect the router and check Magic Hub before preparing a migration.")
	}
	candidate, e := s.hub.ManagementMigration(ctx, hub.Identity{TenantID: biz, RouterID: device, RequestedBy: user}, "candidate", hub.ManagementMove{})
	if e != nil {
		return nil, managementmigration.Explain("hub_migration_unavailable", "The Hub migration protocol is unavailable or its allocation changed. Refresh availability; do not reenroll or replace router keys.")
	}
	if candidate.OldAddress != peer.TunnelAddress || candidate.RouterPublicKey != peer.RouterPublicKey || candidate.Phase != "candidate" {
		return nil, fmt.Errorf("Hub identity changed")
	}
	id, e := moveUUID()
	if e != nil {
		return nil, e
	}
	j := &managementmigration.Job{ID: id, BusinessID: biz, DeviceID: device, RequestedBy: user, OldAddress: strings.TrimSuffix(candidate.OldAddress, "/32"), NewAddress: strings.TrimSuffix(candidate.NewAddress, "/32"), RouterKey: peer.RouterPublicKey, Phase: "planned", ExpiresAt: time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)}
	router, e := routeros.NewManagementMigrationRouter(ctx, s.source, s.allowed, biz, device, nil)
	if e != nil {
		return nil, e
	}
	if e = router.Inspect(ctx, j); e != nil {
		return nil, e
	}
	if e = s.store.Create(ctx, j); e != nil {
		return nil, e
	}
	return j, nil
}

// Run is synchronous and bounded. On any uncertain response the UI refreshes
// Status; it never replays Apply. Recovery reloads the journal under its lease.
func (s *managementMoveService) Run(ctx context.Context, biz, device, user, id, action string, revision int) (*managementmigration.Job, error) {
	unlock, e := s.store.Lease(ctx, biz, device)
	if e != nil {
		return nil, e
	}
	defer unlock()
	// Share the app's network-operation mutex as well as the cross-process lease.
	release := routeros.LockManagementDevice(biz, device)
	defer release()
	j, e := s.store.Load(ctx, biz, device, id)
	if e != nil {
		return nil, e
	}
	if j.Revision != revision {
		return j, fmt.Errorf("migration state changed; refresh")
	}
	router, e := routeros.NewManagementMigrationRouter(ctx, s.source, s.allowed, biz, device, nil)
	if e != nil {
		return j, e
	}
	target, e := s.source.TargetForDevice(ctx, biz, device)
	if e != nil {
		return j, e
	}
	host, _, e := net.SplitHostPort(target.Addr)
	if e != nil {
		return j, e
	}
	expected := j.OldAddress
	if j.Phase == "connection_saved" || j.Phase == "router_cleaned" || j.Phase == "completed" {
		expected = j.NewAddress
	}
	if host != expected || target.Fingerprint != j.Fingerprint {
		return j, fmt.Errorf("saved connection changed outside this migration; operator review required")
	}
	engine := managementmigration.Engine{Store: s.store, Hub: moveHub{s.hub, hub.Identity{TenantID: biz, RouterID: device, RequestedBy: user}}, Router: router}
	if action == "apply" {
		expires, e := time.Parse(time.RFC3339, j.ExpiresAt)
		if e != nil || !time.Now().Before(expires) {
			return j, managementmigration.Explain("preview_expired", "This preview expired. Cancel it and prepare a fresh migration review.")
		}
		e = engine.Apply(ctx, j)
	} else if action == "recover" {
		e = engine.Recover(ctx, j)
	} else {
		return j, fmt.Errorf("unsupported migration action")
	}
	// The transaction commit response could have been lost. Reload before any
	// recovery choice. A failed refresh returns an error rather than guessing.
	saved, readErr := s.store.Load(ctx, biz, device, id)
	if readErr != nil {
		return nil, fmt.Errorf("migration status unavailable; refresh before any further action")
	}
	return saved, e
}

func configureManagementMoves(h *handler.SecurityHandler, db *database.DB, cfg *config.Config, client *hub.Client) error {
	if client == nil || !cfg.Provisioner.Applier.Enabled {
		return nil
	}
	if os.Getenv("MM_MANAGEMENT_MIGRATION_ENABLED") == "true" {
		if e := database.CheckManagementMigrationSchema(db); e != nil {
			return e
		}
	}
	box, e := secretbox.New(cfg.Provisioner.Applier.CredentialKey)
	if e != nil {
		return fmt.Errorf("management migration credential source unavailable")
	}
	h.ConfigureManagementMigrations(&managementMoveService{db: db, store: database.ManagementMigrationStore{DB: db}, source: database.NewDeviceCredentialSource(db, box), allowed: cfg.Provisioner.AllowedTargetCIDRs, hub: client})
	return nil
}
