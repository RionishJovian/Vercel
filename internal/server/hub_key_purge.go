package server

import (
	"context"
	"errors"
	"regexp"

	"mega-magic/internal/database"
)

// redactedRouterKey replaces a WireGuard private key inside a stored setup script.
const redactedRouterKey = `private-key="removed-after-handshake"`

var privateKeyAssignment = regexp.MustCompile(`private-key="[^"]*"`)

// redactManagementTunnelKey scrubs the router's WireGuard private key from the
// stored one-paste management envelope. The envelope has to outlive the handshake
// (it still holds the management SSH key used to finish the connection), but the
// tunnel script inside it is only ever needed to reissue a paste, which is refused
// once the router has connected. Redacting is idempotent.
func redactManagementTunnelKey(raw string) (string, error) {
	p, err := decodePendingManagement(raw)
	if err != nil {
		return "", err
	}
	p.tunnelScript = privateKeyAssignment.ReplaceAllString(p.tunnelScript, redactedRouterKey)
	return encodePendingManagement(p)
}

// purgeRouterKeyMaterial removes every server-held copy of a router's WireGuard
// private key: the dedicated router_key row and the copy embedded in the
// management envelope. It is called once the router has proven it holds the key by
// completing a handshake.
func purgeRouterKeyMaterial(store database.PendingEnrollmentStore) func(context.Context, string, string) error {
	return func(ctx context.Context, biz, device string) error {
		return errors.Join(
			store.DeletePurpose(ctx, biz, device, "router_key"),
			store.Rewrite(ctx, biz, device, "management", redactManagementTunnelKey),
		)
	}
}
