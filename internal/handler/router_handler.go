package handler

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/discovery"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
)

type RouterHandler struct {
	hubRemoval       func(ctx context.Context, businessID, requester, deviceID string) (string, error)
	hubForget        func(ctx context.Context, businessID, deviceID string) error
	recordRouterRead func(ctx context.Context, businessID, deviceID string, at time.Time) error
	SubscriptionOnly bool
	db               *database.DB
	deviceRepo       *database.DeviceRepository
	allowRepo        *database.DeviceAllowanceRepository
	messaging        MessagingClient
	observations     interface {
		ReadObservations(context.Context, string, string) (routeros.NetworkInventory, error)
	}
	guestActions GuestRouterActions
}

type MessagingClient interface {
	Publish(businessID, subject string, data []byte) error
}

func NewRouterHandler(db *database.DB, deviceRepo *database.DeviceRepository, allowRepo *database.DeviceAllowanceRepository, msg MessagingClient) *RouterHandler {
	return &RouterHandler{db: db, deviceRepo: deviceRepo, allowRepo: allowRepo, messaging: msg}
}

// ConfigureHubForget clears a kept router's old setup material after its connection ends.
func (h *RouterHandler) ConfigureHubForget(forget func(ctx context.Context, businessID, deviceID string) error) {
	h.hubForget = forget
}

// ConfigureHubRemoval makes Delete end a router's Magic Hub connection. The function
// returns a script that removes the setup from the router, or "" when there is none.
func (h *RouterHandler) ConfigureHubRemoval(remove func(ctx context.Context, businessID, requester, deviceID string) (string, error)) {
	h.hubRemoval = remove
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

		if existing.Transport == "tunnel" && existing.ManagementIP != nil && req.Transport != nil && *req.Transport != "tunnel" {
			return &httpError{status: http.StatusConflict, msg: "Disconnect the saved Hub connection before changing its transport"}
		}
		if req.ManagementIP != nil && existing.Transport == "tunnel" && existing.ManagementIP != nil {
			next := parseIP(req.ManagementIP)
			if next == nil || !(*next).Equal(*existing.ManagementIP) {
				return &httpError{status: http.StatusConflict, msg: "Use the guided management-address migration in Magic Hub; editing a saved record cannot move a live tunnel"}
			}
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

	cleanup, kept, hubEnded := "", false, false
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

		// A blocked delete aborts the transaction, so a savepoint lets the same one go on.
		if _, err := tx.Exec("SAVEPOINT router_delete"); err != nil {
			log.Printf("router delete: savepoint failed for %s: %v", deviceID, err)
			return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
		}
		if err := h.deviceRepo.DeleteDeviceTXN(tx, deviceID); err != nil {
			var pending *pq.Error
			if errors.As(err, &pending) && pending.Code == "55000" && pending.Message == "management_migration_pending" {
				return &httpError{status: http.StatusConflict, msg: "Recover or cancel this router's management migration before deleting its record"}
			}
			hasHistory := errors.As(err, &pending) && pending.Code == "23503"
			switch {
			case hasHistory && h.hubRemoval != nil && existing.DeploymentLayer == "MAIN_GATEWAY":
				// Sales history must stay, but the owner still has to be able to reconnect
				// this router or a replacement: end the connection and keep the record.
				if _, err := tx.Exec("ROLLBACK TO SAVEPOINT router_delete"); err != nil {
					log.Printf("router delete: could not roll back to savepoint for %s: %v", deviceID, err)
					return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
				}
				if err := h.deviceRepo.ClearManagementTXN(tx, businessID, deviceID); err != nil {
					log.Printf("router delete: could not end the connection and keep the record for %s: %v", deviceID, err)
					return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
				}
				kept = true
			case hasHistory && strings.HasPrefix(pending.Constraint, "broadband_"):
				return &httpError{status: http.StatusConflict, msg: "This device record has paid PPPoE accounts and payment history, so it cannot be deleted."}
			case hasHistory:
				return &httpError{status: http.StatusConflict, msg: "This device record is linked to vouchers, issuance batches or guest recovery history and cannot be deleted. Keep the device record to preserve that history. No records were deleted."}
			default:
				log.Printf("router delete: delete failed for %s: %v", deviceID, err)
				return &httpError{status: http.StatusInternalServerError, msg: "failed to delete router"}
			}
		}
		// Inside the transaction: if the Hub cannot end the connection, nothing changes.
		if h.hubRemoval != nil && existing.DeploymentLayer == "MAIN_GATEWAY" {
			script, err := h.hubRemoval(c.Request.Context(), businessID, auth.GetUserID(c), deviceID)
			if err != nil {
				return &httpError{status: http.StatusServiceUnavailable, msg: "Magic Hub could not end this router's connection, so nothing was deleted. Try again."}
			}
			cleanup, hubEnded = script, true
		}
		return nil
	})
	if err != nil {
		// The Hub step is last, so this means only the commit failed. The connection is
		// gone and the record is unchanged. Pressing Delete again finishes it: the Hub
		// then has nothing left to end. This is the only copy of the cleanup script.
		if hubEnded {
			log.Printf("router delete: Hub connection ended for %s but the record was not updated: %v", deviceID, err)
			response := gin.H{"error": "Magic Hub ended this router's connection, but its record could not be updated. Press Delete again to finish.", "code": "hub_ended_record_unchanged"}
			if cleanup != "" {
				c.Header("Cache-Control", "no-store")
				response["router_cleanup_script"] = cleanup
			}
			c.JSON(http.StatusServiceUnavailable, response)
			return
		}
		respondErr(c, err)
		return
	}

	response := gin.H{"message": "router deleted"}
	if kept {
		response = gin.H{"message": "connection ended; record and history kept", "record_kept": true}
		// Old setup material would only mislead the next connection. The tenant role cannot
		// touch this table, and a leftover is harmless, so a failure here is not fatal.
		if h.hubForget != nil {
			if err := h.hubForget(c.Request.Context(), businessID, deviceID); err != nil {
				log.Printf("router delete: could not clear old setup for %s", deviceID)
			}
		}
	}
	if cleanup != "" {
		c.Header("Cache-Control", "no-store")
		response["router_cleanup_script"] = cleanup
	}
	c.JSON(http.StatusOK, response)
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
	if h.recordRouterRead != nil && inventoryAnswered(inventory) {
		if err := h.recordRouterRead(c.Request.Context(), businessID, deviceID, time.Now()); err != nil {
			log.Printf("router read: could not record for %s: %v", deviceID, err)
		}
	}
	inventory.DeviceControlsEnabled = deviceControlsEnabled()
	inventory.WriteEnabled = false
	inventory.WiFiWriteEnabled = false
	inventory.AddressWriteEnabled = false
	inventory.AddressPreviewOnly = false
	c.JSON(200, gin.H{"inventory": inventory, "guest_actions_enabled": h.guestActionsReady()})
}

// GetRouterDiscovery provides a read-only, normalized discovery snapshot
// using the SAME tenant-bound gateway credentials as the observation route.
// It does not probe any LAN address or mutate devices; unsupported management
// capabilities and physical attachments remain explicitly unverified.
func (h *RouterHandler) GetRouterDiscovery(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	businessID, deviceID := auth.GetBusinessID(c), c.Param("id")
	var labels []*database.DiscoveryLabel
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		d, e := h.deviceRepo.GetDeviceByIDTXN(tx, deviceID)
		if e != nil {return e}
		if d == nil || d.BusinessID != businessID {
			return &httpError{status: http.StatusNotFound, msg: "router not found"}
		}
		labels, e = h.deviceRepo.GetDiscoveryLabelsTXN(tx, businessID, deviceID)
		return e
	})
	if err != nil {respondErr(c, err);return}
	if h.observations == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error":"gateway read-only observations unavailable","code":"discovery_unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	inventory, err := h.observations.ReadObservations(ctx, businessID, deviceID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error":"gateway discovery observations unavailable","code":"discovery_unavailable"})
		return
	}
	overrides := make([]discovery.Label, 0, len(labels))
	for _, l := range labels {
		if l == nil {continue}
		overrides = append(overrides, discovery.Label{MAC:l.MACAddress,Name:l.Name,Kind:l.Kind})
	}
	snapshot := discovery.Build(deviceID, inventory, overrides)
	c.JSON(http.StatusOK, gin.H{"discovery":snapshot})
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

// DisconnectGuest ends one session, and only reports success once the router confirms it.
func (h *RouterHandler) DisconnectGuest(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	deviceID := c.Param("id")
	businessID := auth.GetBusinessID(c)
	if !h.guestGateway(c, businessID, deviceID) {
		return
	}
	var req DisconnectGuestRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirmation required"})
		return
	}
	if !h.guestActionsReady() {
		guestActionsOff(c)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), guestActionTimeout)
	defer cancel()
	err := h.guestActions.DisconnectGuest(ctx, businessID, deviceID, routeros.GuestSession{ID: req.ID, MAC: strings.ToUpper(req.MAC), Address: req.Address, User: req.User})
	h.finishGuestAction(c, businessID, deviceID, "router.guest.disconnected", "Owner disconnected guest MAC "+strings.ToUpper(req.MAC), err)
}

// inventoryAnswered reports whether the router itself answered, not just the request.
func inventoryAnswered(inventory routeros.NetworkInventory) bool {
	if inventory.ObservedAt == "" {
		return false
	}
	if inventory.Health["version"] != "" {
		return true
	}
	for _, available := range inventory.Available {
		if available {
			return true
		}
	}
	return false
}

// ConfigureRouterReadRecorder feeds successful reads into the shared status source.
func (h *RouterHandler) ConfigureRouterReadRecorder(record func(ctx context.Context, businessID, deviceID string, at time.Time) error) {
	h.recordRouterRead = record
}

// EnableObservations supplies a read-only interface, separately from router writes.
func (h *RouterHandler) EnableObservations(reader interface {
	ReadObservations(context.Context, string, string) (routeros.NetworkInventory, error)
}) {
	h.observations = reader
}
