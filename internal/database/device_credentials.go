package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/secretbox"
)

// DeviceCredentialSource resolves each device's OWN SSH target from the
// database. It is the piece that makes the per-device transport real rather
// than theoretical: before this, there was nowhere to store a router's SSH
// credentials at all (devices had no credential columns, and
// router_connections.legacy_password_ciphertext is from the retired REST stack
// and undecryptable by design).
type DeviceCredentialSource struct {
	db  *DB
	box *secretbox.Box
}

// NewDeviceCredentialSource wires the source to a database and a cipher.
// A nil box is allowed and means "no key configured": every lookup then fails
// with ErrNoTarget rather than attempting to read a plaintext secret.
func NewDeviceCredentialSource(db *DB, box *secretbox.Box) *DeviceCredentialSource {
	return &DeviceCredentialSource{db: db, box: box}
}

// TargetForDevice returns the peer's own connection facts.
//
// businessID is required and used in the WHERE clause. The sweep is a
// cross-tenant system path, so it cannot rely on RLS alone to stop a device id
// from one tenant being used to read another tenant's device row.
//
// Every "cannot use this" case returns routeros.ErrNoTarget:
//   - no such device in this tenant
//   - no management address
//   - credentials absent
//   - ciphertext that will not decrypt (wrong key / tampered / legacy)
//
// collapsing all of them to "no target configured" is deliberate. There is no
// branch in this function that yields a usable target it did not decrypt.
func (s *DeviceCredentialSource) TargetForDevice(ctx context.Context, businessID, deviceID string) (*routeros.DeviceTarget, error) {
	if s == nil || s.db == nil {
		return nil, routeros.ErrNoTarget
	}

	var (
		addr, user, cipherText, kind, fingerprint sql.NullString
		port                                      sql.NullInt32
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(host(management_ip), ''),
		       ssh_port,
		       ssh_username,
		       ssh_secret_ciphertext,
		       ssh_secret_kind,
		       ssh_host_key_fingerprint
		  FROM devices
		 WHERE device_id = $1
		   AND business_id = $2`, deviceID, businessID).
		Scan(&addr, &port, &user, &cipherText, &kind, &fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, routeros.ErrNoTarget
	}
	if err != nil {
		return nil, fmt.Errorf("device ssh target: %w", err)
	}

	hostPort := routeros.JoinHostPort(addr.String, int(port.Int32))
	if hostPort == "" || !user.Valid || !cipherText.Valid || !kind.Valid {
		return nil, routeros.ErrNoTarget
	}

	if s.box == nil {
		// No key: refuse. Never treat "cannot decrypt" as "empty password".
		return nil, routeros.ErrNoTarget
	}
	secret, err := s.box.Open(cipherText.String)
	if err != nil {
		return nil, routeros.ErrNoTarget
	}

	return &routeros.DeviceTarget{
		DeviceID:    deviceID,
		Addr:        hostPort,
		User:        user.String,
		Secret:      secret,
		Kind:        kind.String,
		Fingerprint: fingerprint.String,
	}, nil
}

// SetDeviceCredentials stores a device's SSH target, encrypting the secret.
//
// This is the ONLY supported writer. It refuses plaintext-at-rest by
// construction: the secret arrives as an argument and is sealed before it
// reaches SQL.
func (s *DeviceCredentialSource) SetDeviceCredentials(ctx context.Context, businessID, deviceID, address, username, kind, secret string, port int, fingerprint string) error {
	if s == nil || s.box == nil {
		return secretbox.ErrNoKey
	}
	if address == "" || secret == "" || username == "" {
		return errors.New("device ssh target: address, username and secret are required")
	}
	fingerprint = strings.TrimSpace(fingerprint)
	if err := routeros.ValidateHostKeyFingerprint(fingerprint); err != nil {
		return errors.New("device ssh target: valid SHA256 host-key fingerprint is required")
	}
	if kind != "password" && kind != "key" {
		return fmt.Errorf("device ssh target: unknown secret kind %q", kind)
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("device ssh target: invalid port %d", port)
	}

	sealed, err := s.box.Seal(secret)
	if err != nil {
		return fmt.Errorf("device ssh target: seal: %w", err)
	}

	if s.db == nil {
		return errors.New("device ssh target: database unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	err = WithTenant(tx, businessID, func() error {
		res, err := tx.ExecContext(ctx, `
		UPDATE devices
		   SET management_ip = $1::inet,
		       ssh_port = $2,
		       ssh_username = $3,
		       ssh_secret_ciphertext = $4,
		       ssh_secret_kind = $5,
		       ssh_host_key_fingerprint = $6,
		       updated_at = now()
		 WHERE device_id = $7
		   AND business_id = $8`,
			address, port, username, sealed, kind, fingerprint, deviceID, businessID)
		if err != nil {
			return fmt.Errorf("device ssh target: update: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			// Report a no-op loudly: a silently-scoped-away update would look like
			// success while the device stayed unreachable.
			return sql.ErrNoRows
		}
		return nil
	})
	if err != nil {
		return err
	}
	return tx.Commit()
}
