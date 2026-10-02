package server

import (
	"mega-magic/internal/config"
	"mega-magic/internal/handler"
)

// Set the policy together; enabling an individual legacy-gate bypass without
// shared access enforcement is never a supported startup configuration.
func configureSubscriptionHandlers(cfg config.BillingConfig, a *handler.AuthHandler, r *handler.RouterHandler, s *handler.SiteHandler, store *handler.StoreHandler, t *handler.TopologyHandler, v *handler.VPSHandler, ai *handler.MagicDudeHandler) {
	a.SubscriptionOnly = cfg.SubscriptionOnly
	a.SubscriptionExistingPolicy = cfg.ExistingAccountPolicy
	r.SubscriptionOnly = cfg.SubscriptionOnly
	s.SubscriptionOnly = cfg.SubscriptionOnly
	store.SubscriptionOnly = cfg.SubscriptionOnly
	t.SubscriptionOnly = cfg.SubscriptionOnly
	v.SubscriptionOnly = cfg.SubscriptionOnly
	ai.SubscriptionOnly = cfg.SubscriptionOnly
}
