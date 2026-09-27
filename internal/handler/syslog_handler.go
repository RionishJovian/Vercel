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

// SyslogHandler serves the ported router event log — a read-only, paged,
// filterable feed.
type SyslogHandler struct {
	db         *database.DB
	syslogRepo *database.SyslogRepository
}

// NewSyslogHandler wires the syslog repository.
func NewSyslogHandler(db *database.DB, syslogRepo *database.SyslogRepository) *SyslogHandler {
	return &SyslogHandler{db: db, syslogRepo: syslogRepo}
}

// ListEvents returns one newest-first page plus the total and the facet values.
func (h *SyslogHandler) ListEvents(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	filter := &model.SyslogFilter{
		Severity: c.Query("severity"),
		Program:  c.Query("program"),
		Query:    c.Query("q"),
	}
	// An invalid severity is a client error, NOT an empty result — otherwise a
	// typo silently looks like "this tenant has no critical events".
	if filter.Severity != "" && !model.IsSyslogSeverity(filter.Severity) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":      "invalid severity",
			"reason":     "severity must be one of info, warning, critical",
			"severities": model.SyslogSeverities,
		})
		return
	}

	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		filter.Limit = n
	}
	if v := c.Query("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid offset"})
			return
		}
		filter.Offset = n
	}

	var events []*model.SyslogEvent
	var total int
	var programs []string
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		list, n, err := h.syslogRepo.ListSyslogEventsTXN(tx, businessID, filter)
		if err != nil {
			return err
		}
		events, total = list, n
		progs, err := h.syslogRepo.SyslogFacetsTXN(tx, businessID)
		if err != nil {
			return err
		}
		programs = progs
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	c.JSON(http.StatusOK, gin.H{
		"events":     events,
		"count":      len(events),
		"total":      total,
		"limit":      limit,
		"offset":     filter.Offset,
		"has_more":   filter.Offset+len(events) < total,
		"programs":   programs,
		"severities": model.SyslogSeverities,
	})
}
