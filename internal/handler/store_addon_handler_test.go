package handler

import (
	"strings"
	"testing"
)

func intp(n int) *int { return &n }

func validAddon() storeAddonInput {
	return storeAddonInput{Key: "extra_router_slot", Kind: "router_slot", NameEN: "Extra router", NameMY: "Router တိုးမည်",
		PriceMMK: intp(5000), Period: "monthly"}
}

func TestValidateStoreAddon(t *testing.T) {
	if err := validateStoreAddon(validAddon()); err != nil {
		t.Fatalf("valid add-on rejected: %v", err)
	}
	cases := map[string]func(*storeAddonInput){
		"bad key":        func(a *storeAddonInput) { a.Key = "Bad Key!" },
		"short key":      func(a *storeAddonInput) { a.Key = "ab" },
		"bad kind":       func(a *storeAddonInput) { a.Kind = "gift" },
		"bad period":     func(a *storeAddonInput) { a.Period = "weekly" },
		"empty english":  func(a *storeAddonInput) { a.NameEN = "  " },
		"empty burmese":  func(a *storeAddonInput) { a.NameMY = "" },
		"long name":      func(a *storeAddonInput) { a.NameEN = strings.Repeat("a", 121) },
		"long desc":      func(a *storeAddonInput) { a.DescriptionMY = strings.Repeat("က", 601) },
		"missing price":  func(a *storeAddonInput) { a.PriceMMK = nil },
		"negative price": func(a *storeAddonInput) { a.PriceMMK = intp(-1) },
		"huge price":     func(a *storeAddonInput) { a.PriceMMK = intp(100000001) },
	}
	for name, mutate := range cases {
		a := validAddon()
		mutate(&a)
		if err := validateStoreAddon(a); err == nil {
			t.Errorf("%s should be rejected", name)
		}
	}
	free := validAddon()
	free.PriceMMK = intp(0)
	if err := validateStoreAddon(free); err != nil {
		t.Errorf("a free add-on is allowed: %v", err)
	}
}
