// Package discoverystore keeps short-lived discovery sessions and the unverified observations a paired helper uploads.
// It never contacts a router and has no router client: discovery cannot change configuration.
package discoverystore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/lib/pq"
	"mega-magic/internal/database"
)

const (
	SessionTTL       = 15 * time.Minute
	PairTTL          = 10 * time.Minute
	MaxPairAttempts  = 5
	MaxObservations  = 64
	RetainAfterEnd   = time.Hour
	shortAlphabet    = "ABCDEFGHJKMNPQRSTVWXYZ" // no I, L, O, U: matches the guest code alphabet
	shortLen, secLen = 6, 8
)

// ErrUnavailable is the one error callers see for any pairing or token failure, so responses reveal nothing about which part failed.
var (
	ErrUnavailable = errors.New("discovery session unavailable")
	ErrLiveSession = errors.New("a discovery session is already open")
	ErrBadSite     = errors.New("site not found")
)

type Store struct{ db *database.DB }

func New(db *database.DB) *Store { return &Store{db: db} }

// Created is returned once; the code is never stored in clear.
type Created struct {
	SessionID     string    `json:"session_id"`
	PairingCode   string    `json:"pairing_code"`
	ExpiresAt     time.Time `json:"expires_at"`
	PairExpiresAt time.Time `json:"pairing_expires_at"`
}

type Candidate struct {
	DeviceKey  string    `json:"device_key"`
	MACs       []string  `json:"macs"`
	Identity   string    `json:"identity,omitempty"`
	Board      string    `json:"board,omitempty"`
	Platform   string    `json:"platform,omitempty"`
	Version    string    `json:"version,omitempty"`
	Interface  string    `json:"interface,omitempty"`
	ClaimedIP  string    `json:"claimed_ip,omitempty"`
	SourceIP   string    `json:"source_ip,omitempty"`
	IPMismatch bool      `json:"ip_mismatch,omitempty"`
	IPChanged  bool      `json:"ip_changed,omitempty"`
	HeardCount int       `json:"heard_count"`
	LastSeen   time.Time `json:"last_seen"`
	Selected   bool      `json:"selected"`
	// Existing is set when this device was already registered from a discovery. A hint, not proof of identity.
	Existing *ExistingRouter `json:"existing_router,omitempty"`
	// Status is always "nearby_unverified": hearing a device proves nothing about ownership, connection or online state.
	Status string `json:"status"`
}

// ExistingRouter names the registered router a discovered device became.
type ExistingRouter struct {
	DeviceID string `json:"device_id"`
	Name     string `json:"name"`
}

type View struct {
	SessionID string     `json:"session_id"`
	State     string     `json:"state"` // open, paired, cancelled, closed, expired
	SiteID    string     `json:"site_id,omitempty"`
	ExpiresAt time.Time  `json:"expires_at"`
	PairedAt  *time.Time `json:"paired_at,omitempty"`
	// PairExpiresAt is set while the pairing code can still be used; the console shows when it runs out.
	PairExpiresAt *time.Time  `json:"pairing_expires_at,omitempty"`
	Candidates    []Candidate `json:"candidates"`
}

func hashOf(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func randomFrom(alphabet string, n int) (string, error) {
	out := make([]byte, n)
	max := big.NewInt(int64(len(alphabet)))
	for i := range out {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = alphabet[v.Int64()]
	}
	return string(out), nil
}

func (s *Store) tenant(ctx context.Context, bid string, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = database.WithTenant(tx, bid, func() error { return fn(tx) }); err != nil {
		return err
	}
	return tx.Commit()
}

// Create opens a session. Sessions that ran out are closed first, so only a genuinely live one blocks a new one.
func (s *Store) Create(ctx context.Context, bid, userID, siteID string) (Created, error) {
	var out Created
	secret, err := randomFrom(shortAlphabet, secLen)
	if err != nil {
		return out, err
	}
	err = s.tenant(ctx, bid, func(tx *sql.Tx) error {
		if siteID != "" {
			var ok bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sites WHERE site_id=$1 AND business_id=$2)`, siteID, bid).Scan(&ok); err != nil || !ok {
				return ErrBadSite
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE discovery_sessions SET state='closed', ended_at=now(), pair_hash=NULL, token_hash=NULL WHERE business_id=$1 AND state IN ('open','paired') AND expires_at<=now()`, bid); err != nil {
			return err
		}
		for attempt := 0; attempt < 5; attempt++ {
			short, err := randomFrom(shortAlphabet, shortLen)
			if err != nil {
				return err
			}
			var site any
			if siteID != "" {
				site = siteID
			}
			err = tx.QueryRowContext(ctx, `INSERT INTO discovery_sessions(business_id,created_by,site_id,short_id,pair_hash,pair_expires_at,expires_at)
				VALUES($1,$2,$3,$4,$5,now()+$6::interval,now()+$7::interval) RETURNING session_id,expires_at,pair_expires_at`,
				bid, userID, site, short, hashOf(secret), PairTTL.String(), SessionTTL.String()).Scan(&out.SessionID, &out.ExpiresAt, &out.PairExpiresAt)
			if err == nil {
				out.PairingCode = short + "-" + secret
				return nil
			}
			if isUnique(err, "discovery_sessions_one_live") {
				return ErrLiveSession
			}
			if !isUnique(err, "discovery_sessions_short_live") {
				return err
			}
		}
		return errors.New("could not allocate a pairing code")
	})
	return out, err
}

func isUnique(err error, index string) bool {
	var pe *pq.Error
	return errors.As(err, &pe) && pe.Code == "23505" && pe.Constraint == index
}

// Get returns the session and its candidates for the owning business. An unknown or foreign id is ErrUnavailable.
func (s *Store) Get(ctx context.Context, bid, id string) (View, error) {
	v := View{Candidates: []Candidate{}}
	err := s.tenant(ctx, bid, func(tx *sql.Tx) error {
		var site sql.NullString
		var paired, pairExp sql.NullTime
		var state string
		var expired bool
		err := tx.QueryRowContext(ctx, `SELECT session_id,state,site_id,expires_at,paired_at,pair_expires_at,(expires_at<=now()) FROM discovery_sessions WHERE session_id=$1 AND business_id=$2`, id, bid).
			Scan(&v.SessionID, &state, &site, &v.ExpiresAt, &paired, &pairExp, &expired)
		if err != nil {
			return ErrUnavailable
		}
		if expired && (state == "open" || state == "paired") {
			state = "expired"
		}
		v.State, v.SiteID = state, site.String
		if pairExp.Valid && state == "open" {
			t := pairExp.Time
			v.PairExpiresAt = &t
		}
		if paired.Valid {
			t := paired.Time
			v.PairedAt = &t
		}
		rows, err := tx.QueryContext(ctx, `SELECT o.device_key,o.macs,COALESCE(o.identity,''),COALESCE(o.board,''),COALESCE(o.platform,''),COALESCE(o.version,''),COALESCE(o.interface_name,''),
			COALESCE(host(o.claimed_ip),''),COALESCE(host(o.source_ip),''),o.ip_mismatch,o.ip_changed,o.heard_count,o.last_received_at,o.selected_at IS NOT NULL,
			COALESCE(l.device_id::text,''),COALESCE(dv.hardware_name,'')
			FROM discovery_observations o LEFT JOIN discovery_links l ON l.business_id=o.business_id AND l.device_key=o.device_key
			LEFT JOIN devices dv ON dv.device_id=l.device_id AND dv.business_id=l.business_id
			WHERE o.session_id=$1 AND o.business_id=$2 ORDER BY o.last_received_at DESC, o.device_key`, id, bid)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c Candidate
			var macs pq.StringArray
			var linkedID, linkedName string
			if err := rows.Scan(&c.DeviceKey, &macs, &c.Identity, &c.Board, &c.Platform, &c.Version, &c.Interface, &c.ClaimedIP, &c.SourceIP, &c.IPMismatch, &c.IPChanged, &c.HeardCount, &c.LastSeen, &c.Selected, &linkedID, &linkedName); err != nil {
				return err
			}
			if linkedID != "" {
				c.Existing = &ExistingRouter{DeviceID: linkedID, Name: linkedName}
			}
			c.MACs, c.Status = []string(macs), "nearby_unverified"
			v.Candidates = append(v.Candidates, c)
		}
		return rows.Err()
	})
	return v, err
}

// Cancel ends the session and revokes its code and token.
func (s *Store) Cancel(ctx context.Context, bid, id string) error {
	return s.tenant(ctx, bid, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE discovery_sessions SET state='cancelled', ended_at=now(), pair_hash=NULL, token_hash=NULL WHERE session_id=$1 AND business_id=$2 AND state IN ('open','paired')`, id, bid)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrUnavailable
		}
		return nil
	})
}

// Sweep ends sessions that ran out and deletes sessions (and their observations) a while after they ended.
// It runs on the owner connection because it spans tenants and touches no router.
func (s *Store) Sweep(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE discovery_sessions SET state='closed', ended_at=now(), pair_hash=NULL, token_hash=NULL WHERE state IN ('open','paired') AND expires_at<=now()`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM discovery_sessions WHERE (ended_at IS NOT NULL AND ended_at<now()-$1::interval) OR created_at<now()-interval '24 hours'`, RetainAfterEnd.String())
	return err
}

var codePattern = regexp.MustCompile(`^([A-HJKMNP-TV-Z]{6})-([A-HJKMNP-TV-Z]{8})$`)

// Paired is what a helper receives once, in exchange for a valid code.
type Paired struct {
	SessionID string    `json:"session_id"`
	Token     string    `json:"helper_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Pair exchanges a pairing code for a helper token. The code is single-use. A wrong secret counts as an attempt and the
// session closes after MaxPairAttempts. Every failure is ErrUnavailable. The business is found from the code, never supplied.
func (s *Store) Pair(ctx context.Context, code string) (Paired, error) {
	m := codePattern.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(code)))
	if m == nil {
		return Paired{}, ErrUnavailable
	}
	var id, bid string
	err := s.db.QueryRowContext(ctx, `SELECT d.session_id,d.business_id FROM discovery_sessions d JOIN businesses b USING(business_id)
		WHERE b.is_active AND d.short_id=$1 AND d.state='open' AND d.pair_expires_at>now() AND d.expires_at>now()`, m[1]).Scan(&id, &bid)
	if err != nil {
		return Paired{}, ErrUnavailable
	}
	token, err := randomFrom("0123456789abcdef", 64)
	if err != nil {
		return Paired{}, err
	}
	var out Paired
	var good bool
	err = s.tenant(ctx, bid, func(tx *sql.Tx) error {
		var stored sql.NullString
		var attempts int
		if err := tx.QueryRowContext(ctx, `SELECT pair_hash,pair_attempts FROM discovery_sessions WHERE session_id=$1 AND business_id=$2 AND state='open' AND pair_expires_at>now() AND expires_at>now() FOR UPDATE`, id, bid).Scan(&stored, &attempts); err != nil {
			return nil
		}
		if !stored.Valid || attempts >= MaxPairAttempts {
			return nil
		}
		want := hashOf(m[2])
		if subtle.ConstantTimeCompare([]byte(stored.String), []byte(want)) != 1 {
			// Count the miss and, at the limit, end the session so the code cannot be guessed further.
			_, err := tx.ExecContext(ctx, `UPDATE discovery_sessions SET pair_attempts=pair_attempts+1,
				state=CASE WHEN pair_attempts+1>=$3 THEN 'closed' ELSE state END,
				ended_at=CASE WHEN pair_attempts+1>=$3 THEN now() ELSE ended_at END,
				pair_hash=CASE WHEN pair_attempts+1>=$3 THEN NULL ELSE pair_hash END
				WHERE session_id=$1 AND business_id=$2`, id, bid, MaxPairAttempts)
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE discovery_sessions SET state='paired',pair_hash=NULL,pair_expires_at=NULL,token_hash=$3,token_expires_at=expires_at,paired_at=now()
			WHERE session_id=$1 AND business_id=$2 AND state='open' AND pair_hash=$4`, id, bid, hashOf(token), want)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			good = true
			err = tx.QueryRowContext(ctx, `SELECT expires_at FROM discovery_sessions WHERE session_id=$1 AND business_id=$2`, id, bid).Scan(&out.ExpiresAt)
		}
		return err
	})
	if err != nil {
		return Paired{}, err
	}
	if !good {
		return Paired{}, ErrUnavailable
	}
	out.SessionID, out.Token = id, token
	return out, nil
}

// ObservationIn is one unverified announcement as validated by the handler.
type ObservationIn struct {
	DeviceKey  string
	MACs       []string
	Identity   string
	Board      string
	Platform   string
	Version    string
	Interface  string
	ClaimedIP  string
	SourceIP   string
	IPMismatch bool
	IPChanged  bool
	HeardCount int
}

var (
	// ErrTooFast means the helper uploaded again before MinUploadGap.
	ErrTooFast = errors.New("uploads too frequent")
)

const MinUploadGap = 2 * time.Second

// AddObservations stores what a paired helper heard. The token resolves the session; a session id from the caller is only checked against it.
func (s *Store) AddObservations(ctx context.Context, token, sessionID string, in []ObservationIn) (accepted, dropped int, err error) {
	if len(token) != 64 || !isHex(token) {
		return 0, 0, ErrUnavailable
	}
	var id, bid string
	if err := s.db.QueryRowContext(ctx, `SELECT d.session_id,d.business_id FROM discovery_sessions d JOIN businesses b USING(business_id)
		WHERE b.is_active AND d.token_hash=$1 AND d.state='paired' AND d.token_expires_at>now() AND d.expires_at>now()`, hashOf(token)).Scan(&id, &bid); err != nil || id != sessionID {
		return 0, 0, ErrUnavailable
	}
	err = s.tenant(ctx, bid, func(tx *sql.Tx) error {
		var last sql.NullTime
		var tooFast bool
		if err := tx.QueryRowContext(ctx, `SELECT last_upload_at,(last_upload_at IS NOT NULL AND last_upload_at>now()-$3::interval) FROM discovery_sessions
			WHERE session_id=$1 AND business_id=$2 AND state='paired' AND expires_at>now() FOR UPDATE`, id, bid, MinUploadGap.String()).Scan(&last, &tooFast); err != nil {
			return ErrUnavailable
		}
		if tooFast {
			return ErrTooFast
		}
		var have int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM discovery_observations WHERE session_id=$1 AND business_id=$2`, id, bid).Scan(&have); err != nil {
			return err
		}
		for _, o := range in {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM discovery_observations WHERE session_id=$1 AND device_key=$2)`, id, o.DeviceKey).Scan(&exists); err != nil {
				return err
			}
			if !exists && have >= MaxObservations {
				dropped++
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO discovery_observations(session_id,business_id,device_key,macs,identity,board,platform,version,interface_name,claimed_ip,source_ip,ip_mismatch,ip_changed,heard_count)
				VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,'')::inet,NULLIF($11,'')::inet,$12,$13,$14)
				ON CONFLICT(session_id,device_key) DO UPDATE SET macs=(SELECT ARRAY(SELECT DISTINCT m FROM unnest(discovery_observations.macs || EXCLUDED.macs) m ORDER BY m LIMIT 8)),
				identity=EXCLUDED.identity,board=EXCLUDED.board,platform=EXCLUDED.platform,version=EXCLUDED.version,interface_name=EXCLUDED.interface_name,
				claimed_ip=EXCLUDED.claimed_ip,source_ip=EXCLUDED.source_ip,ip_mismatch=EXCLUDED.ip_mismatch,
				ip_changed=discovery_observations.ip_changed OR EXCLUDED.ip_changed,heard_count=GREATEST(discovery_observations.heard_count,EXCLUDED.heard_count),last_received_at=now()`,
				id, bid, o.DeviceKey, pq.Array(o.MACs), o.Identity, o.Board, o.Platform, o.Version, o.Interface, o.ClaimedIP, o.SourceIP, o.IPMismatch, o.IPChanged, o.HeardCount); err != nil {
				return err
			}
			if !exists {
				have++
			}
			accepted++
		}
		_, err := tx.ExecContext(ctx, `UPDATE discovery_sessions SET last_upload_at=now() WHERE session_id=$1 AND business_id=$2`, id, bid)
		return err
	})
	return accepted, dropped, err
}

func isHex(s string) bool {
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// Selection is what the owner chose. LocalAddress is where the router's own page may be, never verified.
type Selection struct {
	DeviceKey     string          `json:"device_key"`
	SuggestedName string          `json:"suggested_name"`
	Model         string          `json:"device_model,omitempty"`
	LocalAddress  string          `json:"local_address,omitempty"`
	SiteID        string          `json:"site_id,omitempty"`
	Existing      *ExistingRouter `json:"existing_router,omitempty"`
}

var ErrConflict = errors.New("device already registered")

// Select records the owner's choice of one candidate. Only one candidate is selected at a time. Nothing is registered here:
// the console then uses the existing add-router and one-paste flow.
func (s *Store) Select(ctx context.Context, bid, id, deviceKey string) (Selection, error) {
	out := Selection{DeviceKey: deviceKey}
	err := s.tenant(ctx, bid, func(tx *sql.Tx) error {
		var site sql.NullString
		var live bool
		if err := tx.QueryRowContext(ctx, `SELECT site_id,(state IN ('open','paired') AND expires_at>now()) FROM discovery_sessions WHERE session_id=$1 AND business_id=$2 FOR UPDATE`, id, bid).Scan(&site, &live); err != nil || !live {
			return ErrUnavailable
		}
		var identity, board, ip, src string
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(identity,''),COALESCE(board,''),COALESCE(host(claimed_ip),''),COALESCE(host(source_ip),'') FROM discovery_observations WHERE session_id=$1 AND business_id=$2 AND device_key=$3`, id, bid, deviceKey).Scan(&identity, &board, &ip, &src); err != nil {
			return ErrUnavailable
		}
		if _, err := tx.ExecContext(ctx, `UPDATE discovery_observations SET selected_at=CASE WHEN device_key=$3 THEN now() ELSE NULL END WHERE session_id=$1 AND business_id=$2`, id, bid, deviceKey); err != nil {
			return err
		}
		out.SiteID, out.Model = site.String, board
		out.SuggestedName = firstNonEmpty(identity, board, "MikroTik router")
		// Prefer the address the announcement actually came from over the one it claims.
		out.LocalAddress = firstNonEmpty(src, ip)
		var did, dname string
		err := tx.QueryRowContext(ctx, `SELECT l.device_id::text,d.hardware_name FROM discovery_links l JOIN devices d ON d.device_id=l.device_id AND d.business_id=l.business_id WHERE l.business_id=$1 AND l.device_key=$2`, bid, deviceKey).Scan(&did, &dname)
		if err == nil {
			out.Existing = &ExistingRouter{DeviceID: did, Name: dname}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	return out, err
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// Link remembers that the selected candidate became this registered router. It requires the owner's earlier selection, so a
// discovery result can never register or claim anything by itself.
func (s *Store) Link(ctx context.Context, bid, id, deviceKey, deviceID string) error {
	return s.tenant(ctx, bid, func(tx *sql.Tx) error {
		var selected bool
		if err := tx.QueryRowContext(ctx, `SELECT selected_at IS NOT NULL FROM discovery_observations WHERE session_id=$1 AND business_id=$2 AND device_key=$3`, id, bid, deviceKey).Scan(&selected); err != nil || !selected {
			return ErrUnavailable
		}
		var ok bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM devices WHERE device_id=$1 AND business_id=$2)`, deviceID, bid).Scan(&ok); err != nil || !ok {
			return ErrUnavailable
		}
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT device_id::text FROM discovery_links WHERE business_id=$1 AND device_key=$2`, bid, deviceKey).Scan(&existing)
		switch {
		case err == nil && existing == deviceID:
			return nil
		case err == nil:
			return ErrConflict
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO discovery_links(business_id,device_key,device_id) VALUES($1,$2,$3)`, bid, deviceKey, deviceID)
		return err
	})
}
