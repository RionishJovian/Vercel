package handler

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"mega-magic/internal/auth"
	"mega-magic/internal/model"
)

// Sign-up, change-password and the platform paths must all use one minimum length.
func TestRegisterPasswordRuleMatchesSharedMinimum(t *testing.T) {
	f, _ := reflect.TypeOf(model.RegisterRequest{}).FieldByName("Password")
	for _, part := range strings.Split(f.Tag.Get("binding"), ",") {
		if v, ok := strings.CutPrefix(part, "min="); ok {
			if n, _ := strconv.Atoi(v); n != auth.MinPasswordLength {
				t.Fatalf("register requires %d characters, shared minimum is %d", n, auth.MinPasswordLength)
			}
			return
		}
	}
	t.Fatal("register password has no min= rule")
}
