package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"mega-magic/internal/database"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"mega-magic/internal/model"
)

// Service handles JWT creation/validation and password hashing
type Service struct {
	db            *database.DB
	secret        []byte
	expiry        time.Duration
	refreshExpiry time.Duration
	bcryptCost    int
}

// NewService creates a new auth service
func NewService(secret string, expiry, refreshExpiry time.Duration, bcryptCost int) (*Service, error) {
	if expiry <= 0 || refreshExpiry <= 0 {
		return nil, errors.New("token lifetimes must be positive")
	}
	if len(secret) < 16 {
		return nil, errors.New("jwt secret must be at least 16 characters")
	}
	if bcryptCost < 4 || bcryptCost > 15 {
		bcryptCost = 12
	}
	return &Service{
		secret:        []byte(secret),
		expiry:        expiry,
		refreshExpiry: refreshExpiry,
		bcryptCost:    bcryptCost,
	}, nil
}

// HashPassword creates a bcrypt hash of the password
func (s *Service) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(bytes), nil
}

// VerifyPassword checks a password against its hash
func (s *Service) VerifyPassword(password, hash string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return errors.New("invalid credentials")
	}
	return nil
}

// GenerateToken creates a signed JWT for the user
func (s *Service) GenerateToken(user *model.User) (string, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss":         "mega-magic",
		"aud":         "mega-magic-api",
		"jti":         randomID(),
		"sub":         user.UserID,
		"business_id": user.BusinessID,
		"type":        "access",
		"email":       user.Email,
		"role":        string(user.UserRole),
		"iat":         now.Unix(),
		"exp":         now.Add(s.expiry).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}

	return signed, nil
}

// GenerateRefreshToken creates a longer-lived refresh token
func (s *Service) GenerateRefreshToken(user *model.User) (string, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss":         "mega-magic",
		"aud":         "mega-magic-api",
		"jti":         randomID(),
		"sub":         user.UserID,
		"business_id": user.BusinessID,
		"type":        "refresh",
		"iat":         now.Unix(),
		"exp":         now.Add(s.refreshExpiry).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("sign refresh token: %w", err)
	}

	return signed, nil
}

// ValidateToken parses and validates a JWT, returning the claims
func (s *Service) ValidateToken(tokenString string) (*model.TokenClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuer("mega-magic"), jwt.WithAudience("mega-magic-api"))
	if err != nil {
		return nil, fmt.Errorf("parse jwt: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	sub, _ := claims["sub"].(string)
	businessID, _ := claims["business_id"].(string)
	email, _ := claims["email"].(string)
	role, _ := claims["role"].(string)

	if claims["type"] != "access" || role == "" || sub == "" || businessID == "" {
		return nil, errors.New("token missing required claims")
	}

	if s.db != nil {
		if err := s.validateSession(tokenString, sub, businessID); err != nil {
			return nil, err
		}
	}
	sid, _ := claims["sid"].(string)
	return &model.TokenClaims{
		SessionID:  sid,
		UserID:     sub,
		BusinessID: businessID,
		Email:      email,
		UserRole:   role,
	}, nil
}

// ValidateRefreshToken validates a refresh token and returns the user id
func (s *Service) ValidateRefreshToken(tokenString string) (string, string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuer("mega-magic"), jwt.WithAudience("mega-magic-api"))
	if err != nil {
		return "", "", fmt.Errorf("parse refresh token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", "", errors.New("invalid refresh token")
	}

	tokenType, _ := claims["type"].(string)
	if tokenType != "refresh" {
		return "", "", errors.New("not a refresh token")
	}

	sub, _ := claims["sub"].(string)
	businessID, _ := claims["business_id"].(string)

	if sub == "" || businessID == "" {
		return "", "", errors.New("refresh token missing required claims")
	}
	return sub, businessID, nil
}
func randomID() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (s *Service) AccessLifetime() time.Duration  { return s.expiry }
func (s *Service) RefreshLifetime() time.Duration { return s.refreshExpiry }
