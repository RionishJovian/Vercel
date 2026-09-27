package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// SiteHandler serves sites — physical locations with the geo data ported from
// Mikromagic (location, timezone, latitude/longitude) plus the device count.
type SiteHandler struct {
	db       *database.DB
	siteRepo *database.SiteRepository
}

// NewSiteHandler wires the site repository.
func NewSiteHandler(db *database.DB, siteRepo *database.SiteRepository) *SiteHandler {
	return &SiteHandler{db: db, siteRepo: siteRepo}
}

// ListSites returns every site in the caller's business.
func (h *SiteHandler) ListSites(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var sites []*model.Site
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		list, err := h.siteRepo.ListSitesTXN(tx, businessID)
		if err != nil {
			return err
		}
		sites = list
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"sites": sites, "count": len(sites)})
}

// CreateSite adds a site (ADMIN+ — enforced at the route).
func (h *SiteHandler) CreateSite(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req model.SiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var site *model.Site
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		s, err := h.siteRepo.CreateSiteTXN(tx, businessID, &req)
		if err != nil {
			return err
		}
		site = s
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"site": site})
}

// UpdateSite edits a site's name/geo/notes (ADMIN+).
func (h *SiteHandler) UpdateSite(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	siteID := c.Param("id")
	if !isUUID(siteID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}

	var req model.SiteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var site *model.Site
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		s, err := h.siteRepo.UpdateSiteTXN(tx, businessID, siteID, &req)
		if err != nil {
			return err
		}
		site = s
		return nil
	})
	if err != nil {
		if errors.Is(err, database.ErrSiteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"site": site})
}

// DeleteSite removes a site (ADMIN+). Its devices are detached, not deleted.
func (h *SiteHandler) DeleteSite(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	siteID := c.Param("id")
	if !isUUID(siteID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid site id"})
		return
	}

	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		return h.siteRepo.DeleteSiteTXN(tx, businessID, siteID)
	})
	if err != nil {
		if errors.Is(err, database.ErrSiteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
