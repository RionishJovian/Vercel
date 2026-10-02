package handler

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/media/bridge"
	"net/http"
	"strings"
	"unicode"
)

type MediaHandler struct{ store bridge.Store }

func NewMediaHandler(store bridge.Store) *MediaHandler { return &MediaHandler{store: store} }
func mediaBody(c *gin.Context, v any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	if e := c.ShouldBindJSON(v); e != nil {
		c.JSON(400, gin.H{"error": "invalid media request"})
		return false
	}
	return true
}
func mediaFailure(c *gin.Context) {
	c.JSON(409, gin.H{"error": "Media connection or request is unavailable. Refresh its status before trying again."})
}
func (h *MediaHandler) Pair(c *gin.Context) {
	secret, e := bridge.Secret()
	if e != nil {
		mediaFailure(c)
		return
	}
	hash, _ := bridge.Hash(secret)
	if e = h.store.Pair(c.Request.Context(), auth.GetBusinessID(c), hash); e != nil {
		mediaFailure(c)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(201, gin.H{"pairing_code": secret, "expires_in_seconds": 600})
}
func (h *MediaHandler) Connection(c *gin.Context) {
	v, e := h.store.Connection(c.Request.Context(), auth.GetBusinessID(c))
	if e != nil {
		mediaFailure(c)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, v)
}
func (h *MediaHandler) Revoke(c *gin.Context) {
	if e := h.store.Revoke(c.Request.Context(), auth.GetBusinessID(c)); e != nil {
		mediaFailure(c)
		return
	}
	c.Status(204)
}
func (h *MediaHandler) Enqueue(c *gin.Context) {
	var j bridge.Job
	if !mediaBody(c, &j) {
		return
	}
	if !bridge.UUID.MatchString(j.ID) {
		c.JSON(400, gin.H{"error": "invalid request ID"})
		return
	}
	if len(j.Payload) == 0 {
		j.Payload = json.RawMessage(`{}`)
	}
	if e := bridge.ValidateOperation(j.Operation, j.Payload); e != nil {
		c.JSON(400, gin.H{"error": e.Error()})
		return
	}
	if e := h.store.Enqueue(c.Request.Context(), auth.GetBusinessID(c), auth.GetUserID(c), j); e != nil {
		mediaFailure(c)
		return
	}
	c.JSON(202, gin.H{"id": j.ID, "state": "queued"})
}
func (h *MediaHandler) Job(c *gin.Context) {
	id := c.Param("id")
	if !bridge.UUID.MatchString(id) {
		c.Status(404)
		return
	}
	j, e := h.store.GetJob(c.Request.Context(), auth.GetBusinessID(c), id)
	if e != nil {
		mediaFailure(c)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, j)
}

// Connector endpoints do not accept browser cookies or business IDs. Their only
// identity is a secret token/code; the store resolves the tenant and suspension.
func (h *MediaHandler) Redeem(c *gin.Context) {
	var body struct {
		Code string `json:"pairing_code"`
		Name string `json:"server_name"`
	}
	if !mediaBody(c, &body) {
		return
	}
	hash, e := bridge.Hash(body.Code)
	if e != nil {
		c.Status(401)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if len(body.Name) > 120 || strings.IndexFunc(body.Name, unicode.IsControl) >= 0 {
		c.JSON(400, gin.H{"error": "invalid server name"})
		return
	}
	token, e := bridge.Secret()
	if e != nil {
		mediaFailure(c)
		return
	}
	tokenHash, _ := bridge.Hash(token)
	if e = h.store.Redeem(c.Request.Context(), hash, tokenHash, body.Name); e != nil {
		c.Status(401)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(201, gin.H{"connector_token": token, "expires_in_seconds": 86400})
}
func connectorHash(c *gin.Context) (string, bool) {
	raw := c.GetHeader("Authorization")
	if !strings.HasPrefix(raw, "Bearer ") {
		c.Status(401)
		return "", false
	}
	hash, e := bridge.Hash(strings.TrimPrefix(raw, "Bearer "))
	if e != nil {
		c.Status(401)
		return "", false
	}
	return hash, true
}
func (h *MediaHandler) Poll(c *gin.Context) {
	hash, ok := connectorHash(c)
	if !ok {
		return
	}
	job, e := h.store.Poll(c.Request.Context(), hash)
	if e != nil {
		c.Status(401)
		return
	}
	c.Header("Cache-Control", "no-store")
	if job == nil {
		c.Status(204)
		return
	}
	c.JSON(200, job)
}
func (h *MediaHandler) Complete(c *gin.Context) {
	hash, ok := connectorHash(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if !bridge.UUID.MatchString(id) {
		c.Status(400)
		return
	}
	var body bridge.Completion
	if !mediaBody(c, &body) {
		return
	}
	switch body.ErrorCode {
	case "", "local_server_unavailable", "local_credential_denied", "invalid_request", "operation_failed":
	default:
		c.Status(400)
		return
	}
	if e := h.store.Complete(c.Request.Context(), hash, id, body); e != nil {
		mediaFailure(c)
		return
	}
	c.Status(204)
}
