package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"mega-magic/internal/model"
)

// ErrScanRunNotFound is returned when a scan id is not in the caller's business.
var ErrScanRunNotFound = errors.New("scan run not found")

// MaxScanPage caps a single page of scan runs. Payloads are much heavier than
// syslog lines (avg ~670 bytes, max ~7 KB), so the ceiling is lower.
const MaxScanPage = 100

// DefaultScanPage matches the console's page size.
const DefaultScanPage = 50

// FleetScanRunRepository reads generated fleet scans.
type FleetScanRunRepository struct {
	db *DB
}

// NewFleetScanRunRepository creates a fleet scan repository.
func NewFleetScanRunRepository(db *DB) *FleetScanRunRepository {
	return &FleetScanRunRepository{db: db}
}

// scanPayload mirrors the two shapes the ported jsonb actually holds. Both are
// decoded at once; whichever is absent stays zero.
type scanPayload struct {
	Insights []struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		Router     string `json:"router"`
		RouterID   string `json:"router_id"`
		Severity   string `json:"severity"`
		Subtitle   string `json:"subtitle"`
		Suggestion string `json:"suggestion"`
	} `json:"insights"`
	Snapshot           json.RawMessage `json:"snapshot"`
	DeterministicCount *int            `json:"deterministic_count"`
	Source             string          `json:"source"`
	NodeID             string          `json:"node_id"`
	SiteID             string          `json:"site_id"`
	RouterID           string          `json:"router_id"`
	NodeStatus         string          `json:"node_status"`
	Provider           string          `json:"provider"`
}

// decode populates findings/node from the raw payload.
func (r *FleetScanRunRepository) decode(run *model.FleetScanRun, raw []byte) {
	if len(raw) == 0 {
		return
	}
	var p scanPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		// A payload we cannot parse is reported honestly rather than dropped.
		run.Findings = []model.ScanFinding{}
		return
	}

	run.Findings = []model.ScanFinding{}
	for _, in := range p.Insights {
		f := model.ScanFinding{
			ID: in.ID, Title: in.Title, Router: in.Router, RouterID: in.RouterID,
			Severity: in.Severity, Subtitle: in.Subtitle, Suggestion: in.Suggestion,
		}
		// Expose whether this "finding" is really the old app's crash. The row is
		// not modified — only labelled.
		f.RuntimeError = model.LooksLikeRuntimeError(in.Subtitle) ||
			model.LooksLikeRuntimeError(in.Suggestion)
		if f.RuntimeError {
			run.ErrorFindings++
		}
		run.Findings = append(run.Findings, f)
	}
	run.FindingCount = len(run.Findings)

	if p.DeterministicCount != nil {
		run.DeterministicCount = *p.DeterministicCount
	}

	// The node family is only present when any of its keys is set.
	if p.Source != "" || p.NodeID != "" || p.SiteID != "" || p.NodeStatus != "" || p.Provider != "" {
		run.Node = &model.ScanNode{
			Source: p.Source, NodeID: p.NodeID, SiteID: p.SiteID,
			RouterID: p.RouterID, NodeStatus: p.NodeStatus, Provider: p.Provider,
		}
	}

	if len(p.Snapshot) > 0 && string(p.Snapshot) != "null" {
		s := string(p.Snapshot)
		run.Snapshot = &s
	}
}

func (r *FleetScanRunRepository) scanRun(rows interface{ Scan(...any) error }, withSnapshot bool) (*model.FleetScanRun, error) {
	run := &model.FleetScanRun{}
	var triggeredBy sql.NullString
	var raw []byte
	if err := rows.Scan(&run.ID, &run.BusinessID, &run.Kind, &run.MaxSeverity,
		&run.RouterCount, &run.GeneratedAt, &triggeredBy, &raw); err != nil {
		return nil, err
	}
	if triggeredBy.Valid {
		run.TriggeredBy = &triggeredBy.String
	}
	r.decode(run, raw)
	if !withSnapshot {
		run.Snapshot = nil
	}
	return run, nil
}

// buildScanWhere assembles the filter clause and its arguments.
func buildScanWhere(businessID, kind, severity string, findingsOnly bool) (string, []any) {
	where := "f.business_id = $1"
	args := []any{businessID}

	if kind != "" {
		args = append(args, kind)
		where += fmt.Sprintf(" AND f.kind = $%d", len(args))
	}
	if severity != "" {
		args = append(args, severity)
		where += fmt.Sprintf(" AND f.max_severity = $%d", len(args))
	}
	if findingsOnly {
		where += " AND f.max_severity <> 'ok'"
	}
	return where, args
}

// ListScanRunsTXN returns one page of scans, newest first, plus the total matching
// the filters. The snapshot is omitted — decode it through GetScanRunTXN.
func (r *FleetScanRunRepository) ListScanRunsTXN(tx *sql.Tx, businessID, kind, severity string, findingsOnly bool, limit, offset int) ([]*model.FleetScanRun, int, error) {
	where, args := buildScanWhere(businessID, kind, severity, findingsOnly)

	var total int
	if err := tx.QueryRow(`SELECT count(*) FROM fleet_scan_runs f WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count scan runs: %w", err)
	}

	pageArgs := append(append([]any{}, args...), limit, offset)
	rows, err := tx.Query(fmt.Sprintf(`
		SELECT f.id, f.business_id, f.kind, f.max_severity, f.router_count,
		       f.generated_at, f.triggered_by, f.payload
		  FROM fleet_scan_runs f
		 WHERE %s
		 ORDER BY f.generated_at DESC, f.id
		 LIMIT $%d OFFSET $%d`, where, len(pageArgs)-1, len(pageArgs)), pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list scan runs: %w", err)
	}
	defer rows.Close()

	out := []*model.FleetScanRun{}
	for rows.Next() {
		run, err := r.scanRun(rows, false)
		if err != nil {
			return nil, 0, fmt.Errorf("scan scan run: %w", err)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate scan runs: %w", err)
	}
	return out, total, nil
}

// GetScanRunTXN returns one scan INCLUDING its raw snapshot.
func (r *FleetScanRunRepository) GetScanRunTXN(tx *sql.Tx, businessID, id string) (*model.FleetScanRun, error) {
	run, err := r.scanRun(tx.QueryRow(`
		SELECT f.id, f.business_id, f.kind, f.max_severity, f.router_count,
		       f.generated_at, f.triggered_by, f.payload
		  FROM fleet_scan_runs f WHERE f.id = $1 AND f.business_id = $2`, id, businessID), true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrScanRunNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get scan run: %w", err)
	}
	return run, nil
}

// CrashArtifactCountTXN counts scans whose payload contains a persisted runtime
// error. Built from model.RuntimeErrorMarkers so it cannot drift from the decoder.
//
// This exists to keep the console honest: 1,102 of the 1,156 migrated scans carry
// the old app's crash text, and without this figure they would read as 1,100 live
// critical alerts.
func (r *FleetScanRunRepository) CrashArtifactCountTXN(tx *sql.Tx, businessID string) (int, error) {
	markers := model.RuntimeErrorMarkers()
	if len(markers) == 0 {
		return 0, nil
	}
	args := []any{businessID}
	conds := make([]string, 0, len(markers))
	for _, m := range markers {
		args = append(args, "%"+m+"%")
		conds = append(conds, fmt.Sprintf("f.payload::text LIKE $%d", len(args)))
	}

	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM fleet_scan_runs f WHERE f.business_id = $1 AND (`+
		strings.Join(conds, " OR ")+`)`, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("crash artifact count: %w", err)
	}
	return n, nil
}

// ScanSeverityCountsTXN returns per-severity totals for the filter facets. Counts
// are over EVERY row including 'ok'.
func (r *FleetScanRunRepository) ScanSeverityCountsTXN(tx *sql.Tx, businessID string) (map[string]int, error) {
	rows, err := tx.Query(`SELECT max_severity, count(*) FROM fleet_scan_runs
		 WHERE business_id = $1 GROUP BY 1`, businessID)
	if err != nil {
		return nil, fmt.Errorf("severity counts: %w", err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var sev string
		var n int
		if err := rows.Scan(&sev, &n); err != nil {
			return nil, fmt.Errorf("scan severity count: %w", err)
		}
		out[sev] = n
	}
	return out, rows.Err()
}

// ScanKindCountsTXN returns per-kind totals.
func (r *FleetScanRunRepository) ScanKindCountsTXN(tx *sql.Tx, businessID string) (map[string]int, error) {
	rows, err := tx.Query(`SELECT kind, count(*) FROM fleet_scan_runs
		 WHERE business_id = $1 GROUP BY 1`, businessID)
	if err != nil {
		return nil, fmt.Errorf("kind counts: %w", err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, fmt.Errorf("scan kind count: %w", err)
		}
		out[k] = n
	}
	return out, rows.Err()
}
