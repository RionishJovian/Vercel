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

// VoucherResellerHandler manages voucher resellers and the printed-voucher layout.
type VoucherResellerHandler struct {
	db      *database.DB
	resRepo *database.VoucherResellerRepository
}

// NewVoucherResellerHandler wires the reseller repository.
func NewVoucherResellerHandler(db *database.DB, resRepo *database.VoucherResellerRepository) *VoucherResellerHandler {
	return &VoucherResellerHandler{db: db, resRepo: resRepo}
}

// ListResellers returns the business's resellers.
func (h *VoucherResellerHandler) ListResellers(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var resellers []*model.VoucherReseller
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		list, err := h.resRepo.ListResellersTXN(tx, businessID)
		if err != nil {
			return err
		}
		resellers = list
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	active := 0
	for _, r := range resellers {
		if r.Active {
			active++
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"resellers": resellers,
		"count":     len(resellers),
		"active":    active,
		"alphabet":  model.CodePrefixAlphabet,
	})
}

// validatePrefix mirrors the ported CHECK so a bad prefix is a 400 that explains
// itself, instead of a 500 naming the constraint.
func validatePrefix(c *gin.Context, prefix string) bool {
	if !model.IsValidCodePrefix(prefix) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "invalid code prefix",
			"reason":   "2–4 uppercase characters from " + model.CodePrefixAlphabet + " (I, L, O, 0 and 1 are excluded so printed codes cannot be misread)",
			"alphabet": model.CodePrefixAlphabet,
		})
		return false
	}
	return true
}

// CreateReseller adds a reseller (ADMIN+ — enforced at the route).
func (h *VoucherResellerHandler) CreateReseller(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req model.VoucherResellerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.CodePrefix = strings.ToUpper(strings.TrimSpace(req.CodePrefix))
	if !validatePrefix(c, req.CodePrefix) {
		return
	}

	var created *model.VoucherReseller
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		r, err := h.resRepo.CreateResellerTXN(tx, businessID, &req)
		if err != nil {
			return err
		}
		created = r
		return nil
	})
	if err != nil {
		h.respondResellerErr(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"reseller": created})
}

// UpdateReseller edits a reseller (ADMIN+).
func (h *VoucherResellerHandler) UpdateReseller(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id := c.Param("id")
	if !isUUID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid reseller id"})
		return
	}

	var req model.VoucherResellerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.CodePrefix = strings.ToUpper(strings.TrimSpace(req.CodePrefix))
	if !validatePrefix(c, req.CodePrefix) {
		return
	}

	var updated *model.VoucherReseller
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		r, err := h.resRepo.UpdateResellerTXN(tx, businessID, id, &req)
		if err != nil {
			return err
		}
		updated = r
		return nil
	})
	if err != nil {
		h.respondResellerErr(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"reseller": updated})
}

// DeleteReseller removes a reseller (ADMIN+).
func (h *VoucherResellerHandler) DeleteReseller(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id := c.Param("id")
	if !isUUID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid reseller id"})
		return
	}

	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		return h.resRepo.DeleteResellerTXN(tx, businessID, id)
	})
	if err != nil {
		h.respondResellerErr(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *VoucherResellerHandler) respondResellerErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, database.ErrResellerAssigned):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, database.ErrResellerNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "reseller not found"})
	case errors.Is(err, database.ErrResellerPrefixInUse):
		c.JSON(http.StatusConflict, gin.H{
			"error":  "code prefix already in use",
			"reason": "each reseller needs its own prefix, otherwise their voucher codes are indistinguishable",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// defaultPrintLayout is what a business that has never saved a layout sees. The
// values match the ported column defaults, so the UI never shows a blank form.
func defaultPrintLayout(businessName string) *model.VoucherPrintLayout {
	return &model.VoucherPrintLayout{
		BusinessName:        businessName,
		WifiName:            "Hotspot",
		SupportContact:      "Ask the front desk",
		Terms:               "By connecting you agree to our fair-use policy.",
		PaperWidthMM:        80,
		ShowQR:              true,
		ShowPrice:           true,
		ShowExpiry:          true,
		A4Columns:           3,
		A4RowsPerPage:       12,
		ThermalColumns:      2,
		ThermalRowsPerStrip: 8,
	}
}

// GetPrintLayout returns the saved layout, or a computed default with
// saved=false. It never writes on a read.
func (h *VoucherResellerHandler) GetPrintLayout(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var layout *model.VoucherPrintLayout
	var companyName string
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		l, err := h.resRepo.GetPrintLayoutTXN(tx, businessID)
		if err != nil {
			return err
		}
		layout = l
		if l == nil {
			name, err := h.resRepo.BusinessNameTXN(tx, businessID)
			if err != nil {
				return err
			}
			companyName = name
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if layout == nil {
		if companyName == "" {
			companyName = "Your business"
		}
		c.JSON(http.StatusOK, gin.H{"layout": defaultPrintLayout(companyName), "saved": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"layout": layout, "saved": true})
}

// SavePrintLayout upserts the singleton layout (ADMIN+).
func (h *VoucherResellerHandler) SavePrintLayout(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req model.VoucherPrintLayoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var saved *model.VoucherPrintLayout
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		l, err := h.resRepo.UpsertPrintLayoutTXN(tx, businessID, &req)
		if err != nil {
			return err
		}
		saved = l
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"layout": saved, "saved": true})
}
