package handler

import (
	"database/sql"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

type RouterHandler struct {
	db         *database.DB
	deviceRepo *database.DeviceRepository
	messaging  MessagingClient
}

type MessagingClient interface {
	Publish(businessID, subject string, data []byte) error
}

func NewRouterHandler(db *database.DB, deviceRepo *database.DeviceRepository, msg MessagingClient) *RouterHandler {
	return &RouterHandler{db: db, deviceRepo: deviceRepo, messaging: msg}
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
		cr, err := h.deviceRepo.CreateDeviceTXN(tx, device)
		if err != nil {
			return err
		}
		created = cr
		return nil
	})
	if err != nil {
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
