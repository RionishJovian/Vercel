package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// AIUsageHandler serves the platform's AI call history. READ-ONLY: these rows are
// written by the AI features themselves, so there is no create route.
type AIUsageHandler struct {
	db        *database.DB
	usageRepo *database.AIUsageRepository
}

// NewAIUsageHandler wires the AI usage repository.
func NewAIUsageHandler(db *database.DB, usageRepo *database.AIUsageRepository) *AIUsageHandler {
	return &AIUsageHandler{db: db, usageRepo: usageRepo}
}

// ListUsageEvents returns one page of AI calls plus the tenant-wide dashboard.
//
// `feature` and `model` are free-text and unvalidated on purpose: there is no
// enumerated set to check against, so an unknown value honestly yields an empty
// page. Only `limit`/`offset` are bounded.
func (h *AIUsageHandler) ListUsageEvents(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	feature := c.Query("feature")
	modelName := c.Query("model")

	limit, err := parseBoundedInt(c, "limit", database.DefaultAIUsagePage, 1, database.MaxAIUsagePage)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	offset, err := parseBoundedInt(c, "offset", 0, 0, 1_000_000)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var events []*model.AIUsageEvent
	var summary *model.AIUsageSummary
	var total int
	err = runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		list, n, err := h.usageRepo.ListAIUsageEventsTXN(tx, businessID, feature, modelName, limit, offset)
		if err != nil {
			return err
		}
		events, total = list, n

		s, err := h.usageRepo.AIUsageSummaryTXN(tx, businessID)
		if err != nil {
			return err
		}
		summary = s
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"events":   events,
		"count":    len(events),
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": offset+len(events) < total,
		"summary":  summary,
	})
}
