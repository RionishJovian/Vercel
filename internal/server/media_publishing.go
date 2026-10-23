package server

import (
	"errors"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"mega-magic/internal/media/bridge"
	"mega-magic/internal/model"
)

// Hosted appearance is independent from a paired local Jellyfin computer.
func registerMediaPublishingRoutes(r *gin.Engine, service *auth.Service, db *database.DB, cfg *config.Config, access ...gin.HandlerFunc) error {
	var schemaReady bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname='magic_media_publications' AND c.relrowsecurity AND c.relforcerowsecurity)`).Scan(&schemaReady); err != nil {
		return err
	}
	if cfg.Media.PublishingEnabled && !schemaReady {
		return errors.New("media publication migration required before enabling publishing")
	}
	ready := schemaReady && (cfg.Media.PublishingEnabled || cfg.Media.ConnectorEnabled)
	owner := r.Group("/api/v1/media", auth.CookieOrigin(), auth.Middleware(service), auth.RequireActiveTenant(db), auth.RequireMinRole(model.RoleAdmin))
	owner.Use(access...)
	publishing := &handler.MediaPublicationHandler{Store: bridge.NewPostgres(db), Ready: ready, PurchasesEnabled: false}
	owner.GET("/publication", publishing.Get)
	owner.PUT("/publication", publishing.Publish)
	owner.DELETE("/publication", publishing.Unpublish)
	owner.GET("/capabilities", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		reason := "publishing_disabled"
		if ready {
			reason = "ready"
		} else if !schemaReady {
			reason = "publication_schema_unavailable"
		}
		c.JSON(200, gin.H{"publication": gin.H{"available": ready, "reason": reason}, "connector": gin.H{"available": cfg.Media.ConnectorEnabled}, "paid_access": gin.H{"available": false, "reason": "local_playback_acceptance_pending"}})
	})
	r.GET("/api/v1/media-guest/:id", auth.BoundedAuthRateLimit(), publishing.Public)
	return nil
}
