package model

import "time"

// MembershipStatus describes a user's relationship to a business tenant.
// A membership is separate from the user's login identity so future support
// access and multi-business administration do not require duplicating users.
type MembershipStatus string

const (
	MembershipActive    MembershipStatus = "active"
	MembershipSuspended MembershipStatus = "suspended"
	MembershipRevoked   MembershipStatus = "revoked"
)

type BusinessMembership struct {
	MembershipID string           `json:"membership_id"`
	BusinessID   string           `json:"business_id"`
	UserID       string           `json:"user_id"`
	Role         UserRole         `json:"role"`
	Status       MembershipStatus `json:"status"`
	InvitedBy    *string          `json:"invited_by,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

type BusinessInvitation struct {
	InvitationID string     `json:"invitation_id"`
	BusinessID   string     `json:"business_id"`
	Email        string     `json:"email"`
	FullName     string     `json:"full_name"`
	Role         UserRole   `json:"role"`
	InvitedBy    string     `json:"invited_by"`
	ExpiresAt    time.Time  `json:"expires_at"`
	AcceptedAt   *time.Time `json:"accepted_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}
