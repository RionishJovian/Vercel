package hub

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

type ManagementMove struct {
	PeerID          string `json:"peerId"`
	TenantID        string `json:"tenantId"`
	RouterID        string `json:"routerId"`
	RouterPublicKey string `json:"routerPublicKey"`
	OperationID     string `json:"operationId"`
	OldAddress      string `json:"oldAddress"`
	NewAddress      string `json:"newAddress"`
	Phase           string `json:"phase"`
	ExpiresAt       string `json:"expiresAt"`
}

func (c *Client) ManagementMigration(ctx context.Context, id Identity, action string, move ManagementMove) (ManagementMove, error) {
	if !identifier.MatchString(id.TenantID) || !identifier.MatchString(id.RouterID) || !identifier.MatchString(id.RequestedBy) {
		return ManagementMove{}, errors.New("invalid migration identity")
	}
	method := http.MethodPost
	if action == "inspect" || action == "candidate" {
		method = http.MethodGet
	}
	switch action {
	case "inspect", "candidate", "stage", "commit", "finalize", "abort":
	default:
		return ManagementMove{}, errors.New("invalid migration action")
	}
	path := "/v1/peers/by-router/" + id.RouterID + "/management-migration/" + action
	payload := []byte{}
	if method == http.MethodPost {
		payload, _ = json.Marshal(struct {
			OperationID     string `json:"operationId"`
			OldAddress      string `json:"oldAddress"`
			NewAddress      string `json:"newAddress"`
			RouterPublicKey string `json:"routerPublicKey"`
		}{move.OperationID, move.OldAddress, move.NewAddress, move.RouterPublicKey})
	}
	random := make([]byte, 16)
	if _, e := rand.Read(random); e != nil {
		return ManagementMove{}, e
	}
	rid := hex.EncodeToString(random)
	stamp := c.now().UTC().Format(time.RFC3339Nano)
	req, e := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(payload))
	if e != nil {
		return ManagementMove{}, errors.New("migration request unavailable")
	}
	canonical := strings.Join([]string{method, path, stamp, rid, "peers:migrate", id.TenantID, id.RouterID, id.RequestedBy, string(payload)}, "\n")
	for k, v := range map[string]string{"x-mm-key-id": c.keyID, "x-mm-timestamp": stamp, "x-mm-request-id": rid, "x-mm-scope": "peers:migrate", "x-mm-tenant-id": id.TenantID, "x-mm-router-id": id.RouterID, "x-mm-requested-by": id.RequestedBy, "x-mm-signature": signature(c.key, canonical), "Content-Type": "application/json"} {
		req.Header.Set(k, v)
	}
	res, e := c.http.Do(req)
	if e != nil {
		return ManagementMove{}, errors.New("Hub migration request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return ManagementMove{}, errors.New("Hub migration unavailable or allocation changed; no fallback enrollment is allowed")
	}
	raw, e := io.ReadAll(io.LimitReader(res.Body, 16385))
	if e != nil || len(raw) > 16384 {
		return ManagementMove{}, errors.New("invalid Hub migration response")
	}
	var out ManagementMove
	if json.Unmarshal(raw, &out) != nil || out.TenantID != id.TenantID || out.RouterID != id.RouterID || !identifier.MatchString(out.PeerID) {
		return ManagementMove{}, errors.New("mismatched Hub migration response")
	}
	old, e := netip.ParsePrefix(out.OldAddress)
	if e != nil || old.Bits() != 32 || !old.Addr().Is4() || !old.Addr().IsPrivate() {
		return ManagementMove{}, errors.New("invalid old migration address")
	}
	if action != "inspect" || out.Phase != "none" {
		next, e := netip.ParsePrefix(out.NewAddress)
		if e != nil || next.Bits() != 32 || !next.Addr().Is4() || !next.Addr().IsPrivate() || next == old {
			return ManagementMove{}, errors.New("invalid next migration address")
		}
	}
	if method == http.MethodPost && (out.OperationID != move.OperationID || out.RouterPublicKey != move.RouterPublicKey || out.OldAddress != move.OldAddress || out.NewAddress != move.NewAddress) {
		return ManagementMove{}, errors.New("Hub migration identity changed")
	}
	return out, nil
}
