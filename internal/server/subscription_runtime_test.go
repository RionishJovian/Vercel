package server

import (
	"mega-magic/internal/config"
	"mega-magic/internal/handler"
	"testing"
)

func TestSubscriptionSwitchesConfiguredTogether(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		a, r, s, store, tops, v, ai := &handler.AuthHandler{}, &handler.RouterHandler{}, &handler.SiteHandler{}, &handler.StoreHandler{}, &handler.TopologyHandler{}, &handler.VPSHandler{}, &handler.MagicDudeHandler{}
		configureSubscriptionHandlers(config.BillingConfig{SubscriptionOnly: enabled, ExistingAccountPolicy: "require_payment"}, a, r, s, store, tops, v, ai)
		for _, got := range []bool{a.SubscriptionOnly, r.SubscriptionOnly, s.SubscriptionOnly, store.SubscriptionOnly, tops.SubscriptionOnly, v.SubscriptionOnly, ai.SubscriptionOnly} {
			if got != enabled {
				t.Fatal("partial subscription configuration")
			}
		}
		if a.SubscriptionExistingPolicy != "require_payment" {
			t.Fatal("missing existing-account policy")
		}
	}
}
