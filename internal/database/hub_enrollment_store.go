package database

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"
)

// RouterKeyLifetime is how long the application may keep a server-generated router
// private key for the legacy one-paste flow. It only has to survive a retry of the
// same paste; an expired key simply makes the next attempt allocate a fresh one.
// The key is also deleted as soon as a verified handshake is seen. The router-
// generated flow (HubCallbackTokenStore) never creates such a key at all.
const RouterKeyLifetime = time.Hour

// ErrCallbackTokenInvalid covers an unknown, already used or expired token. The
// cases are deliberately indistinguishable to the caller.
var ErrCallbackTokenInvalid = errors.New("callback token invalid, used or expired")

// ErrTunnelAddressInUse means the Hub allocation collides with another router's
// /32 address or key on the same Hub endpoint.
var ErrTunnelAddressInUse = errors.New("tunnel address or router key is already allocated to another router")

// HubCallbackTokenStore holds the single-use tokens of router-generated-key
// enrolment. Only a SHA-256 of each token is stored.
type HubCallbackTokenStore struct{ DB *DB }

func tokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Issue replaces any earlier token for the router, so exactly one is ever live.
func (s HubCallbackTokenStore) Issue(ctx context.Context, biz, device, requestedBy string, ttl time.Duration) (string, time.Time, error) {
	if !uuidRe.MatchString(biz) || !uuidRe.MatchString(device) || !uuidRe.MatchString(requestedBy) || ttl <= 0 || ttl > time.Hour {
		return "", time.Time{}, errors.New("invalid callback token request")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().UTC().Add(ttl).Truncate(time.Microsecond)
	_, err := s.DB.ExecContext(ctx, `INSERT INTO hub_callback_tokens(business_id,device_id,token_hash,requested_by,expires_at)
VALUES($1,$2,$3,$4,$5)
ON CONFLICT(business_id,device_id) DO UPDATE SET token_hash=excluded.token_hash,requested_by=excluded.requested_by,
 expires_at=excluded.expires_at,consumed_at=NULL,created_at=clock_timestamp()`, biz, device, tokenHash(token), requestedBy, expires)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// Consume atomically spends a live token and returns the router it was issued for.
// Two concurrent callers presenting the same token cannot both succeed.
func (s HubCallbackTokenStore) Consume(ctx context.Context, token string) (biz, device string, err error) {
	if len(token) != 43 {
		return "", "", ErrCallbackTokenInvalid
	}
	err = s.DB.QueryRowContext(ctx, `UPDATE hub_callback_tokens SET consumed_at=clock_timestamp()
WHERE token_hash=$1 AND consumed_at IS NULL AND expires_at>clock_timestamp()
RETURNING business_id::text,device_id::text`, tokenHash(token)).Scan(&biz, &device)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrCallbackTokenInvalid
	}
	return biz, device, err
}

// HubPeerState is the app's durable record of a router's Hub allocation. It never
// holds private key material.
type HubPeerState struct {
	BusinessID, DeviceID, PeerID string
	RouterPublicKey              string
	TunnelAddress, Endpoint      string
	LastHandshakeAt              *time.Time
}

type HubPeerStateStore struct{ DB *DB }

// Record stores the allocation for a router, replacing its earlier one. A /32 or
// router key already held by a different router on the same Hub endpoint is
// refused with ErrTunnelAddressInUse. A changed key resets the handshake time.
func (s HubPeerStateStore) Record(ctx context.Context, st HubPeerState) error {
	if !uuidRe.MatchString(st.BusinessID) || !uuidRe.MatchString(st.DeviceID) || st.PeerID == "" || st.Endpoint == "" {
		return errors.New("invalid peer state")
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO hub_peer_state(business_id,device_id,peer_id,router_public_key,tunnel_address,endpoint)
VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(business_id,device_id) DO UPDATE SET peer_id=excluded.peer_id,
 last_handshake_at=CASE WHEN hub_peer_state.router_public_key=excluded.router_public_key THEN hub_peer_state.last_handshake_at END,
 router_public_key=excluded.router_public_key,tunnel_address=excluded.tunnel_address,endpoint=excluded.endpoint,
 updated_at=clock_timestamp()`, st.BusinessID, st.DeviceID, st.PeerID, st.RouterPublicKey, st.TunnelAddress, st.Endpoint)
	if isUniqueViolation(err) {
		return ErrTunnelAddressInUse
	}
	return err
}

// TouchHandshake advances the last verified handshake time. It never moves it back.
func (s HubPeerStateStore) TouchHandshake(ctx context.Context, biz, device string, at time.Time) error {
	if !uuidRe.MatchString(biz) || !uuidRe.MatchString(device) {
		return errors.New("invalid peer state identity")
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE hub_peer_state SET last_handshake_at=GREATEST(COALESCE(last_handshake_at,'-infinity'::timestamptz),$3),updated_at=clock_timestamp()
WHERE business_id=$1 AND device_id=$2`, biz, device, at.UTC())
	return err
}

// Get returns nil, nil when the router has no recorded allocation.
func (s HubPeerStateStore) Get(ctx context.Context, biz, device string) (*HubPeerState, error) {
	if !uuidRe.MatchString(biz) || !uuidRe.MatchString(device) {
		return nil, errors.New("invalid peer state identity")
	}
	st := HubPeerState{BusinessID: biz, DeviceID: device}
	var handshake sql.NullTime
	err := s.DB.QueryRowContext(ctx, `SELECT peer_id,router_public_key,tunnel_address,endpoint,last_handshake_at FROM hub_peer_state WHERE business_id=$1 AND device_id=$2`, biz, device).
		Scan(&st.PeerID, &st.RouterPublicKey, &st.TunnelAddress, &st.Endpoint, &handshake)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if handshake.Valid {
		t := handshake.Time.UTC()
		st.LastHandshakeAt = &t
	}
	return &st, nil
}

// Delete forgets a router's allocation, for example after its Hub peer is removed.
func (s HubPeerStateStore) Delete(ctx context.Context, biz, device string) error {
	if !uuidRe.MatchString(biz) || !uuidRe.MatchString(device) {
		return errors.New("invalid peer state identity")
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM hub_peer_state WHERE business_id=$1 AND device_id=$2`, biz, device)
	return err
}

// CheckHubEnrollmentTables refuses to start enrolment unless both tables exist with
// forced row security and no tenant-role access, the same bar the pending-enrolment
// secrets table is held to. The deploy applies migrations before the API starts, so
// a failure here means the database does not match the release.
func CheckHubEnrollmentTables(db *DB) error {
	for _, table := range []string{"hub_callback_tokens", "hub_peer_state"} {
		var protected bool
		e := db.QueryRow(`SELECT relrowsecurity AND relforcerowsecurity AND NOT has_table_privilege('authenticated',oid,'SELECT,INSERT,UPDATE,DELETE') FROM pg_class WHERE oid=to_regclass('public.'||$1)`, table).Scan(&protected)
		if e != nil || !protected {
			return errors.New("hub enrollment tables or their protections are unavailable: " + table)
		}
	}
	return nil
}
