package database

import (
	"database/sql"
	"fmt"

	"mega-magic/internal/model"
)

// MaxAIUsagePage caps a page of usage events. Rows are tiny (no jsonb payload), so
// the ceiling is higher than the scan list's.
const MaxAIUsagePage = 200

// DefaultAIUsagePage matches the console's page size.
const DefaultAIUsagePage = 50

// AIUsageRepository reads the platform's AI call history.
type AIUsageRepository struct {
	db *DB
}

// NewAIUsageRepository creates an AI usage repository.
func NewAIUsageRepository(db *DB) *AIUsageRepository {
	return &AIUsageRepository{db: db}
}

// ListAIUsageEventsTXN returns one page of events, newest first, plus the total
// matching the filters.
//
// `feature` and `model` are free-text filters and are matched exactly. An unknown
// value returns an empty page rather than an error — unlike an enumerated CHECK
// column, any string is syntactically valid here.
func (r *AIUsageRepository) ListAIUsageEventsTXN(tx *sql.Tx, businessID, feature, modelName string, limit, offset int) ([]*model.AIUsageEvent, int, error) {
	where := "a.business_id = $1"
	args := []any{businessID}

	if feature != "" {
		args = append(args, feature)
		where += fmt.Sprintf(" AND a.feature = $%d", len(args))
	}
	if modelName != "" {
		args = append(args, modelName)
		where += fmt.Sprintf(" AND a.model = $%d", len(args))
	}

	var total int
	if err := tx.QueryRow(`SELECT count(*) FROM ai_usage_events a WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count ai usage: %w", err)
	}

	pageArgs := append(append([]any{}, args...), limit, offset)
	rows, err := tx.Query(fmt.Sprintf(`
		SELECT a.id, a.business_id, a.user_id, a.feature, a.model,
		       a.prompt_tokens, a.completion_tokens, a.total_tokens, a.cached, a.created_at
		  FROM ai_usage_events a
		 WHERE %s
		 ORDER BY a.created_at DESC, a.id
		 LIMIT $%d OFFSET $%d`, where, len(pageArgs)-1, len(pageArgs)), pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list ai usage: %w", err)
	}
	defer rows.Close()

	out := []*model.AIUsageEvent{}
	for rows.Next() {
		e := &model.AIUsageEvent{}
		var userID sql.NullString
		if err := rows.Scan(&e.ID, &e.BusinessID, &userID, &e.Feature, &e.Model,
			&e.PromptTokens, &e.CompletionTokens, &e.TotalTokens, &e.Cached, &e.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan ai usage: %w", err)
		}
		if userID.Valid {
			e.UserID = &userID.String
		}
		e.TokenMismatch = e.TotalTokens != e.PromptTokens+e.CompletionTokens
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate ai usage: %w", err)
	}
	return out, total, nil
}

// groupQuery runs one breakdown (by feature or by model). `column` is a package
// constant, never caller input.
func (r *AIUsageRepository) groupQuery(tx *sql.Tx, businessID, column string) ([]model.AIUsageGroup, error) {
	rows, err := tx.Query(fmt.Sprintf(`
		SELECT %s, count(*), coalesce(sum(prompt_tokens),0),
		       coalesce(sum(completion_tokens),0), coalesce(sum(total_tokens),0)
		  FROM ai_usage_events WHERE business_id = $1
		 GROUP BY 1 ORDER BY 5 DESC, 1`, column), businessID)
	if err != nil {
		return nil, fmt.Errorf("ai usage by %s: %w", column, err)
	}
	defer rows.Close()

	out := []model.AIUsageGroup{}
	for rows.Next() {
		var g model.AIUsageGroup
		if err := rows.Scan(&g.Name, &g.Events, &g.PromptTokens, &g.CompletionTokens, &g.TotalTokens); err != nil {
			return nil, fmt.Errorf("scan ai usage group: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// AIUsageSummaryTXN returns the tenant-wide totals and both breakdowns. It is
// deliberately UNFILTERED: it is the dashboard behind the list, so the figures do
// not move when the user narrows the list.
func (r *AIUsageRepository) AIUsageSummaryTXN(tx *sql.Tx, businessID string) (*model.AIUsageSummary, error) {
	s := &model.AIUsageSummary{}
	err := tx.QueryRow(`
		SELECT count(*),
		       coalesce(sum(prompt_tokens),0), coalesce(sum(completion_tokens),0),
		       coalesce(sum(total_tokens),0),
		       count(*) filter (where cached),
		       count(*) filter (where total_tokens <> prompt_tokens + completion_tokens)
		  FROM ai_usage_events WHERE business_id = $1`, businessID).Scan(
		&s.Events, &s.PromptTokens, &s.CompletionTokens, &s.TotalTokens,
		&s.CachedEvents, &s.MismatchedEvents)
	if err != nil {
		return nil, fmt.Errorf("ai usage totals: %w", err)
	}

	if s.ByFeature, err = r.groupQuery(tx, businessID, "feature"); err != nil {
		return nil, err
	}
	if s.ByModel, err = r.groupQuery(tx, businessID, "model"); err != nil {
		return nil, err
	}
	return s, nil
}
