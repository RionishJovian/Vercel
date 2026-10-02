package bridge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mega-magic/internal/media/jellyfin"
	"regexp"
	"time"
)

var ErrUnavailable = errors.New("media connection unavailable")
var UUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var secretPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func Secret() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func Hash(secret string) (string, error) {
	if !secretPattern.MatchString(secret) {
		return "", ErrUnavailable
	}
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:]), nil
}

type Connection struct {
	ServerName   string     `json:"server_name"`
	LastSeen     *time.Time `json:"last_seen_at"`
	TokenExpires *time.Time `json:"expires_at"`
	Revoked      *time.Time `json:"revoked_at"`
}
type Job struct {
	ID        string          `json:"id"`
	Operation string          `json:"operation"`
	Payload   json.RawMessage `json:"payload"`
	State     string          `json:"state"`
	Result    json.RawMessage `json:"result,omitempty"`
	ErrorCode string          `json:"error_code,omitempty"`
	Expires   time.Time       `json:"expires_at"`
}
type Completion struct {
	// Fixed result types prevent arbitrary upstream bodies/credentials being relayed.
	Status    *jellyfin.ServerInfo `json:"status,omitempty"`
	Dashboard *jellyfin.Dashboard  `json:"dashboard,omitempty"`
	Libraries []jellyfin.Library   `json:"libraries,omitempty"`
	Accepted  bool                 `json:"accepted,omitempty"`
	ErrorCode string               `json:"error_code,omitempty"`
}
type AddPayload struct {
	Library jellyfin.NewLibrary `json:"library"`
	Confirm bool                `json:"confirm"`
}

func ValidateOperation(op string, payload json.RawMessage) error {
	switch op {
	case "status", "libraries", "dashboard":
	case "scan":
		var p struct {
			Confirm bool `json:"confirm"`
		}
		if json.Unmarshal(payload, &p) != nil || !p.Confirm {
			return errors.New("confirm the scan request")
		}
	case "add_library":
		var p AddPayload
		if json.Unmarshal(payload, &p) != nil || !p.Confirm || p.Library.Name == "" || len(p.Library.Paths) == 0 {
			return errors.New("confirm library name, type and server folders")
		}
		if err := jellyfin.ValidateLibrary(p.Library); err != nil {
			return err
		}
	default:
		return errors.New("unsupported media operation")
	}
	if len(payload) > 32768 {
		return errors.New("media request too large")
	}
	return nil
}

// Store implementations must serialize connection revocation/pairing/claims.
type Store interface {
	Pair(context.Context, string, string) error
	Redeem(context.Context, string, string, string) error
	Connection(context.Context, string) (Connection, error)
	Revoke(context.Context, string) error
	Enqueue(context.Context, string, string, Job) error
	GetJob(context.Context, string, string) (Job, error)
	Poll(context.Context, string) (*Job, error)
	Complete(context.Context, string, string, Completion) error
}
