package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/invitations"
)

type InvitationHandler struct {
	db    *database.DB
	users *database.UserRepository
	auth  *auth.Service
}

func NewInvitationHandler(db *database.DB, users *database.UserRepository, authService *auth.Service) *InvitationHandler {
	return &InvitationHandler{db: db, users: users, auth: authService}
}

// Accept: POST /api/v1/auth/invitations/accept {token,password}
// The token is consumed atomically with user and membership creation.
func (h *InvitationHandler) Accept(c *gin.Context) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	body.Token = strings.TrimSpace(body.Token)
	if body.Token == "" || len(body.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token and an 8+ character password are required"})
		return
	}
	hash, err := h.auth.HashPassword(body.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password hash failed"})
		return
	}
	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer tx.Rollback()
	invite, err := h.users.FindPendingInvitationTXN(tx, invitations.HashToken(body.Token))
	if err != nil || invite == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invitation is invalid, expired, revoked, or already accepted"})
		return
	}
	if existing, e := h.users.FindByEmailTXN(tx, invite.Email); e != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	} else if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
		return
	}
	user, err := h.users.AcceptInvitationTXN(tx, invite.InvitationID, invite.BusinessID, invite.FullName, invite.Email, hash, string(invite.Role))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to accept invitation"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to finalize invitation"})
		return
	}
	token, refresh, err := h.auth.StartSession(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unable to establish session"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"token": token, "refresh_token": refresh, "user": user})
}
