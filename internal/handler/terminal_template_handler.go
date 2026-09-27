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

// TerminalTemplateHandler serves the saved RouterOS REST call library.
type TerminalTemplateHandler struct {
	db           *database.DB
	templateRepo *database.TerminalTemplateRepository
}

// NewTerminalTemplateHandler wires the terminal template repository.
func NewTerminalTemplateHandler(db *database.DB, templateRepo *database.TerminalTemplateRepository) *TerminalTemplateHandler {
	return &TerminalTemplateHandler{db: db, templateRepo: templateRepo}
}

// ListTemplates returns the library with optional category/search filters.
func (h *TerminalTemplateHandler) ListTemplates(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	category := c.Query("category")
	query := c.Query("q")

	var templates []*model.TerminalTemplate
	var categories []string
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		list, err := h.templateRepo.ListTerminalTemplatesTXN(tx, businessID, category, query)
		if err != nil {
			return err
		}
		templates = list
		cats, err := h.templateRepo.TemplateCategoriesTXN(tx, businessID)
		if err != nil {
			return err
		}
		categories = cats
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	builtin := 0
	for _, t := range templates {
		if t.IsBuiltin {
			builtin++
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"templates":  templates,
		"count":      len(templates),
		"builtin":    builtin,
		"custom":     len(templates) - builtin,
		"categories": categories,
		"methods":    model.TerminalTemplateMethods,
	})
}

// validateTemplate mirrors the DB CHECK constraints so a bad value is a 400
// rather than a 500 surfacing the constraint name.
func validateTemplate(c *gin.Context, req *model.TerminalTemplateRequest) bool {
	if !model.IsTerminalTemplateMethod(strings.ToUpper(req.Method)) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid method",
			"reason":  "method must be one of " + joinKinds(model.TerminalTemplateMethods),
			"methods": model.TerminalTemplateMethods,
		})
		return false
	}
	req.Method = strings.ToUpper(req.Method)
	if !strings.HasPrefix(req.Path, "/") {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "invalid path",
			"reason": "path must start with / (e.g. /ip/arp)",
		})
		return false
	}
	return true
}

// CreateTemplate adds a CUSTOM template (ADMIN+ — enforced at the route).
func (h *TerminalTemplateHandler) CreateTemplate(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	userID := auth.GetUserID(c)
	if businessID == "" || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req model.TerminalTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !validateTemplate(c, &req) {
		return
	}

	var created *model.TerminalTemplate
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		t, err := h.templateRepo.CreateTerminalTemplateTXN(tx, businessID, userID, &req)
		if err != nil {
			return err
		}
		created = t
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"template": created})
}

// UpdateTemplate edits a custom template; built-ins are refused (ADMIN+).
func (h *TerminalTemplateHandler) UpdateTemplate(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id := c.Param("id")
	if !isUUID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template id"})
		return
	}

	var req model.TerminalTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !validateTemplate(c, &req) {
		return
	}

	var updated *model.TerminalTemplate
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		t, err := h.templateRepo.UpdateTerminalTemplateTXN(tx, businessID, id, &req)
		if err != nil {
			return err
		}
		updated = t
		return nil
	})
	if err != nil {
		h.respondTemplateErr(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"template": updated})
}

// DeleteTemplate removes a custom template; built-ins are refused (ADMIN+).
func (h *TerminalTemplateHandler) DeleteTemplate(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id := c.Param("id")
	if !isUUID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template id"})
		return
	}

	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		return h.templateRepo.DeleteTerminalTemplateTXN(tx, businessID, id)
	})
	if err != nil {
		h.respondTemplateErr(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *TerminalTemplateHandler) respondTemplateErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, database.ErrTerminalTemplateNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
	case errors.Is(err, database.ErrTerminalTemplateIsBuiltin):
		// 403, not 409: the caller is not permitted to modify shipped content.
		c.JSON(http.StatusForbidden, gin.H{
			"error":  "built-in template is read-only",
			"reason": "shipped templates cannot be edited or deleted; create your own copy instead",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
