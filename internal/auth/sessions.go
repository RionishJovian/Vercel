package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"time"
)

func digest(value string) string               { h := sha256.Sum256([]byte(value)); return hex.EncodeToString(h[:]) }
func (s *Service) UseSessions(db *database.DB) { s.db = db }
func (s *Service) sessionID(value string) (string, error) {
	t, err := jwt.Parse(value, func(t *jwt.Token) (interface{}, error) { return s.secret, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuer("mega-magic"), jwt.WithAudience("mega-magic-api"))
	if err != nil {
		return "", err
	}
	id, _ := t.Claims.(jwt.MapClaims)["sid"].(string)
	if id == "" {
		return "", errors.New("missing session")
	}
	return id, nil
}
func (s *Service) pair(user *model.User, id string) (string, string, error) {
	access, err := s.GenerateToken(user)
	if err != nil {
		return "", "", err
	}
	refresh, err := s.GenerateRefreshToken(user)
	if err != nil {
		return "", "", err
	}
	attach := func(value string) (string, error) {
		t, err := jwt.Parse(value, func(t *jwt.Token) (interface{}, error) { return s.secret, nil }, jwt.WithValidMethods([]string{"HS256"}))
		if err != nil {
			return "", err
		}
		t.Claims.(jwt.MapClaims)["sid"] = id
		return t.SignedString(s.secret)
	}
	access, err = attach(access)
	if err != nil {
		return "", "", err
	}
	refresh, err = attach(refresh)
	return access, refresh, err
}
func (s *Service) StartSession(user *model.User) (string, string, error) {
	id := randomID()
	a, r, err := s.pair(user, id)
	if err != nil {
		return "", "", err
	}
	// Lock current account state until the session is inserted. A concurrent
	// password/role/suspension change must either precede this check or revoke
	// the inserted session through the migration's triggers.
	tx, err := s.db.Begin()
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	var currentID string
	err = tx.QueryRow(`SELECT u.user_id FROM public.users u JOIN public.businesses b ON b.business_id=u.business_id WHERE u.user_id=$1 AND u.business_id=$2 AND u.password_hash=$3 AND u.user_role=$4 AND u.is_active AND b.is_active FOR SHARE OF u,b`, user.UserID, user.BusinessID, user.PasswordHash, user.UserRole).Scan(&currentID)
	if err != nil {
		return "", "", err
	}
	_, err = tx.Exec(`INSERT INTO auth_sessions(session_id,user_id,business_id,password_digest,user_role,refresh_digest,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, user.UserID, user.BusinessID, digest(user.PasswordHash), user.UserRole, digest(r), time.Now().Add(s.refreshExpiry))
	if err != nil {
		return "", "", err
	}
	err = tx.Commit()
	return a, r, err
}
func (s *Service) validateSession(value, userID, businessID string) error {
	id, err := s.sessionID(value)
	if err != nil {
		return err
	}
	var valid bool
	err = s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM auth_sessions s JOIN users u ON u.user_id=s.user_id JOIN businesses b ON b.business_id=s.business_id WHERE s.session_id=$1 AND s.user_id=$2 AND s.business_id=$3 AND NOT s.revoked AND s.expires_at>now() AND u.is_active AND b.is_active AND s.user_role=u.user_role AND s.password_digest=encode(digest(u.password_hash,'sha256'),'hex'))`, id, userID, businessID).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("session revoked")
	}
	return nil
}
func (s *Service) RotateSession(value string, user *model.User) (string, string, error) {
	userID, businessID, err := s.ValidateRefreshToken(value)
	if err != nil || userID != user.UserID || businessID != user.BusinessID {
		return "", "", errors.New("invalid refresh identity")
	}
	if err := s.validateSession(value, user.UserID, user.BusinessID); err != nil {
		return "", "", err
	}
	id, err := s.sessionID(value)
	if err != nil {
		return "", "", err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	var current string
	var revoked bool
	err = tx.QueryRow(`SELECT refresh_digest,revoked FROM auth_sessions WHERE session_id=$1 FOR UPDATE`, id).Scan(&current, &revoked)
	if err != nil {
		return "", "", err
	}
	if revoked || current != digest(value) {
		_, err = tx.Exec(`UPDATE auth_sessions SET revoked=true WHERE session_id=$1`, id)
		if err != nil {
			return "", "", err
		}
		if err = tx.Commit(); err != nil {
			return "", "", err
		}
		return "", "", errors.New("refresh token reused")
	}
	a, r, err := s.pair(user, id)
	if err != nil {
		return "", "", err
	}
	_, err = tx.Exec(`UPDATE auth_sessions SET refresh_digest=$2,expires_at=$3 WHERE session_id=$1`, id, digest(r), time.Now().Add(s.refreshExpiry))
	if err != nil {
		return "", "", err
	}
	err = tx.Commit()
	return a, r, err
}
func (s *Service) RevokeSession(value string) error {
	t, err := jwt.Parse(value, func(t *jwt.Token) (interface{}, error) { return s.secret, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithoutClaimsValidation())
	if err != nil {
		return err
	}
	claims := t.Claims.(jwt.MapClaims)
	if claims["iss"] != "mega-magic" || claims["aud"] != "mega-magic-api" {
		return errors.New("invalid session token")
	}
	id, _ := claims["sid"].(string)
	if id == "" {
		return errors.New("missing session")
	}
	_, err = s.db.Exec(`UPDATE auth_sessions SET revoked=true WHERE session_id=$1`, id)
	return err
}
