package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// TeamHandler is the OWNER-only team-management API (C3). Every route is
// tenant-scoped and gated by RequireMinRole(OWNER) at the router. A caller can
// only ever see and mutate users of THEIR OWN business (WithTenant), can never
// grant DEVELOPER, and can never remove/demote the last active OWNER.
type TeamHandler struct {
	db       *database.DB
	userRepo *database.UserRepository
	auth     *auth.Service
}

func NewTeamHandler(db *database.DB, userRepo *database.UserRepository, authService *auth.Service) *TeamHandler {
	return &TeamHandler{db: db, userRepo: userRepo, auth: authService}
}

// List: GET /api/v1/team
func (h *TeamHandler) List(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	var users []*model.User
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		us, e := h.userRepo.ListTeamByBusinessTXN(tx, businessID)
		if e != nil {
			return e
		}
		users = us
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	out := make([]gin.H, 0, len(users))
	for _, u := range users {
		out = append(out, gin.H{
			"user_id":       u.UserID,
			"full_name":     u.FullName,
			"email":         u.Email,
			"user_role":     u.UserRole,
			"is_active":     u.IsActive,
			"last_login_at": u.LastLoginAt,
			"created_at":    u.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"team": out, "count": len(out)})
}

// Create: POST /api/v1/team  {full_name,email,password,user_role}
func (h *TeamHandler) Create(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	var body struct {
		FullName string `json:"full_name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		UserRole string `json:"user_role"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	body.FullName = strings.TrimSpace(body.FullName)
	body.Email = strings.ToLower(strings.TrimSpace(body.Email))
	role := model.UserRole(strings.ToUpper(strings.TrimSpace(body.UserRole)))

	if body.FullName == "" || body.Email == "" || len(body.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "full_name, email and an 8+ char password are required"})
		return
	}
	// Never grant a system role to a tenant member.
	if !model.IsTenantRole(role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	hash, err := h.auth.HashPassword(body.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password hash failed"})
		return
	}

	var created *model.User
	err = runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		existing, e := h.userRepo.FindByEmailTXN(tx, body.Email)
		if e != nil {
			return e
		}
		if existing != nil {
			return errEmailTaken
		}
		u, e := h.userRepo.CreateTeamUserTXN(tx, businessID, body.FullName, body.Email, hash, string(role))
		if e != nil {
			return e
		}
		created = u
		return nil
	})
	if err != nil {
		if err == errEmailTaken {
			c.JSON(http.StatusConflict, gin.H{"error": "email already in use"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create team member"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"user_id":    created.UserID,
		"full_name":  created.FullName,
		"email":      created.Email,
		"user_role":  created.UserRole,
		"is_active":  created.IsActive,
		"created_at": created.CreatedAt,
	})
}

// Update: PUT /api/v1/team/:userId  {full_name,user_role,is_active}
func (h *TeamHandler) Update(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	userID := c.Param("userId")
	var body struct {
		FullName string `json:"full_name"`
		UserRole string `json:"user_role"`
		IsActive *bool  `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	var out *model.User
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		current, e := h.userRepo.GetTeamUserTXN(tx, businessID, userID)
		if e != nil {
			return e
		}
		if current == nil {
			return errTeamNotFound
		}

		fullName := strings.TrimSpace(body.FullName)
		if fullName == "" {
			fullName = current.FullName
		}
		role := current.UserRole
		if r := strings.ToUpper(strings.TrimSpace(body.UserRole)); r != "" {
			role = model.UserRole(r)
		}
		if !model.IsTenantRole(role) {
			return errBadRole
		}
		active := current.IsActive
		if body.IsActive != nil {
			active = *body.IsActive
		}

		// Guard: never demote/disable the last active OWNER.
		if current.UserRole == model.RoleOwner && current.IsActive && (role != model.RoleOwner || !active) {
			n, e := h.userRepo.CountActiveOwnersTXN(tx, businessID)
			if e != nil {
				return e
			}
			if n <= 1 {
				return errLastOwner
			}
		}

		u, e := h.userRepo.UpdateTeamUserTXN(tx, businessID, userID, fullName, string(role), active)
		if e != nil {
			return e
		}
		out = u
		return nil
	})
	switch err {
	case nil:
		c.JSON(http.StatusOK, gin.H{"user_id": out.UserID, "full_name": out.FullName, "user_role": out.UserRole, "is_active": out.IsActive})
	case errTeamNotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": "team member not found"})
	case errBadRole:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
	case errLastOwner:
		c.JSON(http.StatusConflict, gin.H{"error": "cannot demote or disable the last owner"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update team member"})
	}
}

// Delete: DELETE /api/v1/team/:userId  (soft-disable; last-owner guarded)
func (h *TeamHandler) Delete(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	userID := c.Param("userId")

	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		current, e := h.userRepo.GetTeamUserTXN(tx, businessID, userID)
		if e != nil {
			return e
		}
		if current == nil {
			return errTeamNotFound
		}
		if current.UserRole == model.RoleOwner && current.IsActive {
			n, e := h.userRepo.CountActiveOwnersTXN(tx, businessID)
			if e != nil {
				return e
			}
			if n <= 1 {
				return errLastOwner
			}
		}
		return h.userRepo.SetTeamActiveTXN(tx, businessID, userID, false)
	})
	switch err {
	case nil:
		c.JSON(http.StatusOK, gin.H{"disabled": userID})
	case errTeamNotFound:
		c.JSON(http.StatusNotFound, gin.H{"error": "team member not found"})
	case errLastOwner:
		c.JSON(http.StatusConflict, gin.H{"error": "cannot remove the last owner"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove team member"})
	}
}

var (
	errEmailTaken   = errTeam("email already in use")
	errTeamNotFound = errTeam("team member not found")
	errBadRole      = errTeam("invalid role")
	errLastOwner    = errTeam("last owner")
)

type errTeam string

func (e errTeam) Error() string { return string(e) }
