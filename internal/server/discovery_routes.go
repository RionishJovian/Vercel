package server

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/discoverystore"
	"mega-magic/internal/handler"
	"mega-magic/internal/model"
)

// Registered only when discovery.enabled is set and its tables exist. Discovery has no router access: it stores
// unverified observations a paired helper uploads, and it never changes configuration.
// The owner surface (cookie, Owner/Admin, subscription checks) and the helper surface (code or token only) share
// no middleware or credentials.
func registerDiscoveryRoutes(r *gin.Engine, service *auth.Service, db *database.DB, access ...gin.HandlerFunc) (*discoverystore.Store, error) {
	var ok bool
	if e := db.QueryRow(`SELECT count(*)=2 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname IN ('discovery_sessions','discovery_observations')`).Scan(&ok); e != nil {
		return nil, e
	}
	if !ok {
		return nil, errors.New("discovery is enabled but its schema is not migrated")
	}
	store := discoverystore.New(db)
	h := handler.NewDiscoveryHandler(store)

	owner := []gin.HandlerFunc{auth.CookieOrigin(), auth.Middleware(service), auth.RequireActiveTenant(db), auth.RequireMinRole(model.RoleAdmin)}
	owner = append(owner, access...)
	// A helper is not a browser: any Origin header is refused, nothing is cached, and sign-in rate limits apply.
	helper := []gin.HandlerFunc{auth.BoundedAuthRateLimit(), func(c *gin.Context) {
		if c.GetHeader("Origin") != "" {
			c.AbortWithStatus(403)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Next()
	}}
	with := func(mw []gin.HandlerFunc, fn gin.HandlerFunc) []gin.HandlerFunc {
		return append(append([]gin.HandlerFunc{}, mw...), fn)
	}

	const base = "/api/v1/discovery/sessions"
	r.POST(base, with(owner, h.CreateSession)...)
	r.GET(base+"/:id", with(owner, h.GetSession)...)
	r.DELETE(base+"/:id", with(owner, h.CancelSession)...)
	r.POST(base+"/:id/select", with(owner, h.SelectCandidate)...)
	r.POST(base+"/:id/link", with(owner, h.LinkRouter)...)
	r.POST(base+"/:id/pair", with(helper, h.Pair)...)
	r.POST(base+"/:id/observations", with(helper, h.Observations)...)
	return store, nil
}

// runDiscoverySweep ends and deletes old discovery sessions until ctx ends.
func runDiscoverySweep(ctx context.Context, store *discoverystore.Store, warn func(string, ...any)) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		if err := store.Sweep(ctx); err != nil && ctx.Err() == nil {
			warn("discovery sweep failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
