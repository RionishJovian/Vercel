package model

import "time"

type UserRole string

const (
	RoleOwner     UserRole = "OWNER"
	RoleAdmin     UserRole = "ADMIN"
	RoleOperator  UserRole = "OPERATOR"
	RoleTechnician UserRole = "TECHNICIAN"
	RoleViewer    UserRole = "VIEWER"
	RoleSupport   UserRole = "SUPPORT"
)

type User struct {
	UserID       string    `json:"user_id" db:"user_id"`
	BusinessID   string    `json:"business_id" db:"business_id"`
	FullName     string    `json:"full_name" db:"full_name"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"`
	UserRole     UserRole  `json:"user_role" db:"user_role"`
	IsActive     bool      `json:"is_active" db:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
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
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type RegisterRequest struct {
	CompanyName string `json:"company_name" binding:"required,min=1,max=255"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=8"`
	FullName    string `json:"full_name" binding:"required,min=1,max=255"`
}

type AuthResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	User         User   `json:"user"`
	Business     Business `json:"business"`
}

type TokenClaims struct {
	UserID     string `json:"user_id"`
	BusinessID string `json:"business_id"`
	Email      string `json:"email"`
	UserRole   string `json:"user_role"`
}