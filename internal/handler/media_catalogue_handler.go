package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/media/bridge"
	"mega-magic/internal/model"
)

// MediaCatalogueHandler serves the guest-facing catalogue for the business that
// owns the requesting hotspot device.
//
// The owner publishes a design once. Guests browsing the captive portal must see
// that design without knowing a publication hash, so this resolves the business
// from the device id the portal already carries and reads the publication under
// that tenant. One guest can therefore never see another business's library.
//
// Catalogue metadata only: titles, captions and poster images. It never returns
// a stream URL, a credential, or any path through this service.
type MediaCatalogueHandler struct {
	Store       bridge.PublicationStore
	DeviceIndex DeviceIndex
	Ready       bool
}

// DeviceIndex resolves a hotspot device to its owning business. It is the same
// device lookup the portal config uses, so tenant scoping is identical.
type DeviceIndex interface {
	GetDeviceByID(deviceID string) (*model.Device, error)
}

// PublicCatalogue: GET /api/v1/portal/:deviceId/media
//
// Registered on the same group as the rest of the guest portal surface, under
// the existing portal rate limit and cookie-origin check.
func (h *MediaCatalogueHandler) PublicCatalogue(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.Ready || h.DeviceIndex == nil {
		c.Status(http.StatusNotFound)
		return
	}
	deviceID := c.Param("deviceId")
	if deviceID == "" {
		c.Status(http.StatusNotFound)
		return
	}
	dev, err := h.DeviceIndex.GetDeviceByID(deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if dev == nil {
		// Unknown device. Reported exactly like "nothing published", so a guest
		// cannot use this to discover which device ids are valid.
		c.Status(http.StatusNotFound)
		return
	}
	owner := dev.BusinessID
	pub, e := h.Store.Publication(c.Request.Context(), owner)
	if e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			c.Status(http.StatusNotFound)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if len(pub.Design.Libraries) == 0 {
		c.Status(http.StatusNotFound)
		return
	}
	c.JSON(http.StatusOK, gin.H{"design": pub.Design, "revision": pub.Revision})
}
