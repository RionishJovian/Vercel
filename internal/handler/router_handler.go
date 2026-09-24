package handler

import (
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

type RouterHandler struct {
	deviceRepo *database.DeviceRepository
	messaging  MessagingClient
}

type MessagingClient interface {
	Publish(businessID, subject string, data []byte) error
}

func NewRouterHandler(deviceRepo *database.DeviceRepository, msg MessagingClient) *RouterHandler {
	return &RouterHandler{deviceRepo: deviceRepo, messaging: msg}
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

	devices, err := h.deviceRepo.ListDevicesByBusiness(businessID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list routers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"routers": devices})
}

// GetRouter returns a single router by ID
func (h *RouterHandler) GetRouter(c *gin.Context) {
	deviceID := c.Param("id")

	device, err := h.deviceRepo.GetDeviceByID(deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if device == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "router not found"})
		return
	}

	// Verify ownership
	businessID := auth.GetBusinessID(c)
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
		BusinessID:       businessID,
		HardwareName:     req.HardwareName,
		DeviceModel:      req.DeviceModel,
		DeploymentLayer:  req.DeploymentLayer,
		Vendor:           req.Vendor,
		Category:         req.Category,
		Transport:        req.Transport,
		MACAddress:       parseMAC(req.MACAddress),
		ManagementIP:     parseIP(req.ManagementIP),
		IsOnline:         false,
		LastSeenAt:       &now,
	}

	created, err := h.deviceRepo.CreateDevice(device)
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

	existing, err := h.deviceRepo.GetDeviceByID(deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "router not found"})
		return
	}
	if existing.BusinessID != businessID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var req UpdateRouterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate only the fields being updated (nil = untouched).
	if req.DeploymentLayer != nil && !validDeploymentLayers[*req.DeploymentLayer] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment_layer: expected MAIN_GATEWAY, DOWNSTREAM_SWITCH, ACCESS_POINT or CONTROLLER"})
		return
	}
	if req.Vendor != nil && !validVendors[*req.Vendor] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vendor: expected mikrotik, ruijie, cisco, tplink, ubiquiti or generic"})
		return
	}
	if req.Category != nil && !validCategories[*req.Category] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category: expected router, gateway, switch, ap or controller"})
		return
	}
	if req.Transport != nil && !validTransports[*req.Transport] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transport: expected direct, connector, tunnel or controller"})
		return
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

	updated, err := h.deviceRepo.UpdateDevice(existing)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update router"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"router": updated})
}

// DeleteRouter removes a router
func (h *RouterHandler) DeleteRouter(c *gin.Context) {
	deviceID := c.Param("id")
	businessID := auth.GetBusinessID(c)

	existing, err := h.deviceRepo.GetDeviceByID(deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "router not found"})
		return
	}
	if existing.BusinessID != businessID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := h.deviceRepo.DeleteDevice(deviceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete router"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "router deleted"})
}