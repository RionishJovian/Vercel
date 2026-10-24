package handler

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
)

type RouterHandler struct {
	SubscriptionOnly bool
	db               *database.DB
	deviceRepo       *database.DeviceRepository
	allowRepo        *database.DeviceAllowanceRepository
	messaging        MessagingClient
	observations     interface {
		ReadObservations(context.Context, string, string) (routeros.NetworkInventory, error)
	}
}

type MessagingClient interface {
	Publish(businessID, subject string, data []byte) error
}

func NewRouterHandler(db *database.DB, deviceRepo *database.DeviceRepository, allowRepo *database.DeviceAllowanceRepository, msg MessagingClient) *RouterHandler {
	return &RouterHandler{db: db, deviceRepo: deviceRepo, allowRepo: allowRepo, messaging: msg}
}

// withTenantTx runs fn in an RLS-bound tenant transaction (pinned conn when
// database.rls_enforce is on, owner tx otherwise). Mirrors the money-path helper.
func (h *RouterHandler) withTenantTx(c *gin.Context, businessID string, fn func(tx *sql.Tx) error) error {
	return runInTenant(c, h.db, businessID, fn)
}

type CreateRouterRequest struct {
	HardwareName    string  `json:"hardware_name" binding:"required"`
	DeviceModel     *string `json:"device_model"`
	DeploymentLayer string  `json:"deployment_layer"`
	Vendor          string  `json:"vendor"`
	Category        string  `json:"category"`
	Transport       string  `json:"transport"`
	MACAddress      *string `json:"mac_address"`
	ManagementIP    *string `json:"management_ip"`
}

type UpdateRouterRequest struct {
	HardwareName    *string `json:"hardware_name"`
	DeviceModel     *string `json:"device_model"`
	DeploymentLayer *string `json:"deployment_layer"`
	Vendor          *string `json:"vendor"`
	Category        *string `json:"category"`
	Transport       *string `json:"transport"`
	MACAddress      *string `json:"mac_address"`
	ManagementIP    *string `json:"management_ip"`
	IsOnline        *bool   `json:"is_online"`
}

func parseMAC(s *string) *net.HardwareAddr {
	if s == nil || *s == "" {
		return nil
	}
	mac, err := net.ParseMAC(*s)
	if err != nil {
		return nil
	}
	return &mac
}

func parseIP(s *string) *net.IP {
	if s == nil || *s == "" {
		return nil
	}
	ip := net.ParseIP(*s)
	if ip == nil {
		return nil
	}
	return &ip
}

var (
	validDeploymentLayers = map[string]bool{"MAIN_GATEWAY": true, "DOWNSTREAM_SWITCH": true, "ACCESS_POINT": true, "CONTROLLER": true}
	validVendors          = map[string]bool{"mikrotik": true, "ruijie": true, "cisco": true, "tplink": true, "ubiquiti": true, "generic": true}
	validCategories       = map[string]bool{"router": true, "gateway": true, "switch": true, "ap": true, "controller": true}
	validTransports       = map[string]bool{"direct": true, "connector": true, "tunnel": true, "controller": true}
)

// validateDeviceEnums enforces the DB check constraints before hitting the database.
func validateDeviceEnums(deploymentLayer, vendor, category, transport string) string {
	if !validDeploymentLayers[deploymentLayer] {
		return "invalid deployment_layer: expected MAIN_GATEWAY, DOWNSTREAM_SWITCH, ACCESS_POINT or CONTROLLER"
	}
	if !validVendors[vendor] {
		return "invalid vendor: expected mikrotik, ruijie, cisco, tplink, ubiquiti or generic"
	}
	if !validCategories[category] {
		return "invalid category: expected router, gateway, switch, ap or controller"
	}
	if !validTransports[transport] {
		return "invalid transport: expected direct, connector, tunnel or controller"
	}
	return ""
}

// ListRouters returns all routers for the authenticated business
func (h *RouterHandler) ListRouters(c *gin.Context) {
	businessID := auth.GetBusinessID(c)

	var devices []*model.Device
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		ds, err := h.deviceRepo.ListDevicesByBusinessTXN(tx, businessID)
		if err != nil {
			return err
		}
		devices = ds
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list routers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"routers": devices})
}

// GetRouter returns a single router by ID
func (h *RouterHandler) GetRouter(c *gin.Context) {
	deviceID := c.Param("id")
	businessID := auth.GetBusinessID(c)

	var device *model.Device
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		d, err := h.deviceRepo.GetDeviceByIDTXN(tx, deviceID)
		if err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		device = d
		return nil
	})
	if err != nil {
		respondErr(c, err)
		return
	}
	if device == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "router not found"})
		return
	}

	// Ownership is enforced by the tenant scope (a cross-business device is
	// invisible → 404); the explicit check remains as defence-in-depth.
	if device.BusinessID != businessID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"router": device})
}

// CreateRouter registers a new router
func (h *RouterHandler) CreateRouter(c *gin.Context) {
	var req CreateRouterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if msg := validateDeviceEnums(req.DeploymentLayer, req.Vendor, req.Category, req.Transport); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}

	businessID := auth.GetBusinessID(c)
	now := time.Now().UTC()

	device := &model.Device{
		BusinessID:      businessID,
		HardwareName:    req.HardwareName,
		DeviceModel:     req.DeviceModel,
		DeploymentLayer: req.DeploymentLayer,
		Vendor:          req.Vendor,
		Category:        req.Category,
		Transport:       req.Transport,
		MACAddress:      parseMAC(req.MACAddress),
		ManagementIP:    parseIP(req.ManagementIP),
		IsOnline:        false,
		LastSeenAt:      &now,
	}

	var created *model.Device
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		kind := ""
		if req.DeploymentLayer == "CONTROLLER" {
			kind = "controllers"
		}
		if req.DeploymentLayer == "MAIN_GATEWAY" || req.DeploymentLayer == "DOWNSTREAM_SWITCH" {
			kind = "routers"
		}
		// Access points are reported separately; the ported grant never defined
		// an access-point limit, so do not invent one here.
		if kind != "" && !h.SubscriptionOnly {
			if err := h.allowRepo.CheckCapacityTXN(tx, businessID, kind); err != nil {
				return err
			}
		}
		cr, err := h.deviceRepo.CreateDeviceTXN(tx, device)
		if err != nil {
			return err
		}
		created = cr
		return nil
	})
	if err != nil {
		if errors.Is(err, database.ErrDeviceCapacityExceeded) {
			c.JSON(http.StatusConflict, gin.H{"error": "capacity exceeded", "reason": "request additional capacity before adding this device"})
			return
		}
		if respondAppCapacity(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register router"})
		return
	}

	// Publish registration event over NATS
	if h.messaging != nil {
		event := []byte(`{"device_id":"` + created.DeviceID + `","hardware":"` + created.HardwareName + `"}`)
		_ = h.messaging.Publish(businessID, "fleet.register", event)
	}

	c.JSON(http.StatusCreated, gin.H{"router": created})
}

// UpdateRouter updates an existing router
func (h *RouterHandler) UpdateRouter(c *gin.Context) {
	deviceID := c.Param("id")
	businessID := auth.GetBusinessID(c)

	var updated *model.Device
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		existing, err := h.deviceRepo.GetDeviceByIDTXN(tx, deviceID)
		if err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		if existing == nil {
			return &httpError{status: http.StatusNotFound, msg: "router not found"}
		}
		if existing.BusinessID != businessID {
			return &httpError{status: http.StatusForbidden, msg: "access denied"}
		}

		var req UpdateRouterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			return &httpError{status: http.StatusBadRequest, msg: err.Error()}
		}

		// Validate only the fields being updated (nil = untouched).
		if req.DeploymentLayer != nil && !validDeploymentLayers[*req.DeploymentLayer] {
			return &httpError{status: http.StatusBadRequest, msg: "invalid deployment_layer: expected MAIN_GATEWAY, DOWNSTREAM_SWITCH, ACCESS_POINT or CONTROLLER"}
		}
		if req.Vendor != nil && !validVendors[*req.Vendor] {
			return &httpError{status: http.StatusBadRequest, msg: "invalid vendor: expected mikrotik, ruijie, cisco, tplink, ubiquiti or generic"}
		}
		if req.Category != nil && !validCategories[*req.Category] {
			return &httpError{status: http.StatusBadRequest, msg: "invalid category: expected router, gateway, switch, ap or controller"}
		}
		if req.Transport != nil && !validTransports[*req.Transport] {
			return &httpError{status: http.StatusBadRequest, msg: "invalid transport: expected direct, connector, tunnel or controller"}
		}

		if req.HardwareName != nil {
			existing.HardwareName = *req.HardwareName
		}
		if req.DeviceModel != nil {
			existing.DeviceModel = req.DeviceModel
		}
		if req.DeploymentLayer != nil {
			existing.DeploymentLayer = *req.DeploymentLayer
		}
		if req.Vendor != nil {
			existing.Vendor = *req.Vendor
		}
		if req.Category != nil {
			existing.Category = *req.Category
		}
		if req.Transport != nil {
			existing.Transport = *req.Transport
		}
		if req.MACAddress != nil {
			existing.MACAddress = parseMAC(req.MACAddress)
		}
		if req.ManagementIP != nil {
			existing.ManagementIP = parseIP(req.ManagementIP)
		}
		if req.IsOnline != nil {
			existing.IsOnline = *req.IsOnline
		}

		upd, err := h.deviceRepo.UpdateDeviceTXN(tx, existing)
		if err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "failed to update router"}
		}
		updated = upd
		return nil
	})
	if err != nil {
		respondErr(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"router": updated})
}

// DeleteRouter removes a router
func (h *RouterHandler) DeleteRouter(c *gin.Context) {
	deviceID := c.Param("id")
	businessID := auth.GetBusinessID(c)

	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		existing, err := h.deviceRepo.GetDeviceByIDTXN(tx, deviceID)
		if err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		if existing == nil {
			return &httpError{status: http.StatusNotFound, msg: "router not found"}
		}
		if existing.BusinessID != businessID {
			return &httpError{status: http.StatusForbidden, msg: "access denied"}
		}

		if err := h.deviceRepo.DeleteDeviceTXN(tx, deviceID); err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
		}
		return nil
	})
	if err != nil {
		respondErr(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "router deleted"})
}

// GetRouterObservations returns the latest observed inventory for a router.
// This is read-only telemetry — no router configuration is changed.
func (h *RouterHandler) GetRouterObservations(c *gin.Context) {
	deviceID := c.Param("id")
	businessID := auth.GetBusinessID(c)

	var device *model.Device
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		d, err := h.deviceRepo.GetDeviceByIDTXN(tx, deviceID)
		if err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		device = d
		return nil
	})
	if err != nil {
		respondErr(c, err)
		return
	}
	if device == nil || device.BusinessID != businessID {
		c.JSON(http.StatusNotFound, gin.H{"error": "router not found"})
		return
	}

	c.Header("Cache-Control", "no-store")
	if h.observations == nil {
		c.JSON(503, gin.H{"error": "Read-only router observations are not enabled", "code": "observations_unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	inventory, err := h.observations.ReadObservations(ctx, businessID, deviceID)
	if err != nil {
		c.JSON(502, gin.H{"error": "Router observations could not be read", "code": "observations_unavailable"})
		return
	}
	inventory.WriteEnabled = false
	inventory.WiFiWriteEnabled = false
	c.JSON(200, gin.H{"inventory": inventory})
}

// GetDiscoveryLabels returns saved friendly names and types for discovered devices.
func (h *RouterHandler) GetDiscoveryLabels(c *gin.Context) {
	businessID, deviceID := auth.GetBusinessID(c), c.Param("id")
	var labels []*database.DiscoveryLabel
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		d, e := h.deviceRepo.GetDeviceByIDTXN(tx, deviceID)
		if e != nil {
			return e
		}
		if d == nil || d.BusinessID != businessID {
			return &httpError{status: 404, msg: "router not found"}
		}
		labels, e = h.deviceRepo.GetDiscoveryLabelsTXN(tx, businessID, deviceID)
		return e
	})
	if err != nil {
		respondErr(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"labels": labels})
}

type UpsertDiscoveryLabelRequest struct {
	MACAddress string `json:"mac" binding:"required"`
	Name       string `json:"name" binding:"required"`
	Kind       string `json:"kind" binding:"required,oneof=other switch ap computer printer controller"`
}

func (h *RouterHandler) UpsertDiscoveryLabel(c *gin.Context) {
	var req UpsertDiscoveryLabelRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"error": "mac, name and supported kind required"})
		return
	}
	mac, e := net.ParseMAC(req.MACAddress)
	req.Name = strings.TrimSpace(req.Name)
	if e != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" || len(req.Name) == 0 || len(req.Name) > 120 {
		c.JSON(400, gin.H{"error": "valid unicast MAC and name of 1–120 bytes required"})
		return
	}
	businessID, deviceID := auth.GetBusinessID(c), c.Param("id")
	var labels []*database.DiscoveryLabel
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		d, e := h.deviceRepo.GetDeviceByIDTXN(tx, deviceID)
		if e != nil {
			return e
		}
		if d == nil || d.BusinessID != businessID {
			return &httpError{status: 404, msg: "router not found"}
		}
		if _, e = h.deviceRepo.UpsertDiscoveryLabelTXN(tx, businessID, deviceID, strings.ToUpper(mac.String()), req.Name, req.Kind); e != nil {
			return e
		}
		labels, e = h.deviceRepo.GetDiscoveryLabelsTXN(tx, businessID, deviceID)
		return e
	})
	if err != nil {
		respondErr(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"labels": labels})
}

type DisconnectGuestRequest struct {
	ID      string `json:"id" binding:"required"`
	MAC     string `json:"mac" binding:"required"`
	Address string `json:"address" binding:"required"`
	User    string `json:"user" binding:"required"`
	Confirm bool   `json:"confirm" binding:"required"`
}

// DisconnectGuest refuses unverified actions until a router write connector is accepted.
func (h *RouterHandler) DisconnectGuest(c *gin.Context) {
	deviceID := c.Param("id")
	businessID := auth.GetBusinessID(c)

	var device *model.Device
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		d, err := h.deviceRepo.GetDeviceByIDTXN(tx, deviceID)
		if err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		device = d
		return nil
	})
	if err != nil {
		respondErr(c, err)
		return
	}
	if device == nil || device.BusinessID != businessID {
		c.JSON(http.StatusNotFound, gin.H{"error": "router not found"})
		return
	}

	var req DisconnectGuestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirmation required"})
		return
	}

	// Report unavailability as an error, never as successful disconnection.
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Guest disconnection is unavailable until the router action has passed acceptance", "code": "guest_disconnect_unavailable", "verified": false})
}

// EnableObservations supplies a read-only interface, separately from router writes.
func (h *RouterHandler) EnableObservations(reader interface {
	ReadObservations(context.Context, string, string) (routeros.NetworkInventory, error)
}) {
	h.observations = reader
}
