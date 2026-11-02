package database

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"mega-magic/internal/secretbox"
)

// Integration secrets that the app must read back in plaintext (a Telegram bot token to call the Bot API, a
// webhook secret to sign deliveries) are stored sealed with the same key as the router credentials. A sealed
// value carries the prefix below, so values written before encryption existed are still readable until
// EncryptLegacyFieldSecrets rewrites them; nothing else in the database can be mistaken for a sealed value.
const sealedPrefix = "enc:v1:"

var fieldBox atomic.Pointer[secretbox.Box]

// SetFieldBox turns encryption on for new writes. Without a box (tests, local development) values stay plaintext.
// Production start-up refuses to run without a key (see server.New).
func SetFieldBox(b *secretbox.Box) { fieldBox.Store(b) }

func sealField(plain string) (string, error) {
	b := fieldBox.Load()
	if b == nil || plain == "" || strings.HasPrefix(plain, sealedPrefix) {
		return plain, nil
	}
	sealed, err := b.Seal(plain)
	if err != nil {
		return "", fmt.Errorf("seal secret: %w", err)
	}
	return sealedPrefix + sealed, nil
}

// openField returns the plaintext. A sealed value that cannot be opened (no key, wrong key, tampered) is an error:
// returning it unchanged would send the ciphertext to Telegram or sign webhooks with it.
func openField(stored string) (string, error) {
	if !strings.HasPrefix(stored, sealedPrefix) {
		return stored, nil
	}
	b := fieldBox.Load()
	if b == nil {
		return "", secretbox.ErrNoKey
	}
	plain, err := b.Open(strings.TrimPrefix(stored, sealedPrefix))
	if err != nil {
		return "", fmt.Errorf("open secret: %w", err)
	}
	return plain, nil
}

// EncryptLegacyFieldSecrets seals every bot token and webhook secret still stored in plaintext. It is idempotent and
// runs at start-up on the service connection (cross-tenant). It does nothing when no key is configured.
func EncryptLegacyFieldSecrets(ctx context.Context, db *DB) (bots, hooks int, err error) {
	if fieldBox.Load() == nil {
		return 0, 0, nil
	}
	bots, err = sealColumn(ctx, db, "business_telegram_bots", "business_id", "bot_token")
	if err != nil {
		return bots, 0, err
	}
	hooks, err = sealColumn(ctx, db, "webhook_endpoints", "id", "secret")
	return bots, hooks, err
}

func sealColumn(ctx context.Context, db *DB, table, key, col string) (int, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		`SELECT %s::text, %s FROM %s WHERE %s IS NOT NULL AND %s <> '' AND %s NOT LIKE $1`,
		key, col, table, col, col, col), sealedPrefix+"%")
	if err != nil {
		return 0, fmt.Errorf("list plaintext %s.%s: %w", table, col, err)
	}
	type row struct{ id, plain string }
	var todo []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.plain); err != nil {
			rows.Close()
			return 0, err
		}
		todo = append(todo, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	done := 0
	for _, r := range todo {
		sealed, err := sealField(r.plain)
		if err != nil {
			return done, err
		}
		// The WHERE clause keeps a concurrent edit (already sealed, or changed) from being overwritten.
		res, err := db.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET %s = $1 WHERE %s::text = $2 AND %s = $3`, table, col, key, col), sealed, r.id, r.plain)
		if err != nil {
			return done, fmt.Errorf("seal %s.%s: %w", table, col, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			done++
		}
	}
	return done, nil
}
