package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/billing"
	"mega-magic/internal/media/bridge"
)

type MediaPurchaseStore interface {
	ResolveMediaPublication(context.Context, string) (string, error)
	PublicPublication(context.Context, string) (bridge.Publication, error)
	ResolveGuestMediaPass(context.Context, string, string, string) (string, error)
	CreateMediaPass(context.Context, string, string, string, string) (bridge.MediaPass, error)
	MediaPass(context.Context, string, string, string) (bridge.MediaPass, error)
	SubmitMediaReceipt(context.Context, string, string, string, string, string) (bridge.MediaReceipt, error)
	MediaReceiptSubmitted(context.Context, string, string, string) (bool, error)
}

type MediaPurchaseHandler struct {
	Store       MediaPurchaseStore
	Ready       bool // schema AND real playback acceptance; never inferred from a theme
	ReceiptRoot string
	Notify      func(context.Context, string, bridge.MediaPass, bridge.MediaReceipt, []byte) error
	ReviewReady func(context.Context, string) bool
}

func (h *MediaPurchaseHandler) guest(c *gin.Context) (string, bool) {
	c.Header("Cache-Control", "no-store")
	if !h.Ready {
		c.JSON(503, gin.H{"error": "Media purchases are not available yet. Do not transfer payment."})
		return "", false
	}
	// No owner/staff cookie or submitted business ID authenticates a guest pass.
	value := c.GetHeader("Authorization")
	if !strings.HasPrefix(value, "Bearer ") {
		c.AbortWithStatus(401)
		return "", false
	}
	hash, e := bridge.Hash(strings.TrimPrefix(value, "Bearer "))
	if e != nil {
		c.AbortWithStatus(401)
		return "", false
	}
	return hash, true
}

func (h *MediaPurchaseHandler) Checkout(c *gin.Context) {
	hash, ok := h.guest(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var body struct {
		Key string `json:"checkout_key"`
	}
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&body) != nil {
		c.Status(400)
		return
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		c.Status(400)
		return
	}
	if _, e := bridge.Hash(body.Key); e != nil {
		c.Status(400)
		return
	}
	ctx := c.Request.Context()
	pubID := c.Param("id")
	bid, e := h.Store.ResolveMediaPublication(ctx, pubID)
	if e != nil {
		c.Status(404)
		return
	}
	if h.ReviewReady == nil || !h.ReviewReady(ctx, bid) {
		c.JSON(409, gin.H{"error": "The owner must configure their Telegram receipt-review bot first."})
		return
	}
	// The store validates receiving details within order creation, while returning
	// an existing checkout's original terms even after the owner edits them.
	pass, e := h.Store.CreateMediaPass(ctx, bid, pubID, hash, body.Key)
	if e != nil {
		c.JSON(409, gin.H{"error": "Purchase could not be created. No access has been granted."})
		return
	}
	c.JSON(200, gin.H{"pass": pass})
}

func (h *MediaPurchaseHandler) Status(c *gin.Context) {
	hash, ok := h.guest(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	id := c.Param("passId")
	bid, e := h.Store.ResolveGuestMediaPass(ctx, c.Param("id"), id, hash)
	if e != nil {
		c.Status(404)
		return
	}
	pass, e := h.Store.MediaPass(ctx, bid, id, hash)
	if e != nil {
		c.Status(404)
		return
	}
	submitted, e := h.Store.MediaReceiptSubmitted(ctx, bid, id, hash)
	if e != nil {
		c.Status(503)
		return
	}
	c.JSON(200, gin.H{"pass": pass, "receipt_submitted": submitted})
}

func (h *MediaPurchaseHandler) Receipt(c *gin.Context) {
	hash, ok := h.guest(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	id := c.Param("passId")
	bid, e := h.Store.ResolveGuestMediaPass(ctx, c.Param("id"), id, hash)
	if e != nil {
		c.Status(404)
		return
	}
	pass, e := h.Store.MediaPass(ctx, bid, id, hash)
	if e != nil || pass.State != "pending" {
		c.Status(409)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, (5<<20)+(256<<10))
	defer func() {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
	}()
	file, e := c.FormFile("receipt")
	if e != nil {
		c.JSON(400, gin.H{"error": "Attach your successful bank/MMQR receipt."})
		return
	}
	f, e := file.Open()
	if e != nil {
		c.Status(400)
		return
	}
	data, e := io.ReadAll(io.LimitReader(f, (5<<20)+1))
	_ = f.Close()
	mime := http.DetectContentType(data)
	if e != nil || len(data) == 0 || len(data) > 5<<20 || (mime != "image/jpeg" && mime != "image/png" && mime != "image/webp") {
		c.JSON(400, gin.H{"error": "Receipt must be JPEG, PNG or WebP, up to 5 MiB."})
		return
	}
	sum := sha256.Sum256(data)
	proof := hex.EncodeToString(sum[:])
	// Each attempt owns its private file, so rejected/duplicate attempts can be
	// cleaned without deleting another request's already-bound evidence.
	nonce, e := bridge.Secret()
	if e != nil {
		c.Status(503)
		return
	}
	path, e := billing.SaveReceipt(h.ReceiptRoot, bid, id+"-"+nonce, data)
	if e != nil {
		c.Status(503)
		return
	}
	receipt, e := h.Store.SubmitMediaReceipt(ctx, bid, id, hash, path, proof)
	if e != nil {
		// Preserve evidence if a database/commit outcome is uncertain. A definite
		// denial means this attempt never bound its newly generated file.
		if errors.Is(e, bridge.ErrPassDenied) {
			_ = os.Remove(path)
		}
		if errors.Is(e, bridge.ErrPassDenied) {
			c.JSON(409, gin.H{"error": "Receipt deadline passed, receipt changed or order was already decided."})
		} else {
			c.Status(503)
		}
		return
	}
	if receipt.Path != path {
		_ = os.Remove(path)
	}
	delivered := h.Notify != nil && h.Notify(ctx, bid, pass, receipt, data) == nil
	c.JSON(200, gin.H{"receipt_submitted": true, "owner_notified": delivered, "state": "pending", "message": "Receipt saved. Access requires the owner's approval."})
}
