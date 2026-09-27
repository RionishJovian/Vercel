package handler

import (
	"database/sql"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
)

// PulseHandler serves the tenant-scoped "fleet pulse" aggregate (Stage 1B): one
// glance answers "what needs me today" without the UI stitching 7 tiles. It
// fans out the EXISTING tenant-scoped repos — no new service, no heavy query.
type PulseHandler struct {
	db        *database.DB
	devices   *database.DeviceRepository
	incidents *database.IncidentRepository
}

func NewPulseHandler(db *database.DB, devices *database.DeviceRepository, incidents *database.IncidentRepository) *PulseHandler {
	return &PulseHandler{db: db, devices: devices, incidents: incidents}
}

// Pulse: GET /api/v1/fleet/pulse
func (h *PulseHandler) Pulse(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var devRows []*deviceLite
	var incRows []incidentLite

	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		ds, e := h.devices.ListDevicesByBusinessTXN(tx, businessID)
		if e != nil {
			return e
		}
		for _, d := range ds {
			devRows = append(devRows, &deviceLite{ID: d.DeviceID, Name: d.HardwareName, Online: d.IsOnline})
		}
		is, e := h.incidents.ListOpenByBusinessTXN(tx, businessID)
		if e != nil {
			return e
		}
		for _, in := range is {
			var dev *string
			if in.DeviceID != nil {
				dev = in.DeviceID
			}
			incRows = append(incRows, incidentLite{
				ID: in.ID, Kind: in.Kind, Severity: in.Severity,
				SubjectLabel: in.SubjectLabel, DeviceID: dev, OpenedAt: in.OpenedAt,
			})
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	online := 0
	for _, d := range devRows {
		if d.Online {
			online++
		}
	}

	// Most severe first, then most recent — the operator's queue.
	sort.SliceStable(incRows, func(i, j int) bool {
		ri, rj := sevRank(incRows[i].Severity), sevRank(incRows[j].Severity)
		if ri != rj {
			return ri < rj
		}
		return incRows[i].OpenedAt.After(incRows[j].OpenedAt)
	})

	head := incRows
	if len(head) > 3 {
		head = head[:3]
	}
	var attention []incidentLite
	if len(incRows) > 0 {
		attention = []incidentLite{incRows[0]}
	}

	c.JSON(http.StatusOK, gin.H{
		"kpis": gin.H{
			"devices":        len(devRows),
			"online":         online,
			"offline":        len(devRows) - online,
			"open_incidents": len(incRows),
		},
		"open_incidents_head": head,
		"attention":           attention,
	})
}

type deviceLite struct {
	ID     string
	Name   string
	Online bool
}

type incidentLite struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	Severity     string    `json:"severity"`
	SubjectLabel string    `json:"subject_label"`
	DeviceID     *string   `json:"device_id,omitempty"`
	OpenedAt     time.Time `json:"opened_at"`
}

// sevRank orders severities for the attention queue (lower = more urgent).
func sevRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning", "warn":
		return 1
	default:
		return 2
	}
}
