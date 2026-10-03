package server

import (
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"mega-magic/internal/media/bridge"
	"mega-magic/internal/model"
)

// Registered only after an explicit feature flag and schema check. Connector
// and owner surfaces never share authentication middleware or credentials.
func registerMediaRoutes(r *gin.Engine, service *auth.Service, db *database.DB, access ...gin.HandlerFunc) error {
	var ok bool
	if e := db.QueryRow(`SELECT count(*)=2 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname IN ('magic_media_connections','magic_media_jobs') AND c.relrowsecurity AND c.relforcerowsecurity`).Scan(&ok); e != nil {
		return e
	}
	if !ok {
		return bridge.ErrUnavailable
	}
	// The dashboard operation requires the additive constraint migration. Do not
	// start an opted-in release that would advertise jobs the database rejects.
	if e := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='public.magic_media_jobs'::regclass AND conname='magic_media_jobs_operation_check' AND position('dashboard' in pg_get_constraintdef(oid))>0)`).Scan(&ok); e != nil {
		return e
	}
	if !ok {
		return bridge.ErrUnavailable
	}
	h := handler.NewMediaHandler(bridge.NewPostgres(db))
	owner := r.Group("/api/v1/media", auth.CookieOrigin(), auth.Middleware(service), auth.RequireActiveTenant(db), auth.RequireMinRole(model.RoleAdmin))
	owner.Use(access...)
	owner.POST("/pairing", h.Pair)
	owner.GET("/connection", h.Connection)
	owner.DELETE("/connection", h.Revoke)
	owner.POST("/jobs", h.Enqueue)
	owner.GET("/jobs/:id", h.Job)
	// Additive rollout: an unmigrated server keeps the existing connector usable,
	// but cannot report a successful publication.
	var publishingReady bool
	if e := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname='magic_media_publications' AND c.relrowsecurity AND c.relforcerowsecurity)`).Scan(&publishingReady); e != nil {
		return e
	}
	publishing := &handler.MediaPublicationHandler{Store: bridge.NewPostgres(db), Ready: publishingReady}
	owner.GET("/publication", publishing.Get)
	owner.PUT("/publication", publishing.Publish)
	owner.DELETE("/publication", publishing.Unpublish)
	r.GET("/api/v1/media-guest/:id", auth.BoundedAuthRateLimit(), publishing.Public)
	connector := r.Group("/api/v1/media-connector", auth.BoundedAuthRateLimit(), func(c *gin.Context) {
		if c.GetHeader("Origin") != "" {
			c.AbortWithStatus(403)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	connector.POST("/redeem", h.Redeem)
	connector.POST("/poll", h.Poll)
	connector.POST("/jobs/:id/result", h.Complete)
	return nil
}
