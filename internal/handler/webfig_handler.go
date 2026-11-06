package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/hub"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/security"
	"mega-magic/internal/webfig"
)

// webFigRouter is the live router side. Reads change nothing; the two writes run
// only after the owner has reviewed the plan in the console.
type webFigRouter interface {
	WebFigAccessState(ctx context.Context, businessID, deviceID, tunnel, sourceCIDR string) (routeros.WebFigAccess, error)
	EnableWebFigAccess(ctx context.Context, businessID, deviceID, tunnel, sourceCIDR string) (routeros.WebFigAccess, error)
	DisableWebFigAccess(ctx context.Context, businessID, deviceID, tunnel, sourceCIDR string) (routeros.WebFigAccess, error)
}

// WebFigControl is cloud WebFig's console side. Every lookup carries the signed-in
// tenant, so another tenant's device id reads as not found.
type WebFigControl struct {
	router     webFigRouter
	targets    routeros.TargetSource
	store      *webfig.Store
	host       string
	sourceCIDR string
	record     func(c *gin.Context, businessID, deviceID, action, reason string)
}

// webFigConfirm is what the console sends after showing the plan.
const webFigConfirm = "ALLOW_WEBFIG"

// EnableWebFig attaches the control. Until this is called every route says off.
func (h *SecurityHandler) EnableWebFig(router webFigRouter, targets routeros.TargetSource, store *webfig.Store, host, sourceCIDR string) {
	h.webFig = &WebFigControl{router: router, targets: targets, store: store, host: host, sourceCIDR: sourceCIDR, record: func(c *gin.Context, businessID, deviceID, action, reason string) {
		err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
			return h.deviceRepo.RecordChangeTXN(tx, businessID, deviceID, auth.GetUserRole(c), action, reason)
		})
		if err != nil {
			log.Printf("webfig: audit %s for device %s failed: %v", action, deviceID, err)
		}
	}}
}

// RegisterWebFigRoutes declares the routes and their role in one place.
func RegisterWebFigRoutes(routers *gin.RouterGroup, h *SecurityHandler) {
	admin := auth.RequireMinRole(model.RoleAdmin)
	routers.GET("/:id/webfig", admin, h.WebFigStatus)
	routers.POST("/:id/webfig/access", admin, h.WebFigAllow)
	routers.DELETE("/:id/webfig/access", admin, h.WebFigRemove)
	routers.POST("/:id/webfig/session", admin, h.WebFigOpen)
}

// webFigGate answers the switch and the tenant check. It writes the refusal.
func (h *SecurityHandler) webFigGate(c *gin.Context) (*WebFigControl, string, string, string, bool) {
	control, mode := h.webFig, webfig.ModeFromEnv()
	if control == nil || control.host == "" || mode == webfig.ModeOff {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "WebFig is not available", "code": "webfig_off", "access": webfig.ModeOff})
		return nil, "", "", "", false
	}
	if !webfig.Entitled(mode, auth.GetUserRole(c)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "WebFig is locked on this account", "code": "webfig_locked", "access": webfig.ModeLocked})
		return nil, "", "", "", false
	}
	businessID, deviceID := auth.GetBusinessID(c), c.Param("id")
	if _, err := control.targets.TargetForDevice(c.Request.Context(), businessID, deviceID); err != nil {
		if errors.Is(err, routeros.ErrTargetNotAllowed) {
			c.JSON(http.StatusConflict, gin.H{"error": "This router has no Magic Hub address", "code": "webfig_no_tunnel"})
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": "Router not found"})
		}
		return nil, "", "", "", false
	}
	tunnel, err := hub.TunnelInterfaceName(deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Router not found"})
		return nil, "", "", "", false
	}
	return control, businessID, deviceID, tunnel, true
}

func webFigFailure(c *gin.Context, err error) {
	if errors.Is(err, routeros.ErrWebFigNoTunnel) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "code": "webfig_no_tunnel"})
		return
	}
	c.JSON(http.StatusBadGateway, gin.H{"error": "Could not read this router through Magic Hub. Check that it is online.", "code": "webfig_unreachable"})
}

func webFigView(access routeros.WebFigAccess, mode webfig.Mode) gin.H {
	return gin.H{"access": mode, "ready": access.Ready, "plan": access.Plan, "version": access.Version, "update_required": security.WebFigProxyUnsafe(access.Version)}
}

// WebFigStatus: GET /routers/:id/webfig. The locked and off answers do not touch
// the router; the console uses them to show or hide the button.
func (h *SecurityHandler) WebFigStatus(c *gin.Context) {
	mode := webfig.ModeFromEnv()
	if h.webFig == nil || h.webFig.host == "" || mode == webfig.ModeOff {
		c.JSON(http.StatusOK, gin.H{"access": webfig.ModeOff})
		return
	}
	if !webfig.Entitled(mode, auth.GetUserRole(c)) {
		c.JSON(http.StatusOK, gin.H{"access": webfig.ModeLocked})
		return
	}
	control, businessID, deviceID, tunnel, ok := h.webFigGate(c)
	if !ok {
		return
	}
	access, err := control.router.WebFigAccessState(c.Request.Context(), businessID, deviceID, tunnel, control.sourceCIDR)
	if err != nil {
		webFigFailure(c, err)
		return
	}
	c.JSON(http.StatusOK, webFigView(access, mode))
}

// WebFigAllow: POST /routers/:id/webfig/access applies the reviewed plan.
func (h *SecurityHandler) WebFigAllow(c *gin.Context) {
	control, businessID, deviceID, tunnel, ok := h.webFigGate(c)
	if !ok {
		return
	}
	var body struct {
		Confirm string `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Confirm != webFigConfirm {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Review the change before applying it"})
		return
	}
	access, err := control.router.EnableWebFigAccess(c.Request.Context(), businessID, deviceID, tunnel, control.sourceCIDR)
	if err != nil {
		control.record(c, businessID, deviceID, "router.webfig.access.failed", "by user "+auth.GetUserID(c)+": "+err.Error())
		if errors.Is(err, routeros.ErrWebFigNoTunnel) {
			webFigFailure(c, err)
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "code": "webfig_change_failed"})
		return
	}
	control.record(c, businessID, deviceID, "router.webfig.access.added", "by user "+auth.GetUserID(c)+": Hub allowed to reach WebFig through the tunnel")
	c.JSON(http.StatusOK, webFigView(access, webfig.ModeFromEnv()))
}

// WebFigRemove: DELETE /routers/:id/webfig/access undoes it on the router.
func (h *SecurityHandler) WebFigRemove(c *gin.Context) {
	control, businessID, deviceID, tunnel, ok := h.webFigGate(c)
	if !ok {
		return
	}
	access, err := control.router.DisableWebFigAccess(c.Request.Context(), businessID, deviceID, tunnel, control.sourceCIDR)
	if err != nil {
		control.record(c, businessID, deviceID, "router.webfig.access.remove_failed", "by user "+auth.GetUserID(c)+": "+err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "code": "webfig_change_failed"})
		return
	}
	control.record(c, businessID, deviceID, "router.webfig.access.removed", "by user "+auth.GetUserID(c))
	c.JSON(http.StatusOK, webFigView(access, webfig.ModeFromEnv()))
}

// WebFigOpen: POST /routers/:id/webfig/session returns a one-use, one-minute link.
func (h *SecurityHandler) WebFigOpen(c *gin.Context) {
	control, businessID, deviceID, tunnel, ok := h.webFigGate(c)
	if !ok {
		return
	}
	sessionID, _ := c.Get("auth_session_id")
	authSession, _ := sessionID.(string)
	if authSession == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Sign in again to open WebFig"})
		return
	}
	// The checks below read the router at this address. The ticket must go to the same one.
	checked, err := control.targets.TargetForDevice(c.Request.Context(), businessID, deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Router not found"})
		return
	}
	access, err := control.router.WebFigAccessState(c.Request.Context(), businessID, deviceID, tunnel, control.sourceCIDR)
	if err != nil {
		webFigFailure(c, err)
		return
	}
	if security.WebFigProxyUnsafe(access.Version) {
		control.record(c, businessID, deviceID, "router.webfig.refused", "by user "+auth.GetUserID(c)+": RouterOS "+access.Version+" has a WebFig security flaw")
		c.JSON(http.StatusConflict, gin.H{"error": "Update RouterOS on this router first. Version " + access.Version + " has a known WebFig security flaw.", "code": "webfig_update_required"})
		return
	}
	if !access.Ready {
		c.JSON(http.StatusConflict, gin.H{"error": "This router needs one change before WebFig can open", "code": "webfig_needs_setup", "plan": access.Plan})
		return
	}
	target, err := control.targets.TargetForDevice(c.Request.Context(), businessID, deviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Router not found"})
		return
	}
	host, _, err := net.SplitHostPort(target.Addr)
	if err != nil || target.Addr != checked.Addr {
		c.JSON(http.StatusConflict, gin.H{"error": "This router has no Magic Hub address", "code": "webfig_no_tunnel"})
		return
	}
	ticket, label, err := control.store.Issue(webfig.Grant{UserID: auth.GetUserID(c), BusinessID: businessID, DeviceID: deviceID, Role: auth.GetUserRole(c), AuthSessionID: authSession, Scheme: access.Scheme, Host: host, Port: access.Port})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not open WebFig"})
		return
	}
	control.record(c, businessID, deviceID, "router.webfig.opened", "by user "+auth.GetUserID(c)+": session up to "+strconv.Itoa(int(webfig.SessionLifetime.Minutes()))+" minutes")
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"url": fmt.Sprintf("https://%s.%s/__mm/open?ticket=%s", label, control.host, url.QueryEscape(ticket))})
}

// WebFigChecker re-confirms a running session against the database: the login,
// the role, the switch, the account's pass, and that the router is still this
// tenant's at this address. passAccess is nil when passes aren't enforced.
func WebFigChecker(db interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}, targets routeros.TargetSource, passAccess func(context.Context, string) (bool, error)) webfig.Checker {
	return func(ctx context.Context, g webfig.Grant) error {
		var role string
		err := db.QueryRowContext(ctx, `SELECT s.user_role FROM auth_sessions s
			JOIN users u ON u.user_id = s.user_id
			JOIN businesses b ON b.business_id = s.business_id
			WHERE s.session_id = $1 AND s.user_id = $2 AND s.business_id = $3
			  AND NOT s.revoked AND s.expires_at > now() AND u.is_active AND b.is_active AND s.user_role = u.user_role`,
			g.AuthSessionID, g.UserID, g.BusinessID).Scan(&role)
		if err != nil {
			return fmt.Errorf("login no longer valid: %w", err)
		}
		if err := webFigPassValid(ctx, g.BusinessID, passAccess); err != nil {
			return err
		}
		return webFigStillAllowed(ctx, g, role, targets)
	}
}

// webFigPassValid ends a session when the account's pass ends. An unreadable pass counts as ended.
func webFigPassValid(ctx context.Context, businessID string, passAccess func(context.Context, string) (bool, error)) error {
	if passAccess == nil {
		return nil
	}
	allowed, err := passAccess(ctx, businessID)
	if err != nil {
		return fmt.Errorf("account pass could not be read: %w", err)
	}
	if !allowed {
		return errors.New("account pass has ended")
	}
	return nil
}

func webFigStillAllowed(ctx context.Context, g webfig.Grant, role string, targets routeros.TargetSource) error {
	if !model.HasAtLeastRole(model.UserRole(role), model.RoleAdmin) {
		return errors.New("role no longer qualifies")
	}
	if !webfig.Entitled(webfig.ModeFromEnv(), role) {
		return errors.New("feature is off or locked")
	}
	target, err := targets.TargetForDevice(ctx, g.BusinessID, g.DeviceID)
	if err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(target.Addr)
	if err != nil || host != g.Host {
		return errors.New("router address changed")
	}
	return nil
}
