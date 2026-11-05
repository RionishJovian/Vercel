package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

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
		page = p
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "voucher records unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"vouchers": page.Vouchers, "total": page.Total, "page": req.page, "per_page": req.perPage,
		"status_counts": page.StatusCounts, "usage": usage, "data_limits_mb": limits})
}
