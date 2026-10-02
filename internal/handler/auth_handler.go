package handler

import (
	"database/sql"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

const (
	maxAvatarBytes = 2 << 20
	maxCoverBytes  = 5 << 20
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	SubscriptionOnly           bool
	SubscriptionExistingPolicy string
	userRepo                   *database.UserRepository
	authService                *auth.Service
	db                         *database.DB
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

	// Trial RLS applies even to the table owner. Bind the newly created tenant
	// after system-level registration writes; failure rolls back the whole signup.
	if h.SubscriptionOnly {
		err := database.WithTenant(tx, business.BusinessID, func() error {
			_, e := tx.Exec(`INSERT INTO business_subscription_trials(business_id,trial_started_at,trial_expires_at) VALUES ($1,now(),now()+interval '336 hours')`, business.BusinessID)
			return e
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize trial"})
			return
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to finalize registration"})
		return
	}

	// Generate tokens
	token, refreshToken, err := h.authService.StartSession(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unable to establish session"})
		return
	}
	h.setSessionCookies(c, token, refreshToken)
	// Browser responses expose no reusable tokens; API clients without Origin
	// retain explicit bearer token responses.
	if c.GetHeader("Origin") != "" {
		token = ""
		refreshToken = ""
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

	token, refreshToken, err := h.authService.StartSession(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unable to establish session"})
		return
	}
	h.setSessionCookies(c, token, refreshToken)
	// Browser responses expose no reusable tokens; API clients without Origin
	// retain explicit bearer token responses.
	if c.GetHeader("Origin") != "" {
		token = ""
		refreshToken = ""
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
	req.RefreshToken, _ = c.Cookie("mm_refresh")
	if req.RefreshToken == "" {
		_ = c.ShouldBindJSON(&req)
	}
	if req.RefreshToken == "" {
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

	token, refreshToken, err := h.authService.RotateSession(req.RefreshToken, user)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unable to establish session"})
		return
	}
	h.setSessionCookies(c, token, refreshToken)
	// Browser responses expose no reusable tokens; API clients without Origin
	// retain explicit bearer token responses.
	if c.GetHeader("Origin") != "" {
		token = ""
		refreshToken = ""
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

// Profile returns the signed-in user's own presentation and preferences.
func (h *AuthHandler) Profile(c *gin.Context) {
	userID, businessID := auth.GetUserID(c), auth.GetBusinessID(c)
	tx, err := beginTenantTx(c, h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer tx.Rollback()
	var user *model.User
	var profile *model.UserProfileSettings
	err = database.WithTenant(tx, businessID, func() error {
		var e error
		user, e = h.userRepo.FindByIDTXN(tx, userID)
		if e != nil || user == nil {
			return e
		}
		profile, e = h.userRepo.GetUserProfileSettingsTXN(tx, businessID, userID)
		return e
	})
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user_id": user.UserID, "full_name": user.FullName, "email": user.Email,
		"user_role": user.UserRole, "language": profile.Language,
		"has_avatar": profile.HasAvatar, "has_cover": profile.HasCover,
		"last_login_at": user.LastLoginAt, "created_at": user.CreatedAt,
		"updated_at": profile.UpdatedAt,
	})
}

func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	var req struct {
		FullName string `json:"full_name" binding:"required,min=1,max=255"`
		Language string `json:"language" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and language are required"})
		return
	}
	req.FullName = strings.TrimSpace(req.FullName)
	if req.FullName == "" || (req.Language != "en" && req.Language != "my" && req.Language != "zh") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid profile details"})
		return
	}
	tx, err := beginTenantTx(c, h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer tx.Rollback()
	err = database.WithTenant(tx, auth.GetBusinessID(c), func() error {
		return h.userRepo.UpdateOwnProfileTXN(tx, auth.GetBusinessID(c), auth.GetUserID(c), req.FullName, req.Language)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update profile"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "saved"})
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required,min=8,max=128"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "current password and a new password of at least 8 characters are required"})
		return
	}
	userID, businessID := auth.GetUserID(c), auth.GetBusinessID(c)
	tx, err := beginTenantTx(c, h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer tx.Rollback()
	err = database.WithTenant(tx, businessID, func() error {
		user, e := h.userRepo.FindByIDTXN(tx, userID)
		if e != nil || user == nil {
			return sql.ErrNoRows
		}
		if e = h.authService.VerifyPassword(req.CurrentPassword, user.PasswordHash); e != nil {
			return e
		}
		hash, e := h.authService.HashPassword(req.NewPassword)
		if e != nil {
			return e
		}
		return h.userRepo.UpdateOwnPasswordTXN(tx, businessID, userID, hash)
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "current password is incorrect or the new password is invalid"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "password changed"})
}

func profileAssetLimit(kind string) int64 {
	if kind == "avatar" {
		return maxAvatarBytes
	}
	if kind == "cover" {
		return maxCoverBytes
	}
	return 0
}

func (h *AuthHandler) PutProfileAsset(c *gin.Context) {
	kind := c.Param("kind")
	limit := profileAssetLimit(kind)
	if limit == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "unsupported profile asset"})
		return
	}
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image file required"})
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "profile image is too large"})
		return
	}
	mime := http.DetectContentType(data)
	if mime != "image/jpeg" && mime != "image/png" && mime != "image/webp" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "use a JPEG, PNG, or WebP image"})
		return
	}
	tx, err := beginTenantTx(c, h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer tx.Rollback()
	err = database.WithTenant(tx, auth.GetBusinessID(c), func() error {
		return h.userRepo.PutOwnProfileAssetTXN(tx, auth.GetBusinessID(c), auth.GetUserID(c), kind, mime, data)
	})
	if err != nil || tx.Commit() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save profile image"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "saved", "kind": kind})
}

func (h *AuthHandler) ProfileAsset(c *gin.Context) {
	kind := c.Param("kind")
	if profileAssetLimit(kind) == 0 {
		c.Status(http.StatusNotFound)
		return
	}
	tx, err := beginTenantTx(c, h.db)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()
	var data []byte
	var mime string
	err = database.WithTenant(tx, auth.GetBusinessID(c), func() error {
		var e error
		data, mime, e = h.userRepo.GetOwnProfileAssetTXN(tx, auth.GetBusinessID(c), auth.GetUserID(c), kind)
		return e
	})
	if err != nil || len(data) == 0 {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Data(http.StatusOK, mime, data)
}

func (h *AuthHandler) DeleteProfileAsset(c *gin.Context) {
	kind := c.Param("kind")
	if profileAssetLimit(kind) == 0 {
		c.Status(http.StatusNotFound)
		return
	}
	tx, err := beginTenantTx(c, h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer tx.Rollback()
	err = database.WithTenant(tx, auth.GetBusinessID(c), func() error {
		return h.userRepo.DeleteOwnProfileAssetTXN(tx, auth.GetBusinessID(c), auth.GetUserID(c), kind)
	})
	if err != nil || tx.Commit() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not remove profile image"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

func (h *AuthHandler) setSessionCookies(c *gin.Context, access, refresh string) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("mm_access", access, int(h.authService.AccessLifetime().Seconds()), "/api/v1", "", true, true)
	c.SetCookie("mm_refresh", refresh, int(h.authService.RefreshLifetime().Seconds()), "/api/v1/auth", "", true, true)
	c.Header("Cache-Control", "no-store")
}
func (h *AuthHandler) Logout(c *gin.Context) {
	value, _ := c.Cookie("mm_refresh")
	if value == "" {
		value, _ = c.Cookie("mm_access")
	}
	if value == "" {
		header := c.GetHeader("Authorization")
		parts := strings.SplitN(header, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			value = parts[1]
		}
	}
	if value == "" {
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		_ = c.ShouldBindJSON(&body)
		value = body.RefreshToken
	}
	if value != "" {
		if err := h.authService.RevokeSession(value); err != nil {
			c.JSON(503, gin.H{"error": "unable to revoke session"})
			return
		}
	}
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("mm_access", "", -1, "/api/v1", "", true, true)
	c.SetCookie("mm_refresh", "", -1, "/api/v1/auth", "", true, true)
	c.Status(http.StatusNoContent)
}
