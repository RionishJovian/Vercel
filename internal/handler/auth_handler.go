package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	userRepo    *database.UserRepository
	authService *auth.Service
	db          *database.DB
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(userRepo *database.UserRepository, authService *auth.Service, db *database.DB) *AuthHandler {
	return &AuthHandler{
		userRepo:    userRepo,
		authService: authService,
		db:          db,
	}
}

// Register creates a new tenant (business) with an owner user
func (h *AuthHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Hash password first
	hash, err := h.authService.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to secure password"})
		return
	}

	// Use a transaction: create business + user atomically
	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer tx.Rollback()

	// Check if email already exists
	existing, err := h.userRepo.FindByEmailTXN(tx, req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
		return
	}

	// Create business
	var business model.Business
	err = tx.QueryRow(`
		INSERT INTO businesses (company_name, owner_email, access_tier)
		VALUES ($1, $2, 'SMALL_WISP')
		RETURNING business_id, company_name, owner_email, access_tier,
		          is_active, device_quota_limit, created_at, updated_at`,
		req.CompanyName, req.Email,
	).Scan(
		&business.BusinessID, &business.CompanyName, &business.OwnerEmail,
		&business.AccessTier, &business.IsActive, &business.DeviceQuotaLimit,
		&business.CreatedAt, &business.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create business"})
		return
	}

	// Create NATS subject prefix
	_, err = tx.Exec(`
		INSERT INTO nats_subjects (business_id, subject_prefix)
		VALUES ($1, $2)`,
		business.BusinessID, "tenant."+business.BusinessID[:8]+".",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to configure tenant"})
		return
	}

	// Create portal config
	_, err = tx.Exec(`
		INSERT INTO portal_configs (business_id, business_name)
		VALUES ($1, $2)`,
		business.BusinessID, req.CompanyName+" Wi-Fi",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to configure portal"})
		return
	}

	// Create metered quota row
	_, err = tx.Exec(`
		INSERT INTO business_metered_quotas (business_id, ai_token_balance)
		VALUES ($1, 50000)`,
		business.BusinessID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize quota"})
		return
	}

	// Create the owner user
	// EXCLUSION (system-level): register creates the business ITSELF in this same
	// tx — there is no tenant context until commit, so RLS here is self-defeating.
	user, err := h.userRepo.CreateTXN(tx, business.BusinessID, req.FullName, req.Email, hash, string(model.RoleOwner))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to finalize registration"})
		return
	}

	// Generate tokens
	token, err := h.authService.GenerateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	refreshToken, err := h.authService.GenerateRefreshToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate refresh token"})
		return
	}

	c.JSON(http.StatusCreated, model.AuthResponse{
		Token:        token,
		RefreshToken: refreshToken,
		User:         *user,
		Business:     business,
	})
}

// Login authenticates a user and returns tokens
func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// EXCLUSION (system-level): login runs BEFORE any tenant context exists;
	// scoping this lookup to a business would lock every user out.
	user, err := h.userRepo.FindByEmail(req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if err := h.authService.VerifyPassword(req.Password, user.PasswordHash); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	// Update last login (EXCLUSION: pre-tenant bootstrap write, owner-scoped by design).
	_ = h.userRepo.UpdateLastLogin(user.UserID)

	// Fetch business
	var business model.Business
	err = h.db.QueryRow(`
		SELECT business_id, company_name, owner_email, access_tier,
		       is_active, device_quota_limit, created_at, updated_at
		FROM businesses
		WHERE business_id = $1`, user.BusinessID,
	).Scan(
		&business.BusinessID, &business.CompanyName, &business.OwnerEmail,
		&business.AccessTier, &business.IsActive, &business.DeviceQuotaLimit,
		&business.CreatedAt, &business.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load business"})
		return
	}

	// A suspended tenant must not be able to start a new session. Without this the
	// platform's suspend action wrote is_active=false and nothing read it, so the
	// tenant kept working — a control that reported success and changed nothing.
	// 403 (not 401) is deliberate: the credentials were correct, the account is
	// simply switched off, and saying so avoids a user retyping a working password.
	if !business.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "this business account is suspended"})
		return
	}

	token, err := h.authService.GenerateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	refreshToken, err := h.authService.GenerateRefreshToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate refresh token"})
		return
	}

	c.JSON(http.StatusOK, model.AuthResponse{
		Token:        token,
		RefreshToken: refreshToken,
		User:         *user,
		Business:     business,
	})
}

// RefreshToken issues a new access token from a valid refresh token
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token required"})
		return
	}

	userID, businessID, err := h.authService.ValidateRefreshToken(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}

	// EXCLUSION (system-level): refresh runs BEFORE a fresh tenant context exists;
	// the user.BusinessID != businessID cross-check below is the guard.
	user, err := h.userRepo.FindByID(userID)
	if err != nil || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}

	if user.BusinessID != businessID {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token mismatch"})
		return
	}

	token, err := h.authService.GenerateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	refreshToken, err := h.authService.GenerateRefreshToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate refresh token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":         token,
		"refresh_token": refreshToken,
	})
}

// Me returns the current user's profile.
//
// The session user's OWN profile read is genuinely tenant-scoped: it is
// post-auth and businessID comes from the JWT. Login/refresh/register reads stay
// SYSTEM-LEVEL by design (see the exclusion notes on those call sites).
func (h *AuthHandler) Me(c *gin.Context) {
	userID := auth.GetUserID(c)
	businessID := auth.GetBusinessID(c)

	tx, err := beginTenantTx(c, h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer tx.Rollback()

	var user *model.User
	err = database.WithTenant(tx, businessID, func() error {
		u, e := h.userRepo.FindByIDTXN(tx, userID)
		if e != nil {
			return e
		}
		user = u
		return nil
	})
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	var business model.Business
	err = h.db.QueryRow(`
		SELECT business_id, company_name, owner_email, access_tier,
		       is_active, device_quota_limit, created_at, updated_at
		FROM businesses WHERE business_id = $1`, businessID,
	).Scan(
		&business.BusinessID, &business.CompanyName, &business.OwnerEmail,
		&business.AccessTier, &business.IsActive, &business.DeviceQuotaLimit,
		&business.CreatedAt, &business.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load business"})
		return
	}

	c.JSON(http.StatusOK, model.AuthResponse{
		User:     *user,
		Business: business,
	})
}
