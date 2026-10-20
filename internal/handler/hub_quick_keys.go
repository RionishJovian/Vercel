package handler

import (
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
