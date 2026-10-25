package model

import "time"

type UserRole string

const (
	RoleOwner     UserRole = "OWNER"
	RoleAdmin     UserRole = "ADMIN"
	RoleViewer    UserRole = "VIEWER"
	RoleDeveloper UserRole = "DEVELOPER"
)

// roleRank orders roles by capability. DEVELOPER is system-level (highest).
var roleRank = map[UserRole]int{
	RoleViewer:    10,
	RoleAdmin:     50,
	RoleOwner:     60,
	RoleDeveloper: 100,
}

// RoleRank returns a role's capability rank (0 for unknown roles).
func RoleRank(r UserRole) int { return roleRank[r] }

// HasAtLeastRole reports whether role meets or exceeds min in the hierarchy.
func HasAtLeastRole(role, min UserRole) bool {
	return RoleRank(role) != 0 && RoleRank(role) >= RoleRank(min)
}

// TenantRoles are the roles an OWNER may grant to staff (never DEVELOPER).
var TenantRoles = []UserRole{RoleOwner, RoleAdmin, RoleViewer}

// IsTenantRole reports whether r is a grantable tenant role.
func IsTenantRole(r UserRole) bool {
	for _, t := range TenantRoles {
		if t == r {
			return true
		}
	}
	return false
}

type User struct {
	UserID       string     `json:"user_id" db:"user_id"`
	BusinessID   string     `json:"business_id" db:"business_id"`
	FullName     string     `json:"full_name" db:"full_name"`
	Email        string     `json:"email" db:"email"`
	PasswordHash string     `json:"-" db:"password_hash"`
	UserRole     UserRole   `json:"user_role" db:"user_role"`
	IsActive     bool       `json:"is_active" db:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

// UserProfileSettings contains personal presentation and preference fields
// that do not belong in authorization claims or tenant team administration.
type UserProfileSettings struct {
	UserID     string    `json:"user_id"`
	BusinessID string    `json:"business_id"`
	Language   string    `json:"language"`
	HasAvatar  bool      `json:"has_avatar"`
	HasCover   bool      `json:"has_cover"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Business struct {
	BusinessID       string    `json:"business_id" db:"business_id"`
	CompanyName      string    `json:"company_name" db:"company_name"`
	OwnerEmail       string    `json:"owner_email" db:"owner_email"`
	AccessTier       string    `json:"access_tier" db:"access_tier"`
	IsActive         bool      `json:"is_active" db:"is_active"`
	DeviceQuotaLimit int       `json:"device_quota_limit" db:"device_quota_limit"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

type LoginRequest struct {
	// NOTE: deliberately NOT `binding:"email"`. Migrated Mikromagic logins are
	// TLD-less (e.g. `kyaw@mikromagic`) and must keep working verbatim — an
	// `email` binding rejects them with 400 before the password is ever checked.
	// Register stays strict (`email`) so NEW signups are real addresses.
	Email    string `json:"email" binding:"required,min=3"`
	Password string `json:"password" binding:"required,min=6"`
}

// PlatformIncidentCounts is the cross-tenant incident summary for the DEVELOPER
// platform window.
type PlatformIncidentCounts struct {
	Open     int `json:"open"`
	Resolved int `json:"resolved"`
}

type RegisterRequest struct {
	CompanyName string `json:"company_name" binding:"required,min=1,max=255"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=8"`
	FullName    string `json:"full_name" binding:"required,min=1,max=255"`
}

type AuthResponse struct {
	Token        string   `json:"token"`
	RefreshToken string   `json:"refresh_token,omitempty"`
	User         User     `json:"user"`
	Business     Business `json:"business"`
}

type TokenClaims struct {
	SessionID  string `json:"-"`
	UserID     string `json:"user_id"`
	BusinessID string `json:"business_id"`
	Email      string `json:"email"`
	UserRole   string `json:"user_role"`
}
