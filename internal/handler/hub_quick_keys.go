package handler

import (
	"context"
	"errors"
	"time"

	"mega-magic/internal/hub"
)

const quickEnrollmentKeyLifetime = 30 * time.Minute
const maxPendingQuickEnrollments = 256

type quickEnrollmentKey struct {
	private, public string
	expires         time.Time
}

// Retain the router-local key only in bounded process memory, before the Hub
// call. A timeout may mean the Hub reserved that public key despite returning
// an error. Retrying must reuse the matching private half, never rotate a peer.
// Restart/expiry still requires explicit recovery of an existing enrollment.
func (h *VPSHandler) quickEnrollmentKeys(businessID, deviceID string, create bool) (quickEnrollmentKey, error) {
	if h.durableQuickKeys != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		private, public, e := h.durableQuickKeys(ctx, businessID, deviceID, create)
		return quickEnrollmentKey{private: private, public: public}, e
	}
	h.quickKeysMu.Lock()
	defer h.quickKeysMu.Unlock()
	now := time.Now()
	for id, key := range h.quickKeys {
		if !now.Before(key.expires) {
			delete(h.quickKeys, id)
		}
	}
	id := businessID + "/" + deviceID
	if key, ok := h.quickKeys[id]; ok {
		return key, nil
	}
	if !create {
		return quickEnrollmentKey{}, nil
	}
	if len(h.quickKeys) >= maxPendingQuickEnrollments {
		return quickEnrollmentKey{}, errors.New("too many pending router enrollments")
	}
	private, public, err := hub.GenerateRouterKeypair()
	if err != nil {
		return quickEnrollmentKey{}, err
	}
	key := quickEnrollmentKey{private: private, public: public, expires: now.Add(quickEnrollmentKeyLifetime)}
	if h.quickKeys == nil {
		h.quickKeys = make(map[string]quickEnrollmentKey)
	}
	h.quickKeys[id] = key
	return key, nil
}

// ConfigureDurableQuickKeys supplies encrypted restart-safe installation state.
func (h *VPSHandler) ConfigureDurableQuickKeys(f func(context.Context, string, string, bool) (string, string, error)) {
	h.durableQuickKeys = f
}
func (h *VPSHandler) ConfigureAutomaticPreflight(f func(context.Context, string, string) error) {
	h.automaticPreflight = f
}
