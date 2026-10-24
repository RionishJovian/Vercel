package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/guestaccess"
	"mega-magic/internal/model"
)

// GuestVoucherIssueBatchLimit bounds a single verified router installation.
const GuestVoucherIssueBatchLimit = 200

var errGuestVoucherBatchMismatch = errors.New("batch identity already has different parameters")
var errGuestVoucherBatchMissing = errors.New("voucher batch not found")

// issueGuestVoucherBatch commits identities before router I/O. A client retains
// its UUID across lost responses and retries; installation failure never creates
// replacement codes or deletes evidence of a possibly successful router write.
func (h *GuestVoucherHandler) issueGuestVoucherBatch(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var b guestVoucherBatch
	if c.ShouldBindJSON(&b) != nil || !b.valid() {
		c.JSON(400, gin.H{"error": "valid gateway, label, time, data and batch count (1-200) required"})
		return
	}
	b.Label = strings.TrimSpace(b.Label)
	b.DeviceID = strings.ToLower(b.DeviceID)
	b.ResellerID = strings.ToLower(b.ResellerID)
	id := strings.ToLower(strings.TrimSpace(b.BatchID))
	key := strings.ToLower(strings.TrimSpace(c.GetHeader("Idempotency-Key")))
	if id == "" {
		id = key
	}
	if !guestVoucherUUID.MatchString(id) || key != "" && key != id {
		c.JSON(400, gin.H{"error": "retain a UUID batch_id or matching Idempotency-Key before creating codes"})
		return
	}
	b.BatchID = id
	biz := auth.GetBusinessID(c)
	if biz == "" {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	if h.DB == nil || h.Vouchers == nil || h.Provisioner == nil {
		c.JSON(503, gin.H{"error": "gateway voucher installation unavailable"})
		return
	}
	// Do not hash the key itself: only immutable normalized purchase parameters.
	parameters, _ := json.Marshal(struct {
		DeviceID, Label                  string
		PriceMMK, Minutes, DataMB, Count int
		ResellerID                       string `json:",omitempty"`
	}{b.DeviceID, b.Label, b.PriceMMK, b.Minutes, b.DataMB, b.Count, b.ResellerID})
	sum := sha256.Sum256(parameters)
	batch := database.GuestVoucherIssueBatch{ID: id, BusinessID: biz, DeviceID: b.DeviceID,
		RequestHash: hex.EncodeToString(sum[:]), Label: b.Label, PriceMMK: b.PriceMMK,
		Minutes: b.Minutes, DataMB: b.DataMB, TotalCount: b.Count}
	repo := database.GuestVoucherBatchRepository{}
	err := runInTenant(c, h.DB, biz, func(tx *sql.Tx) error {
		if err := h.checkIssueGatewayTXN(tx, biz, b.DeviceID); err != nil {
			return err
		}
		created, err := repo.CreateTXN(tx, batch)
		if err != nil {
			return err
		}
		saved, err := repo.GetTXN(tx, biz, id, true)
		if err != nil {
			return err
		}
		if saved == nil || saved.RequestHash != batch.RequestHash {
			return errGuestVoucherBatchMismatch
		}
		if !created {
			return nil
		}
		if b.ResellerID != "" {
			if _, err := database.AssignGuestBatchResellerTXN(tx, biz, id, b.ResellerID, "", auth.GetUserID(c)); err != nil {
				return err
			}
		}
		for i := 0; i < b.Count; i++ {
			v, err := h.Vouchers.createOneVoucher(tx, biz, "guest-internet", b.Label, b.PriceMMK,
				&b.Minutes, nil, &b.DeviceID, "unused", nil, time.Now().UTC())
			if err != nil {
				return err
			}
			if err := (database.GuestVoucherRepository{}).IssueTXN(tx, biz, v.ID, b.DeviceID, b.Minutes, b.DataMB); err != nil {
				return err
			}
			if err := repo.AddItemTXN(tx, biz, id, v.ID, i); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errGuestVoucherBatchMismatch) {
			c.JSON(409, gin.H{"error": "this batch_id belongs to different voucher settings; retry its original batch or start a new batch"})
		} else {
			c.JSON(409, gin.H{"error": "guest voucher batch could not be saved for this gateway", "batch_id": id})
		}
		return
	}
	h.installGuestVoucherBatch(c, biz, id)
	h.respondGuestVoucherBatch(c, biz, id, http.StatusCreated)
}

func (h *GuestVoucherHandler) checkIssueGatewayTXN(tx *sql.Tx, biz, deviceID string) error {
	d, err := database.NewDeviceRepository(h.DB).GetDeviceByIDTXN(tx, deviceID)
	if err != nil {
		return err
	}
	if d == nil || d.BusinessID != biz || d.DeploymentLayer != "MAIN_GATEWAY" {
		return guestaccess.ErrProvisionDenied
	}
	return nil
}

func (h *GuestVoucherHandler) installGuestVoucherBatch(c *gin.Context, biz, id string) {
	repo := database.GuestVoucherBatchRepository{}
	var tokenBytes [16]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return
	}
	token := hex.EncodeToString(tokenBytes[:])
	var batch *database.GuestVoucherIssueBatch
	var items []database.GuestVoucherIssueItem
	claimed := false
	err := runInTenant(c, h.DB, biz, func(tx *sql.Tx) error {
		var err error
		batch, err = repo.GetTXN(tx, biz, id, true)
		if err != nil || batch == nil {
			return err
		}
		if err := h.checkIssueGatewayTXN(tx, biz, batch.DeviceID); err != nil {
			return err
		}
		items, err = repo.ItemsTXN(tx, biz, id)
		if err != nil || len(items) != batch.TotalCount {
			return database.ErrGuestVoucherUnavailable
		}
		for _, item := range items {
			if item.Voucher.DeviceID == nil || *item.Voucher.DeviceID != batch.DeviceID ||
				item.Voucher.Status == "revoked" || item.Voucher.Status == "banned" || item.Voucher.Status == "expired" {
				return database.ErrGuestVoucherUnavailable
			}
		}
		claimed, err = repo.ClaimTXN(tx, biz, id, token)
		return err
	})
	if err != nil || !claimed {
		return
	}
	specs := make([]VoucherLoginSpec, 0, len(items))
	for _, item := range items {
		specs = append(specs, VoucherLoginSpec{VoucherID: item.Voucher.ID, Code: item.Voucher.Code,
			Minutes: batch.Minutes, DataMB: batch.DataMB})
	}
	// One transport session per batch. The router may have accepted some or all
	// users before an error; retry must inspect these same UUID-owned users.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	installErr := h.Provisioner.ProvisionVoucherLogins(ctx, biz, batch.DeviceID, specs)
	cancel()
	// A disconnected HTTP client must not prevent us recording uncertainty.
	// A fresh tenant-bound transaction is bounded and never releases the IDs.
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finishCancel()
	tx, err := h.DB.BeginTx(finishCtx, nil)
	if err != nil {
		return // lease expiry makes this saved batch retryable.
	}
	defer tx.Rollback()
	if err := database.WithTenant(tx, biz, func() error {
		return repo.FinishTXN(tx, biz, id, token, installErr == nil)
	}); err != nil {
		return
	}
	_ = tx.Commit() // An uncertain commit remains discoverable by batch_id.
}

func (h *GuestVoucherHandler) GetIssueBatch(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	biz, id, ok := h.issueBatchRequest(c)
	if ok {
		h.respondGuestVoucherBatch(c, biz, id, http.StatusOK)
	}
}

func (h *GuestVoucherHandler) RetryIssueBatch(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	biz, id, ok := h.issueBatchRequest(c)
	if !ok {
		return
	}
	if h.Provisioner == nil {
		c.JSON(503, gin.H{"error": "gateway voucher installation unavailable"})
		return
	}
	h.installGuestVoucherBatch(c, biz, id)
	h.respondGuestVoucherBatch(c, biz, id, http.StatusOK)
}

func (h *GuestVoucherHandler) issueBatchRequest(c *gin.Context) (string, string, bool) {
	biz := auth.GetBusinessID(c)
	id := strings.ToLower(c.Param("batchId"))
	if biz == "" {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return "", "", false
	}
	if !guestVoucherUUID.MatchString(id) {
		c.JSON(400, gin.H{"error": "valid batch_id required"})
		return "", "", false
	}
	if h.DB == nil {
		c.JSON(503, gin.H{"error": "voucher batch records unavailable"})
		return "", "", false
	}
	return biz, id, true
}

func (h *GuestVoucherHandler) ListIssueBatches(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	biz := auth.GetBusinessID(c)
	if biz == "" {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	if h.DB == nil {
		c.JSON(503, gin.H{"error": "voucher batch records unavailable"})
		return
	}
	var batches []database.GuestVoucherIssueSummary
	var stock []database.GuestBatchStock
	err := runInTenant(c, h.DB, biz, func(tx *sql.Tx) error {
		var err error
		batches, err = (database.GuestVoucherBatchRepository{}).ListIncompleteTXN(tx, biz)
		if err != nil {
			return err
		}
		stock, err = database.ListGuestBatchStockTXN(tx, biz)
		return err
	})
	if err != nil {
		c.JSON(503, gin.H{"error": "voucher batch records unavailable"})
		return
	}
	c.JSON(200, gin.H{"batches": batches, "stock_batches": stock, "reseller_assignment_enabled": true, "limit": 100})
}

func (h *GuestVoucherHandler) respondGuestVoucherBatch(c *gin.Context, biz, id string, readyStatus int) {
	var b *database.GuestVoucherIssueBatch
	var items []database.GuestVoucherIssueItem
	err := runInTenant(c, h.DB, biz, func(tx *sql.Tx) error {
		repo := database.GuestVoucherBatchRepository{}
		var err error
		b, err = repo.GetTXN(tx, biz, id, false)
		if err != nil {
			return err
		}
		if b == nil {
			return errGuestVoucherBatchMissing
		}
		items, err = repo.ItemsTXN(tx, biz, id)
		return err
	})
	if err != nil {
		if errors.Is(err, errGuestVoucherBatchMissing) {
			c.JSON(404, gin.H{"error": "voucher batch not found"})
		} else {
			c.JSON(503, gin.H{"error": "batch outcome could not be read; retry this batch_id", "batch_id": id})
		}
		return
	}
	verified, state := 0, "pending"
	metadata := make([]gin.H, 0, len(items))
	vouchers := make([]*model.Voucher, 0, len(items))
	for _, item := range items {
		metadata = append(metadata, gin.H{"voucher_id": item.Voucher.ID, "install_state": item.InstallState})
		if item.InstallState == "verified" {
			verified++
		} else if item.InstallState == "unknown" {
			state = "unknown"
		}
	}
	status := http.StatusAccepted
	if verified == b.TotalCount && len(items) == b.TotalCount {
		state, status = "ready", readyStatus
		for _, item := range items {
			vouchers = append(vouchers, item.Voucher)
		}
	} else if b.LeaseUntil != nil && time.Now().Before(*b.LeaseUntil) {
		state = "installing"
	}
	// GET is a successful observation even while installation remains pending.
	if c.Request.Method == http.MethodGet {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{"batch_id": b.ID, "device_id": b.DeviceID, "state": state,
		"total_count": b.TotalCount, "verified_count": verified, "created_at": b.CreatedAt,
		"items": metadata, "vouchers": vouchers, "product": "guest-internet", "data_mb": b.DataMB,
		"validity_starts": "first_use", "retry_url": "/guest-vouchers/batches/" + b.ID + "/retry"})
}
