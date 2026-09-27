package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"mega-magic/internal/model"
)

// ErrTerminalTemplateNotFound is returned when a template id is not in the
// caller's business.
var ErrTerminalTemplateNotFound = errors.New("terminal template not found")

// ErrTerminalTemplateIsBuiltin is returned when a caller tries to modify or
// delete shipped content. Built-ins are the product's template library.
var ErrTerminalTemplateIsBuiltin = errors.New("built-in terminal templates are read-only")

// TerminalTemplateRepository reads and writes saved RouterOS REST calls.
type TerminalTemplateRepository struct {
	db *DB
}

// NewTerminalTemplateRepository creates a terminal template repository.
func NewTerminalTemplateRepository(db *DB) *TerminalTemplateRepository {
	return &TerminalTemplateRepository{db: db}
}

const templateColumns = `
	t.id, t.business_id, t.created_by, t.name, t.description, t.category,
	t.method, t.path, t.body, t.is_builtin, t.created_at, t.updated_at`

func scanTemplate(row interface{ Scan(...any) error }) (*model.TerminalTemplate, error) {
	t := &model.TerminalTemplate{}
	var createdBy, description, body sql.NullString
	if err := row.Scan(&t.ID, &t.BusinessID, &createdBy, &t.Name, &description,
		&t.Category, &t.Method, &t.Path, &body, &t.IsBuiltin, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	if createdBy.Valid {
		t.CreatedBy = &createdBy.String
	}
	if description.Valid {
		t.Description = &description.String
	}
	if body.Valid {
		t.Body = &body.String
	}
	return t, nil
}

// ListTerminalTemplatesTXN returns the business's library, built-ins first, with
// optional category and name/search filters.
func (r *TerminalTemplateRepository) ListTerminalTemplatesTXN(tx *sql.Tx, businessID, category, query string) ([]*model.TerminalTemplate, error) {
	where := []string{"t.business_id = $1"}
	args := []any{businessID}

	if category != "" {
		args = append(args, category)
		where = append(where, fmt.Sprintf("t.category = $%d", len(args)))
	}
	if query != "" {
		args = append(args, "%"+escapeLike(query)+"%")
		where = append(where, fmt.Sprintf(
			`(t.name ILIKE $%d ESCAPE '\' OR t.path ILIKE $%d ESCAPE '\')`, len(args), len(args)))
	}

	rows, err := tx.Query(fmt.Sprintf(`
		SELECT %s FROM terminal_templates t
		 WHERE %s
		 ORDER BY t.is_builtin DESC, t.category, t.name`, templateColumns, strings.Join(where, " AND ")), args...)
	if err != nil {
		return nil, fmt.Errorf("list terminal templates: %w", err)
	}
	defer rows.Close()

	out := []*model.TerminalTemplate{}
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, fmt.Errorf("scan terminal template: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate terminal templates: %w", err)
	}
	return out, nil
}

// GetTerminalTemplateTXN returns one template, or ErrTerminalTemplateNotFound.
func (r *TerminalTemplateRepository) GetTerminalTemplateTXN(tx *sql.Tx, businessID, id string) (*model.TerminalTemplate, error) {
	t, err := scanTemplate(tx.QueryRow(
		`SELECT `+templateColumns+` FROM terminal_templates t WHERE t.id = $1 AND t.business_id = $2`,
		id, businessID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTerminalTemplateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get terminal template: %w", err)
	}
	return t, nil
}

// CreateTerminalTemplateTXN adds a CUSTOM template. is_builtin is hard-coded
// false — the flag is server-owned.
func (r *TerminalTemplateRepository) CreateTerminalTemplateTXN(tx *sql.Tx, businessID, userID string, req *model.TerminalTemplateRequest) (*model.TerminalTemplate, error) {
	var id string
	if err := tx.QueryRow(`
		INSERT INTO terminal_templates
		  (business_id, created_by, name, description, category, method, path, body, is_builtin)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, false)
		RETURNING id`,
		businessID, userID, req.Name, req.Description, req.Category, req.Method, req.Path, req.Body,
	).Scan(&id); err != nil {
		return nil, fmt.Errorf("create terminal template: %w", err)
	}
	return r.GetTerminalTemplateTXN(tx, businessID, id)
}

// UpdateTerminalTemplateTXN edits a CUSTOM template. A built-in is refused so
// shipped content cannot be silently rewritten per tenant.
func (r *TerminalTemplateRepository) UpdateTerminalTemplateTXN(tx *sql.Tx, businessID, id string, req *model.TerminalTemplateRequest) (*model.TerminalTemplate, error) {
	existing, err := r.GetTerminalTemplateTXN(tx, businessID, id)
	if err != nil {
		return nil, err
	}
	if existing.IsBuiltin {
		return nil, ErrTerminalTemplateIsBuiltin
	}

	res, err := tx.Exec(`
		UPDATE terminal_templates
		   SET name = $1, description = $2, category = $3, method = $4,
		       path = $5, body = $6, updated_at = now()
		 WHERE id = $7 AND business_id = $8 AND is_builtin = false`,
		req.Name, req.Description, req.Category, req.Method, req.Path, req.Body, id, businessID)
	if err != nil {
		return nil, fmt.Errorf("update terminal template: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrTerminalTemplateNotFound
	}
	return r.GetTerminalTemplateTXN(tx, businessID, id)
}

// DeleteTerminalTemplateTXN removes a CUSTOM template; built-ins are refused.
func (r *TerminalTemplateRepository) DeleteTerminalTemplateTXN(tx *sql.Tx, businessID, id string) error {
	existing, err := r.GetTerminalTemplateTXN(tx, businessID, id)
	if err != nil {
		return err
	}
	if existing.IsBuiltin {
		return ErrTerminalTemplateIsBuiltin
	}

	res, err := tx.Exec(
		`DELETE FROM terminal_templates WHERE id = $1 AND business_id = $2 AND is_builtin = false`,
		id, businessID)
	if err != nil {
		return fmt.Errorf("delete terminal template: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTerminalTemplateNotFound
	}
	return nil
}

// TemplateCategoriesTXN returns the categories present for this business.
func (r *TerminalTemplateRepository) TemplateCategoriesTXN(tx *sql.Tx, businessID string) ([]string, error) {
	rows, err := tx.Query(`SELECT DISTINCT category FROM terminal_templates
		 WHERE business_id = $1 ORDER BY category`, businessID)
	if err != nil {
		return nil, fmt.Errorf("template categories: %w", err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
