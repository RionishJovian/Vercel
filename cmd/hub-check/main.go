// hub-check is a read-only operator diagnostic, not an authenticated app route.
// Pass non-secret peer identity JSON on stdin; credentials come from config/env.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"mega-magic/internal/config"
	"mega-magic/internal/hub"
)

func main() {
	configPath := flag.String("config", "config.yaml", "configuration path")
	flag.Parse()
	if err := check(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, "Hub diagnostic failed:", err)
		os.Exit(1)
	}
	fmt.Println("App-to-Hub Go lookup passed: verified HTTPS, signed existing peer, exact identity and cross-tenant denial. No writes requested.")
}

func check(path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("configuration invalid")
	}
	if !cfg.VPS.HubLookupEnabled {
		return fmt.Errorf("lookup disabled")
	}
	var target struct {
		TenantID string `json:"tenantId"`
		RouterID string `json:"routerId"`
		PeerID   string `json:"peerId"`
	}
	decoder := json.NewDecoder(io.LimitReader(os.Stdin, 4096))
	if decoder.Decode(&target) != nil || target.PeerID == "" {
		return fmt.Errorf("invalid diagnostic target")
	}
	client, err := hub.New(hub.Options{URL: cfg.VPS.HubURL, KeyID: cfg.VPS.HubKeyID, SigningKey: cfg.VPS.HubSigningKey})
	if err != nil {
		return fmt.Errorf("Hub configuration invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id := hub.Identity{TenantID: target.TenantID, RouterID: target.RouterID, RequestedBy: "hub-readonly-diagnostic"}
	peer, err := client.InspectRouter(ctx, id)
	if err != nil {
		return err
	}
	if peer.PeerID != target.PeerID {
		return fmt.Errorf("peer identity mismatch")
	}
	id.TenantID = "isolated-denied-tenant"
	if _, err := client.InspectRouter(ctx, id); err == nil || err.Error() != "Hub returned HTTP 404" {
		return fmt.Errorf("cross-tenant denial failed")
	}
	return nil
}
