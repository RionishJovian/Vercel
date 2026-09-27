package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// UnlockKeyHandler serves the key-activation ledger. READ-ONLY: keys are created by
// the purchase flow that debits Magic Coins, never by hand.
type UnlockKeyHandler struct {
	db   *database.DB
	repo *database.UnlockKeyRepository
}

// NewUnlockKeyHandler wires the unlock key repository.
func NewUnlockKeyHandler(db *database.DB, repo *database.UnlockKeyRepository) *UnlockKeyHandler {
	return &UnlockKeyHandler{db: db, repo: repo}
}

// ListUnlockKeys returns one page of activations plus the tenant-wide summary.
func (h *UnlockKeyHandler) ListUnlockKeys(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	kind := c.Query("kind")
	q := c.Query("q")
	status := c.Query("status")

	// Status is derived, so an unknown value is a 400 with the allowed set rather
	// than an empty page — the same reasoning as syslog severity. An empty page
	// would claim "this business has no keys", which is a different statement.
	if status != "" && !model.IsUnlockStatus(status) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "unknown status",
			"allowed": model.UnlockStatuses,
		})
		return
	}

	limit, err := parseBoundedInt(c, "limit", database.DefaultUnlockKeyPage, 1, database.MaxUnlockKeyPage)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	offset, err := parseBoundedInt(c, "offset", 0, 0, 1_000_000)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var keys []*model.UnlockKeyActivation
	var summary *model.UnlockKeySummary
	var total int
	err = runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		list, n, err := h.repo.ListUnlockKeysTXN(tx, businessID, kind, status, q, limit, offset)
		if err != nil {
			return err
		}
		keys, total = list, n

		s, err := h.repo.UnlockKeySummaryTXN(tx, businessID)
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

	kinds := []string{}
	for _, g := range summary.ByKind {
		kinds = append(kinds, g.Name)
	}

	c.JSON(http.StatusOK, gin.H{
		"keys":     keys,
		"count":    len(keys),
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": offset+len(keys) < total,
		"kinds":    kinds,
		"statuses": model.UnlockStatuses,
		"summary":  summary,
	})
}
