package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// FleetScanRunHandler serves generated fleet scans. READ-ONLY: scans are produced
// by the platform's scan engine, so there is deliberately no create/update/delete
// route — inventing one would mean inventing a scan engine.
type FleetScanRunHandler struct {
	db       *database.DB
	scanRepo *database.FleetScanRunRepository
}

// NewFleetScanRunHandler wires the fleet scan repository.
func NewFleetScanRunHandler(db *database.DB, scanRepo *database.FleetScanRunRepository) *FleetScanRunHandler {
	return &FleetScanRunHandler{db: db, scanRepo: scanRepo}
}

// ListScanRuns returns one page of scans, newest first.
func (h *FleetScanRunHandler) ListScanRuns(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	kind := c.Query("kind")
	if kind != "" && !model.IsScanRunKind(kind) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid kind", "kinds": model.ScanRunKinds,
		})
		return
	}
	severity := c.Query("severity")
	if severity != "" && !model.IsScanRunSeverity(severity) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid severity", "severities": model.ScanRunSeverities,
		})
		return
	}
	findingsOnly := c.Query("findings_only") == "1" || c.Query("findings_only") == "true"

	limit, err := parseBoundedInt(c, "limit", database.DefaultScanPage, 1, database.MaxScanPage)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	offset, err := parseBoundedInt(c, "offset", 0, 0, 1_000_000)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var runs []*model.FleetScanRun
	var total, crashes int
	var sevCounts, kindCounts map[string]int
	err = runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		list, n, err := h.scanRepo.ListScanRunsTXN(tx, businessID, kind, severity, findingsOnly, limit, offset)
		if err != nil {
			return err
		}
		runs, total = list, n

		if sevCounts, err = h.scanRepo.ScanSeverityCountsTXN(tx, businessID); err != nil {
			return err
		}
		if kindCounts, err = h.scanRepo.ScanKindCountsTXN(tx, businessID); err != nil {
			return err
		}
		crashes, err = h.scanRepo.CrashArtifactCountTXN(tx, businessID)
		return err
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Error findings inside the returned page.
	pageErrors := 0
	for _, r := range runs {
		pageErrors += r.ErrorFindings
	}

	c.JSON(http.StatusOK, gin.H{
		"scans":               runs,
		"count":               len(runs),
		"total":               total,
		"limit":               limit,
		"offset":              offset,
		"has_more":            offset+len(runs) < total,
		"kinds":               model.ScanRunKinds,
		"severities":          model.ScanRunSeverities,
		"severity_counts":     sevCounts,
		"kind_counts":         kindCounts,
		"page_error_findings": pageErrors,
		// Runs whose payload contains a persisted program error. Exposed so the
		// console can distinguish crash artefacts from real fleet findings.
		"crash_artifact_scans": crashes,
	})
}

// GetScanRun returns one scan including its raw per-router snapshot.
func (h *FleetScanRunHandler) GetScanRun(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id := c.Param("id")
	if !isUUID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scan id"})
		return
	}

	var run *model.FleetScanRun
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		r, err := h.scanRepo.GetScanRunTXN(tx, businessID, id)
		if err != nil {
			return err
		}
		run = r
		return nil
	})
	if err != nil {
		if errors.Is(err, database.ErrScanRunNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "scan not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"scan": run})
}

// parseBoundedInt reads a bounded integer query parameter. Shared by the ported
// list endpoints (fleet scans, AI usage).
func parseBoundedInt(c *gin.Context, name string, def, min, max int) (int, error) {
	raw := c.Query(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("invalid " + name + ": must be an integer")
	}
	if n < min || n > max {
		return 0, errors.New("invalid " + name + ": must be between " +
			strconv.Itoa(min) + " and " + strconv.Itoa(max))
	}
	return n, nil
}
