package server

import (
	"mega-magic/internal/config"
	"mega-magic/internal/handler"
)

// Set the policy together; enabling an individual legacy-gate bypass without
// shared access enforcement is never a supported startup configuration.
func configureSubscriptionHandlers(cfg config.BillingConfig, a *handler.AuthHandler, r *handler.RouterHandler, s *handler.SiteHandler, store *handler.StoreHandler, t *handler.TopologyHandler, v *handler.VPSHandler, ai *handler.MagicDudeHandler) {
	a.SubscriptionOnly = cfg.SubscriptionOnly
	a.IndividualPassesEnabled = cfg.IndividualPassesEnabled
	a.SubscriptionExistingPolicy = cfg.ExistingAccountPolicy
	r.SubscriptionOnly = cfg.SubscriptionOnly || cfg.IndividualPassesEnabled
	s.SubscriptionOnly = cfg.SubscriptionOnly || cfg.IndividualPassesEnabled
	store.SubscriptionOnly = cfg.SubscriptionOnly
	store.IndividualPassesEnabled = cfg.IndividualPassesEnabled
	t.SubscriptionOnly = cfg.SubscriptionOnly || cfg.IndividualPassesEnabled
	v.SubscriptionOnly = cfg.SubscriptionOnly || cfg.IndividualPassesEnabled
	ai.SubscriptionOnly = cfg.SubscriptionOnly || cfg.IndividualPassesEnabled
}
