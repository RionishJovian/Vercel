package server

import (
	"fmt"
	"net/http"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/secretbox"
	"mega-magic/internal/webfig"
)

// configureWebFig wires cloud WebFig. It returns nil, and the routes say off,
// unless live router writes, a Hub source address and the proxy's own hostname
// all exist. MM_WEBFIG_ACCESS still decides whether anything is served.
func configureWebFig(h *handler.SecurityHandler, db *database.DB, cfg *config.Config) (*webfig.Proxy, error) {
	host, appCfg := webfig.HostFromEnv(), cfg.Provisioner.Applier
	if host == "" || !appCfg.Enabled || cfg.WireGuard.ManagementSourceCIDR == "" {
		return nil, nil
	}
	box, err := secretbox.New(appCfg.CredentialKey)
	if err != nil {
		return nil, fmt.Errorf("webfig credential key: %w", err)
	}
	targets, err := routeros.NewCIDRTargetSource(database.NewDeviceCredentialSource(db, box), cfg.Provisioner.AllowedTargetCIDRs)
	if err != nil {
		return nil, fmt.Errorf("webfig target policy: %w", err)
	}
	router := routeros.NewDeviceApplier(targets, appCfg.Timeout, nil, appCfg.Interface, appCfg.ListenPortRotateTo, appCfg.ResetHold)
	store := webfig.NewStore()
	h.EnableWebFig(router, targets, store, host, cfg.WireGuard.ManagementSourceCIDR)
	return webfig.NewProxy(host, store, handler.WebFigChecker(db, targets)), nil
}

// withWebFigHost sends only the WebFig hostname to the proxy, before any
// application route or middleware. No other host can reach a router this way, and
// the WebFig host cannot reach the application.
func withWebFigHost(proxy *webfig.Proxy, app http.Handler) http.Handler {
	if proxy == nil {
		return app
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if proxy.Handles(r) {
			proxy.ServeHTTP(w, r)
			return
		}
		app.ServeHTTP(w, r)
	})
}
