package database

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"mega-magic/internal/model"
)

// MaxI18nPage caps a page of translation overrides.
const MaxI18nPage = 500

// DefaultI18nPage matches the console's page size.
const DefaultI18nPage = 100

var (
	// ErrI18nOverrideNotFound is returned when no override has that id.
	ErrI18nOverrideNotFound = errors.New("translation override not found")
	// ErrI18nKeyExists is returned when (locale, ns, key) already exists. The
	// database enforces this with a real UNIQUE constraint, so the error is
	// detected from the 23505 violation rather than by pre-checking.
	ErrI18nKeyExists = errors.New("an override for that locale, namespace and key already exists")
)

// I18nRepository reads and writes translation overrides.
//
// ⚠️ GLOBAL TABLE — no business_id, RLS disabled. Nothing here is tenant-scoped,
// by design. Write routes must be gated to the DEVELOPER platform role.
type I18nRepository struct {
	db *DB
}

// NewI18nRepository creates a translation repository.
func NewI18nRepository(db *DB) *I18nRepository {
	return &I18nRepository{db: db}
}

// isUniqueViolation reports whether err is a Postgres unique-constraint breach.
func isUniqueViolation(err error) bool {
	var pgErr *pq.Error
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

const i18nColumns = `id, locale, ns, key, value, updated_by, updated_at`

func scanI18n(row interface{ Scan(...any) error }) (*model.I18nOverride, error) {
	o := &model.I18nOverride{}
	var updatedBy sql.NullString
	if err := row.Scan(&o.ID, &o.Locale, &o.NS, &o.Key, &o.Value, &o.UpdatedBy, &o.UpdatedAt); err != nil {
		return nil, err
	}
	if updatedBy.Valid {
		o.UpdatedBy = &updatedBy.String
	}
	return o, nil
}

// ListI18nOverridesTXN returns one page of overrides ordered by locale, namespace
// and key, plus the total matching the filters.
func (r *I18nRepository) ListI18nOverridesTXN(tx *sql.Tx, locale, ns, q string, limit, offset int) ([]*model.I18nOverride, int, error) {
	where := "TRUE"
	args := []any{}

	if locale != "" {
		args = append(args, locale)
		where += fmt.Sprintf(" AND locale = $%d", len(args))
	}
	if ns != "" {
		args = append(args, ns)
		where += fmt.Sprintf(" AND ns = $%d", len(args))
	}
	if q != "" {
		args = append(args, "%"+escapeLike(q)+"%")
		n := len(args)
		where += fmt.Sprintf(" AND (key ILIKE $%d ESCAPE '\\' OR value ILIKE $%d ESCAPE '\\')", n, n)
	}

	var total int
	if err := tx.QueryRow(`SELECT count(*) FROM i18n_overrides WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count i18n overrides: %w", err)
	}

	pageArgs := append(append([]any{}, args...), limit, offset)
	rows, err := tx.Query(fmt.Sprintf(`
		SELECT %s FROM i18n_overrides
		 WHERE %s
		 ORDER BY locale, ns, key
		 LIMIT $%d OFFSET $%d`, i18nColumns, where, len(pageArgs)-1, len(pageArgs)), pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list i18n overrides: %w", err)
	}
	defer rows.Close()

	out := []*model.I18nOverride{}
	for rows.Next() {
		o, err := scanI18n(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan i18n override: %w", err)
		}
		out = append(out, o)
	}
	return out, total, rows.Err()
}

// GetI18nOverrideTXN loads one override by id.
func (r *I18nRepository) GetI18nOverrideTXN(tx *sql.Tx, id string) (*model.I18nOverride, error) {
	o, err := scanI18n(tx.QueryRow(`SELECT `+i18nColumns+` FROM i18n_overrides WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrI18nOverrideNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get i18n override: %w", err)
	}
	return o, nil
}

// CreateI18nOverrideTXN inserts a translation. A duplicate (locale, ns, key) is
// reported as ErrI18nKeyExists.
func (r *I18nRepository) CreateI18nOverrideTXN(tx *sql.Tx, locale, ns, key, value string, updatedBy *string) (*model.I18nOverride, error) {
	o, err := scanI18n(tx.QueryRow(`
		INSERT INTO i18n_overrides (locale, ns, key, value, updated_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+i18nColumns, locale, ns, key, value, updatedBy))
	if isUniqueViolation(err) {
		return nil, ErrI18nKeyExists
	}
	if err != nil {
		return nil, fmt.Errorf("create i18n override: %w", err)
	}
	return o, nil
}

// UpdateI18nOverrideTXN replaces an override's identity and value. Changing the
// key/locale/namespace is allowed — fixing a typo should not require a delete —
// and a collision is reported as ErrI18nKeyExists. updated_at is always bumped and
// updated_by is always overwritten with the caller.
func (r *I18nRepository) UpdateI18nOverrideTXN(tx *sql.Tx, id, locale, ns, key, value string, updatedBy *string) (*model.I18nOverride, error) {
	o, err := scanI18n(tx.QueryRow(`
		UPDATE i18n_overrides
		   SET locale = $2, ns = $3, key = $4, value = $5,
		       updated_by = $6, updated_at = now()
		 WHERE id = $1
		RETURNING `+i18nColumns, id, locale, ns, key, value, updatedBy))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrI18nOverrideNotFound
	}
	if isUniqueViolation(err) {
		return nil, ErrI18nKeyExists
	}
	if err != nil {
		return nil, fmt.Errorf("update i18n override: %w", err)
	}
	return o, nil
}

// DeleteI18nOverrideTXN removes an override, reporting ErrI18nOverrideNotFound if
// nothing matched.
func (r *I18nRepository) DeleteI18nOverrideTXN(tx *sql.Tx, id string) error {
	res, err := tx.Exec(`DELETE FROM i18n_overrides WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete i18n override: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete i18n override: %w", err)
	}
	if n == 0 {
		return ErrI18nOverrideNotFound
	}
	return nil
}

// I18nFacetsTXN returns how many overrides exist per locale and per namespace.
// Values seen in the data, not the constraint's allowed set — the console shows
// both so a locale with zero translations is still selectable.
func (r *I18nRepository) I18nFacetsTXN(tx *sql.Tx) (map[string]int, map[string]int, error) {
	rows, err := tx.Query(`SELECT locale, ns, count(*) FROM i18n_overrides GROUP BY 1, 2`)
	if err != nil {
		return nil, nil, fmt.Errorf("i18n facets: %w", err)
	}
	defer rows.Close()

	byLocale := map[string]int{}
	byNS := map[string]int{}
	for rows.Next() {
		var locale, ns string
		var n int
		if err := rows.Scan(&locale, &ns, &n); err != nil {
			return nil, nil, fmt.Errorf("scan i18n facet: %w", err)
		}
		byLocale[locale] += n
		byNS[ns] += n
	}
	return byLocale, byNS, rows.Err()
}
