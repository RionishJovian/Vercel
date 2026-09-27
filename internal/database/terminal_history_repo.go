package database

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"mega-magic/internal/model"
)

// MaxTerminalHistoryPage caps a page of terminal history.
const MaxTerminalHistoryPage = 200

// DefaultTerminalHistoryPage matches the console's page size.
const DefaultTerminalHistoryPage = 50

// TerminalHistoryRepository reads the command log.
type TerminalHistoryRepository struct {
	db *DB
}

// NewTerminalHistoryRepository creates a terminal history repository.
func NewTerminalHistoryRepository(db *DB) *TerminalHistoryRepository {
	return &TerminalHistoryRepository{db: db}
}

// buildTerminalHistoryWhere assembles the filter clause and its args. Every
// value is a placeholder — nothing is concatenated from caller input.
//
// `source` lives INSIDE the JSON in the `body` TEXT column, so it is matched with
// position() against the serialised key/value pair. Casting to jsonb instead would
// throw on any row whose body is not valid JSON, and the column is TEXT precisely
// because the old app did not guarantee it was.
func buildTerminalHistoryWhere(businessID, source, status, q string) (string, []any) {
	where := "business_id = $1"
	args := []any{businessID}

	if source != "" {
		args = append(args, `"source":"`+source+`"`)
		where += fmt.Sprintf(" AND position($%d in body) > 0", len(args))
	}
	if status != "" {
		args = append(args, status)
		where += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if q != "" {
		args = append(args, "%"+escapeLike(q)+"%")
		n := len(args)
		where += fmt.Sprintf(" AND (path ILIKE $%d ESCAPE '\\' OR coalesce(body,'') ILIKE $%d ESCAPE '\\' OR coalesce(response_snippet,'') ILIKE $%d ESCAPE '\\')", n, n, n)
	}
	return where, args
}

// ListTerminalHistoryTXN returns one page of commands, newest first, plus the total
// matching the filters. Credential values are redacted before returning.
func (r *TerminalHistoryRepository) ListTerminalHistoryTXN(tx *sql.Tx, businessID, source, status, q string, limit, offset int) ([]*model.TerminalHistoryEntry, int, error) {
	where, args := buildTerminalHistoryWhere(businessID, source, status, q)

	var total int
	if err := tx.QueryRow(`SELECT count(*) FROM terminal_history WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count terminal history: %w", err)
	}

	pageArgs := append(append([]any{}, args...), limit, offset)
	rows, err := tx.Query(fmt.Sprintf(`
		SELECT id, business_id, user_id, site_id, router_connection_id, method, path,
		       status, ms, body, response_snippet, created_at
		  FROM terminal_history
		 WHERE %s
		 ORDER BY created_at DESC, id
		 LIMIT $%d OFFSET $%d`, where, len(pageArgs)-1, len(pageArgs)), pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list terminal history: %w", err)
	}
	defer rows.Close()

	out := []*model.TerminalHistoryEntry{}
	for rows.Next() {
		e := &model.TerminalHistoryEntry{}
		var userID, siteID, connID, body, snippet sql.NullString
		var status, ms sql.NullInt32
		if err := rows.Scan(&e.ID, &e.BusinessID, &userID, &siteID, &connID, &e.Method,
			&e.Path, &status, &ms, &body, &snippet, &e.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan terminal history: %w", err)
		}
		// status and ms are nullable in the schema, so a NULL must not abort the row.
		if status.Valid {
			e.Status = int(status.Int32)
			e.StatusKnown = true
		}
		if ms.Valid {
			e.DurationMS = int(ms.Int32)
			e.DurationKnown = true
		}
		if userID.Valid {
			e.UserID = &userID.String
		}
		if siteID.Valid {
			e.SiteID = &siteID.String
		}
		if connID.Valid {
			e.RouterConnectionID = &connID.String
		}
		e.Script, e.Source, e.InsightID, e.BodyUnparsed = decodeCommandBody(body.String)
		e.Script = model.RedactSecrets(e.Script)
		if snippet.Valid {
			e.ResponseSnippet = model.RedactSecrets(snippet.String)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate terminal history: %w", err)
	}
	return out, total, nil
}

// DistinctSourcesTXN lists the `source` values present for a business, newest data
// included. Uses regexp_match on the TEXT column rather than a jsonb cast, so a
// non-JSON body yields NULL instead of an error.
func (r *TerminalHistoryRepository) DistinctSourcesTXN(tx *sql.Tx, businessID string) ([]string, error) {
	rows, err := tx.Query(`
		SELECT DISTINCT (regexp_match(body, '"source"\s*:\s*"([^"]*)"'))[1]
		  FROM terminal_history
		 WHERE business_id = $1 AND body IS NOT NULL`, businessID)
	if err != nil {
		return nil, fmt.Errorf("terminal history sources: %w", err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scan terminal history source: %w", err)
		}
		if s.Valid && s.String != "" {
			out = append(out, s.String)
		}
	}
	return out, rows.Err()
}

// decodeCommandBody lifts script/source/insight_id out of the stored body. The old
// app wrote a JSON object, but the column is TEXT, so a body that does not parse is
// surfaced verbatim with body_unparsed set rather than dropped.
func decodeCommandBody(body string) (script, source, insightID string, unparsed bool) {
	if body == "" {
		return "", "", "", false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return body, "", "", true
	}
	str := func(k string) string {
		if v, ok := m[k].(string); ok {
			return v
		}
		return ""
	}
	script, source, insightID = str("script"), str("source"), str("insight_id")
	if script == "" {
		// No `script` key: keep the raw body visible so nothing is hidden.
		return body, source, insightID, true
	}
	return script, source, insightID, false
}
