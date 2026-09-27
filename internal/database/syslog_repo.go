package database

import (
	"database/sql"
	"fmt"
	"strings"

	"mega-magic/internal/model"
)

// SyslogRepository reads the ported router event log.
type SyslogRepository struct {
	db *DB
}

// NewSyslogRepository creates a syslog repository.
func NewSyslogRepository(db *DB) *SyslogRepository {
	return &SyslogRepository{db: db}
}

// MaxSyslogPage caps a single page so a caller cannot ask for the whole log.
const MaxSyslogPage = 200

// escapeLike neutralises LIKE wildcards in user input, so searching for "50%"
// looks for a literal percent sign instead of matching everything.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// buildSyslogWhere returns the parameterised WHERE clause and its args. Every
// fragment is a fixed string with a positional placeholder — caller values only
// ever travel as bind parameters.
func buildSyslogWhere(businessID string, f *model.SyslogFilter) (string, []any) {
	where := []string{"business_id = $1"}
	args := []any{businessID}

	if f.Severity != "" {
		args = append(args, f.Severity)
		where = append(where, fmt.Sprintf("severity = $%d", len(args)))
	}
	if f.Program != "" {
		args = append(args, f.Program)
		where = append(where, fmt.Sprintf("program = $%d", len(args)))
	}
	if f.Query != "" {
		args = append(args, "%"+escapeLike(f.Query)+"%")
		where = append(where, fmt.Sprintf(`message ILIKE $%d ESCAPE '\'`, len(args)))
	}
	return strings.Join(where, " AND "), args
}

// ListSyslogEventsTXN returns one newest-first page plus the total matching the
// filter, so the UI can show "showing N of M" without a second round trip.
func (r *SyslogRepository) ListSyslogEventsTXN(tx *sql.Tx, businessID string, f *model.SyslogFilter) ([]*model.SyslogEvent, int, error) {
	whereSQL, args := buildSyslogWhere(businessID, f)

	var total int
	if err := tx.QueryRow(`SELECT count(*) FROM syslog_events WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count syslog events: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > MaxSyslogPage {
		limit = MaxSyslogPage
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	args = append(args, limit, offset)
	rows, err := tx.Query(fmt.Sprintf(`
		SELECT id, business_id, router_connection_id, site_id, source_ip, facility,
		       severity, program, message, ai_summary, ai_severity, received_at
		  FROM syslog_events
		 WHERE %s
		 ORDER BY received_at DESC
		 LIMIT $%d OFFSET $%d`, whereSQL, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list syslog events: %w", err)
	}
	defer rows.Close()

	out := []*model.SyslogEvent{}
	for rows.Next() {
		e := &model.SyslogEvent{}
		var connID, siteID, sourceIP, facility, program, aiSummary, aiSeverity sql.NullString
		if err := rows.Scan(&e.ID, &e.BusinessID, &connID, &siteID, &sourceIP, &facility,
			&e.Severity, &program, &e.Message, &aiSummary, &aiSeverity, &e.ReceivedAt); err != nil {
			return nil, 0, fmt.Errorf("scan syslog event: %w", err)
		}
		if connID.Valid {
			e.RouterConnectionID = &connID.String
		}
		if siteID.Valid {
			e.SiteID = &siteID.String
		}
		if sourceIP.Valid {
			e.SourceIP = &sourceIP.String
		}
		if facility.Valid {
			e.Facility = &facility.String
		}
		if program.Valid {
			e.Program = &program.String
		}
		if aiSummary.Valid {
			e.AISummary = &aiSummary.String
		}
		if aiSeverity.Valid {
			e.AISeverity = &aiSeverity.String
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate syslog events: %w", err)
	}
	return out, total, nil
}

// SyslogFacetsTXN returns the programs present in this business's log, so the
// filter offers only values that can actually match.
func (r *SyslogRepository) SyslogFacetsTXN(tx *sql.Tx, businessID string) ([]string, error) {
	rows, err := tx.Query(`
		SELECT DISTINCT program FROM syslog_events
		 WHERE business_id = $1 AND program IS NOT NULL
		 ORDER BY program`, businessID)
	if err != nil {
		return nil, fmt.Errorf("syslog programs: %w", err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("scan program: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
