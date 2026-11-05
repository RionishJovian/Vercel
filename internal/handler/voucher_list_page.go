package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
)

const (
	voucherPageDefaultSize = 100
	voucherPageMaxSize     = 200
	voucherQueryMaxLen     = 64
	voucherCodesMax        = voucherPageMaxSize
)

type voucherPageRequest struct {
	page, perPage int
	filter        database.VoucherListFilter
}

var errVoucherPageRequest = errors.New("invalid voucher page request")

// parseVoucherPageRequest validates the optional paging query of GET /vouchers.
func parseVoucherPageRequest(c *gin.Context) (voucherPageRequest, error) {
	req := voucherPageRequest{page: 1, perPage: voucherPageDefaultSize}
	var err error
	if req.page, err = strconv.Atoi(c.Query("page")); err != nil || req.page < 1 {
		return req, errVoucherPageRequest
	}
	if s := c.Query("per_page"); s != "" {
		if req.perPage, err = strconv.Atoi(s); err != nil || req.perPage < 1 || req.perPage > voucherPageMaxSize {
			return req, errVoucherPageRequest
		}
	}
	f := database.VoucherListFilter{Status: c.Query("status"), Query: strings.TrimSpace(c.Query("q")), BatchID: strings.ToLower(c.Query("batch"))}
	switch f.Status {
	case "", "unused", "active", "used", "revoked", "banned", "expired":
	default:
		return req, errVoucherPageRequest
	}
	if len(f.Query) > voucherQueryMaxLen {
		return req, errVoucherPageRequest
	}
	if f.BatchID != "" && !guestVoucherUUID.MatchString(f.BatchID) {
		return req, errVoucherPageRequest
	}
	if s := c.Query("codes"); s != "" {
		f.Codes = strings.Split(s, ",")
		if len(f.Codes) > voucherCodesMax {
			return req, errVoucherPageRequest
		}
	}
	f.Limit, f.Offset = req.perPage, (req.page-1)*req.perPage
	req.filter = f
	return req, nil
}

// listVouchersPage answers GET /vouchers?page=N. Saved usage and data limits are
// included for the page's rows only.
func (h *VoucherHandler) listVouchersPage(c *gin.Context, businessID string) {
	c.Header("Cache-Control", "no-store")
	req, err := parseVoucherPageRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var page *database.VoucherPage
	var usage map[string]database.GuestVoucherUsage
	var limits map[string]int
	var handouts map[string]time.Time
	var handoutsReady bool
	err = h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		p, err := h.voucherRepo.ListVouchersPageTXN(tx, businessID, req.filter)
		if err != nil {
			return err
		}
		if err := (database.GuestVoucherBatchRepository{}).AnnotateTXN(tx, businessID, p.Vouchers); err != nil {
			return err
		}
		ids := make([]string, 0, len(p.Vouchers))
		for _, v := range p.Vouchers {
			ids = append(ids, v.ID)
		}
		repo := database.GuestVoucherRepository{}
		if usage, err = repo.UsageForVouchersTXN(tx, businessID, ids); err != nil {
			return err
		}
		if limits, err = repo.DataLimitsForVouchersTXN(tx, businessID, ids); err != nil {
			return err
		}
		if handoutsReady, err = h.voucherRepo.HandoutsReadyTXN(tx); err != nil {
			return err
		}
		if handoutsReady {
			if handouts, err = h.voucherRepo.HandoutsForVouchersTXN(tx, businessID, ids); err != nil {
				return err
			}
		}
		page = p
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "voucher records unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"vouchers": page.Vouchers, "total": page.Total, "page": req.page, "per_page": req.perPage,
		"status_counts": page.StatusCounts, "usage": usage, "data_limits_mb": limits,
		"handed_out": handouts, "hand_out_enabled": handoutsReady, "router_delete_enabled": guestActionsEnabled()})
}

// SetVoucherHandout records whether the owner has handed out a code. App data only:
// the router and the voucher's status do not change.
func (h *VoucherHandler) SetVoucherHandout(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	businessID, id := auth.GetBusinessID(c), strings.ToLower(c.Param("id"))
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var body struct {
		HandedOut *bool `json:"handed_out"`
	}
	if !guestVoucherUUID.MatchString(id) || c.ShouldBindJSON(&body) != nil || body.HandedOut == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "voucher and handed_out required"})
		return
	}
	status := http.StatusOK
	message := ""
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		ready, err := h.voucherRepo.HandoutsReadyTXN(tx)
		if err != nil {
			return err
		}
		if !ready {
			status, message = http.StatusServiceUnavailable, "hand out records are not available yet"
			return nil
		}
		v, err := h.voucherRepo.GetVoucherByIDTXN(tx, id)
		if err != nil {
			return err
		}
		if v == nil || v.BusinessID != businessID {
			status, message = http.StatusNotFound, "voucher not found"
			return nil
		}
		if *body.HandedOut && v.Status != "unused" {
			status, message = http.StatusConflict, "only an unused code can be handed out"
			return nil
		}
		return h.voucherRepo.SetHandoutTXN(tx, businessID, id, *body.HandedOut)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "voucher records unavailable"})
		return
	}
	if message != "" {
		c.JSON(status, gin.H{"error": message})
		return
	}
	c.JSON(http.StatusOK, gin.H{"handed_out": *body.HandedOut})
}
