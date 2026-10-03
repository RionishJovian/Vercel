package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/media/bridge"
)

type MediaPublicationHandler struct {
	Store            bridge.PublicationStore
	Ready            bool
	PurchasesEnabled bool
}

func (h *MediaPublicationHandler) available(c *gin.Context) bool {
	c.Header("Cache-Control", "no-store")
	if !h.Ready {
		c.JSON(503, gin.H{"error": "Media publishing requires the server publication migration. No design has been published."})
		return false
	}
	return true
}
func (h *MediaPublicationHandler) Get(c *gin.Context) {
	if !h.available(c) {
		return
	}
	p, e := h.Store.Publication(c.Request.Context(), auth.GetBusinessID(c))
	if errors.Is(e, sql.ErrNoRows) {
		c.JSON(200, gin.H{"publication": nil})
		return
	}
	if e != nil {
		c.JSON(503, gin.H{"error": "Published design could not be loaded."})
		return
	}
	c.JSON(200, gin.H{"publication": p})
}
func (h *MediaPublicationHandler) Publish(c *gin.Context) {
	if !h.available(c) {
		return
	}
	rev, e := strconv.ParseInt(c.Query("revision"), 10, 64)
	if e != nil || rev < 0 {
		c.JSON(400, gin.H{"error": "Expected publication revision is required."})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 3<<20)
	d, e := bridge.DecodeMediaDesign(c.Request.Body)
	if e != nil {
		c.JSON(400, gin.H{"error": "Invalid media design. Use whole-number pricing and resized PNG/JPEG pictures."})
		return
	}
	p, e := h.Store.Publish(c.Request.Context(), auth.GetBusinessID(c), d, rev)
	if errors.Is(e, bridge.ErrPublicationConflict) {
		c.JSON(409, gin.H{"error": "Another operator changed this publication. Reload before publishing."})
		return
	}
	if e != nil {
		c.JSON(503, gin.H{"error": "Publication was not confirmed. Reload before retrying."})
		return
	}
	c.JSON(200, gin.H{"publication": p})
}
func (h *MediaPublicationHandler) Unpublish(c *gin.Context) {
	if !h.available(c) {
		return
	}
	rev, e := strconv.ParseInt(c.Query("revision"), 10, 64)
	if e != nil || rev < 1 {
		c.Status(400)
		return
	}
	e = h.Store.Unpublish(c.Request.Context(), auth.GetBusinessID(c), rev)
	if e != nil {
		c.JSON(409, gin.H{"error": "Unpublishing was not confirmed. Reload the published design."})
		return
	}
	c.Status(204)
}
func (h *MediaPublicationHandler) Public(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.Ready || len(c.Param("id")) != 64 {
		c.Status(404)
		return
	}
	if _, e := bridge.Hash(c.Param("id")); e != nil {
		c.Status(404)
		return
	}
	p, e := h.Store.PublicPublication(c.Request.Context(), c.Param("id"))
	if e != nil {
		c.Status(404)
		return
	}
	// Never claim that publishing a theme enables paid access.
	c.JSON(200, gin.H{"design": p.Design, "revision": p.Revision, "published_at": p.PublishedAt, "purchases_enabled": h.PurchasesEnabled, "playback_enabled": false})
}
