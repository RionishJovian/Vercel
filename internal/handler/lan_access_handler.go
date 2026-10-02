package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/lanaccess"
	"mega-magic/internal/model"
)

// LANScopeResolver must resolve gateway ownership, active plan, fresh verified
// LAN inventory and protected platform networks from server-owned data. Request
// JSON is never an inventory source. Routes remain unmounted until this resolver
// and a separate-origin device access frontend are available.
type LANScopeResolver interface {
	ResolveLANScope(context.Context, string, string, string) (lanaccess.Scope, error)
}

type LANAccessHandler struct {
	Sessions *lanaccess.Sessions
	Scopes   LANScopeResolver
}

func (h *LANAccessHandler) identity(c *gin.Context) (lanaccess.Scope, bool) {
	c.Header("Cache-Control", "no-store")
	s := lanaccess.Scope{BusinessID: auth.GetBusinessID(c), GatewayID: c.Param("deviceId"), UserID: auth.GetUserID(c), Role: auth.GetUserRole(c)}
	if s.BusinessID == "" || s.UserID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return s, false
	}
	if !model.HasAtLeastRole(model.UserRole(s.Role), model.RoleAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"error": "gateway management permission required"})
		return s, false
	}
	if h.Sessions == nil || s.GatewayID == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "temporary LAN access is unavailable"})
		return s, false
	}
	return s, true
}

func (h *LANAccessHandler) Create(c *gin.Context) {
	id, ok := h.identity(c)
	if !ok {
		return
	}
	if h.Scopes == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "verified LAN inventory is unavailable"})
		return
	}
	var request struct {
		TargetIP        string `json:"target_ip"`
		Port            int    `json:"port"`
		DurationMinutes int    `json:"duration_minutes"`
		Confirm         bool   `json:"confirm_access"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || !request.Confirm || request.DurationMinutes < 1 || request.DurationMinutes > 60 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirm an exact LAN address, port and duration of 1–60 minutes"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	scope, err := h.Scopes.ResolveLANScope(ctx, id.BusinessID, id.GatewayID, id.UserID)
	if err != nil || ctx.Err() != nil || scope.BusinessID != id.BusinessID || scope.GatewayID != id.GatewayID || scope.UserID != id.UserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "verified gateway LAN access is unavailable"})
		return
	}
	// The current request's authenticated role always overrides stale inventory.
	scope.Role = id.Role
	sessionID, grant, err := h.Sessions.Create(scope, lanaccess.Request{TargetIP: request.TargetIP, Port: request.Port, Duration: time.Duration(request.DurationMinutes) * time.Minute})
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "LAN destination or session limit denied"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"session_id": sessionID, "target_ip": grant.Target.String(), "port": grant.Port, "expires_at": grant.ExpiresAt, "state": "authorized", "connectivity_verified": false})
}

func (h *LANAccessHandler) Revoke(c *gin.Context) {
	id, ok := h.identity(c)
	if !ok {
		return
	}
	if h.Sessions.Revoke(id, c.Param("sessionId")) != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "LAN session not found"})
		return
	}
	c.Status(http.StatusNoContent)
}
