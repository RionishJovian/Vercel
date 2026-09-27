package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// TerminalHistoryHandler serves the log of commands run against routers.
// READ-ONLY: the log is written by the terminal and the insight auto-fixer.
type TerminalHistoryHandler struct {
	db   *database.DB
	repo *database.TerminalHistoryRepository
}

// NewTerminalHistoryHandler wires the terminal history repository.
func NewTerminalHistoryHandler(db *database.DB, repo *database.TerminalHistoryRepository) *TerminalHistoryHandler {
	return &TerminalHistoryHandler{db: db, repo: repo}
}

// ListTerminalHistory returns one page of commands, newest first.
//
// Credential values inside the command and the response are redacted by the
// repository before they reach the response, so no route can leak one.
func (h *TerminalHistoryHandler) ListTerminalHistory(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	source := c.Query("source")
	q := c.Query("q")

	// `status` is an integer column, so any number is syntactically valid and an
	// unknown one honestly yields an empty page. A non-number is a 400.
	status := c.Query("status")
	if status != "" {
		if _, err := strconv.Atoi(status); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "status must be an integer"})
			return
		}
	}

	limit, err := parseBoundedInt(c, "limit", database.DefaultTerminalHistoryPage, 1, database.MaxTerminalHistoryPage)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	offset, err := parseBoundedInt(c, "offset", 0, 0, 1_000_000)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var entries []*model.TerminalHistoryEntry
	var sources []string
	var total int
	err = runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		list, n, err := h.repo.ListTerminalHistoryTXN(tx, businessID, source, status, q, limit, offset)
		if err != nil {
			return err
		}
		entries, total = list, n

		s, err := h.repo.DistinctSourcesTXN(tx, businessID)
		if err != nil {
			return err
		}
		sources = s
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"entries":  entries,
		"count":    len(entries),
		"total":    total,
		"limit":    limit,
		"offset":   offset,
		"has_more": offset+len(entries) < total,
		"sources":  sources,
		"redacted": true,
	})
}
