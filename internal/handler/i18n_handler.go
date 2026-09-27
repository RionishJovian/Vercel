package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// MaxI18nKeyLen / MaxI18nValueLen bound what an override may store. A UI string
// override is short by nature; an unbounded write here would be replicated to
// every business.
const (
	MaxI18nKeyLen   = 500
	MaxI18nValueLen = 5000
)

// I18nHandler serves translation overrides.
//
// ⚠️ The table is GLOBAL (no business_id, RLS disabled), so reads are open to any
// authenticated user but WRITES are gated to the DEVELOPER platform role: a
// tenant admin editing these would be rewriting every other tenant's UI.
type I18nHandler struct {
	db   *database.DB
	repo *database.I18nRepository
}

// NewI18nHandler wires the translation repository.
func NewI18nHandler(db *database.DB, repo *database.I18nRepository) *I18nHandler {
	return &I18nHandler{db: db, repo: repo}
}

type i18nRequest struct {
	Locale    string `json:"locale"`
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	Value     string `json:"value"`
}

// i18nValidationError carries the reason AND, for a closed-set field, the values
// that would have been accepted. Both are returned to the caller so a bad locale
// is actionable rather than just rejected.
type i18nValidationError struct {
	Message string
	Allowed []string
}

// validate checks the closed sets (locale, namespace) and the free-text fields.
func (req *i18nRequest) validate() *i18nValidationError {
	req.Locale = strings.TrimSpace(req.Locale)
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Key = strings.TrimSpace(req.Key)

	if !model.IsI18nLocale(req.Locale) {
		return &i18nValidationError{"unknown locale", model.I18nLocales}
	}
	if !model.IsI18nNamespace(req.Namespace) {
		return &i18nValidationError{"unknown namespace", model.I18nNamespaces}
	}
	if req.Key == "" {
		return &i18nValidationError{Message: "key is required"}
	}
	if len(req.Key) > MaxI18nKeyLen {
		return &i18nValidationError{Message: "key is too long"}
	}
	// An empty value would replace a live UI string with nothing, which is far
	// more likely a mistake than an intent, so it is rejected rather than stored.
	if strings.TrimSpace(req.Value) == "" {
		return &i18nValidationError{Message: "value is required (an empty value would blank the string)"}
	}
	if len(req.Value) > MaxI18nValueLen {
		return &i18nValidationError{Message: "value is too long"}
	}
	return nil
}

// writeValidationErr emits a 400 in the SAME shape as the query-parameter path, so
// a bad locale looks identical whether it arrived in the body or the query string.
func writeValidationErr(c *gin.Context, v *i18nValidationError) {
	body := gin.H{"error": v.Message}
	if len(v.Allowed) > 0 {
		body["allowed"] = v.Allowed
	}
	c.JSON(http.StatusBadRequest, body)
}

// ListOverrides returns one page of translations plus the per-locale/namespace
// counts. Translate-and-browse is not privileged, so any authenticated role.
func (h *I18nHandler) ListOverrides(c *gin.Context) {
	locale := c.Query("locale")
	ns := c.Query("namespace")

	// Closed sets: an unknown value is a 400 with the allowed list, not an empty
	// page that would read as "there are no translations".
	if locale != "" && !model.IsI18nLocale(locale) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown locale", "allowed": model.I18nLocales})
		return
	}
	if ns != "" && !model.IsI18nNamespace(ns) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown namespace", "allowed": model.I18nNamespaces})
		return
	}

	limit, err := parseBoundedInt(c, "limit", database.DefaultI18nPage, 1, database.MaxI18nPage)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	offset, err := parseBoundedInt(c, "offset", 0, 0, 1_000_000)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	q := c.Query("q")

	var overrides []*model.I18nOverride
	var byLocale, byNS map[string]int
	var total int
	err = runGlobalTx(h.db, func(tx *sql.Tx) error {
		list, n, err := h.repo.ListI18nOverridesTXN(tx, locale, ns, q, limit, offset)
		if err != nil {
			return err
		}
		overrides, total = list, n

		bl, bn, err := h.repo.I18nFacetsTXN(tx)
		if err != nil {
			return err
		}
		byLocale, byNS = bl, bn
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"overrides":    overrides,
		"count":        len(overrides),
		"total":        total,
		"limit":        limit,
		"offset":       offset,
		"has_more":     offset+len(overrides) < total,
		"locales":      model.I18nLocales,
		"namespaces":   model.I18nNamespaces,
		"by_locale":    byLocale,
		"by_namespace": byNS,
		"scope":        "platform", // global table — not tenant-scoped
	})
}

// CreateOverride adds a translation. DEVELOPER only (see the type comment).
func (h *I18nHandler) CreateOverride(c *gin.Context) {
	var req i18nRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if v := req.validate(); v != nil {
		writeValidationErr(c, v)
		return
	}

	actor := auth.GetUserID(c)
	var created *model.I18nOverride
	err := runGlobalTx(h.db, func(tx *sql.Tx) error {
		o, err := h.repo.CreateI18nOverrideTXN(tx, req.Locale, req.Namespace, req.Key, req.Value, &actor)
		if err != nil {
			return err
		}
		created = o
		return nil
	})
	if err != nil {
		h.writeErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"override": created})
}

// UpdateOverride edits a translation. DEVELOPER only.
func (h *I18nHandler) UpdateOverride(c *gin.Context) {
	id := c.Param("id")
	if !isUUID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid override id"})
		return
	}
	var req i18nRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if v := req.validate(); v != nil {
		writeValidationErr(c, v)
		return
	}

	actor := auth.GetUserID(c)
	var updated *model.I18nOverride
	err := runGlobalTx(h.db, func(tx *sql.Tx) error {
		o, err := h.repo.UpdateI18nOverrideTXN(tx, id, req.Locale, req.Namespace, req.Key, req.Value, &actor)
		if err != nil {
			return err
		}
		updated = o
		return nil
	})
	if err != nil {
		h.writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"override": updated})
}

// DeleteOverride removes a translation. DEVELOPER only.
func (h *I18nHandler) DeleteOverride(c *gin.Context) {
	id := c.Param("id")
	if !isUUID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid override id"})
		return
	}
	err := runGlobalTx(h.db, func(tx *sql.Tx) error {
		return h.repo.DeleteI18nOverrideTXN(tx, id)
	})
	if err != nil {
		h.writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// writeErr maps the repository's typed errors onto status codes.
func (h *I18nHandler) writeErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, database.ErrI18nOverrideNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, database.ErrI18nKeyExists):
		c.JSON(http.StatusConflict, gin.H{
			"error": err.Error(),
			"hint":  "that locale/namespace/key already has an override — edit it instead",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
