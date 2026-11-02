package cache

import "testing"

// Keys are the tenant boundary in Redis: two businesses must never share a key, and a business id must not be able to
// collide with a global key.
func TestKeysAreNamespacedPerTenant(t *testing.T) {
	c := &Client{prefix: "mm"}
	if got := c.Key("biz-1", "sessions"); got != "mm:biz-1:sessions" {
		t.Fatalf("Key = %q", got)
	}
	if c.Key("biz-1", "x") == c.Key("biz-2", "x") {
		t.Fatal("two tenants share a key")
	}
	if got := c.GlobalKey("rate"); got != "mm:rate" {
		t.Fatalf("GlobalKey = %q", got)
	}
}
