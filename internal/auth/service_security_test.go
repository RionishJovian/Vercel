package auth

import (
	"github.com/golang-jwt/jwt/v5"
	"mega-magic/internal/model"
	"testing"
	"time"
)

func TestTokenBoundaries(t *testing.T) {
	s, _ := NewService("test-secret-with-enough-characters", time.Hour, 24*time.Hour, 4)
	u := &model.User{UserID: "user", BusinessID: "business", UserRole: model.RoleOwner}
	access, _ := s.GenerateToken(u)
	refresh, _ := s.GenerateRefreshToken(u)
	if _, err := s.ValidateToken(access); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ValidateToken(refresh); err == nil {
		t.Fatal("refresh accepted as access")
	}
	if _, _, err := s.ValidateRefreshToken(access); err == nil {
		t.Fatal("access accepted as refresh")
	}
	cases := []struct {
		name   string
		alg    jwt.SigningMethod
		mutate func(jwt.MapClaims)
	}{
		{"wrong algorithm", jwt.SigningMethodHS384, func(c jwt.MapClaims) {}},
		{"missing expiry", jwt.SigningMethodHS256, func(c jwt.MapClaims) { delete(c, "exp") }},
		{"expired", jwt.SigningMethodHS256, func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }},
		{"wrong issuer", jwt.SigningMethodHS256, func(c jwt.MapClaims) { c["iss"] = "other" }},
		{"wrong audience", jwt.SigningMethodHS256, func(c jwt.MapClaims) { c["aud"] = "other" }},
		{"missing role", jwt.SigningMethodHS256, func(c jwt.MapClaims) { delete(c, "role") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := jwt.MapClaims{"sub": "user", "business_id": "business", "role": "OWNER", "type": "access", "iss": "mega-magic", "aud": "mega-magic-api", "exp": time.Now().Add(time.Hour).Unix()}
			tc.mutate(c)
			v, _ := jwt.NewWithClaims(tc.alg, c).SignedString(s.secret)
			if _, err := s.ValidateToken(v); err == nil {
				t.Fatal("unsafe token accepted")
			}
		})
	}
}
