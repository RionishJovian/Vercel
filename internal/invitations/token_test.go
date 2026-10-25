package invitations

import (
	"strings"
	"testing"
)

func TestNewTokenStoresOnlyDigest(t *testing.T) {
	raw, digest, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 40 || strings.ContainsAny(raw, "+/=") {
		t.Fatalf("token is not URL-safe/high entropy enough: %q", raw)
	}
	if len(digest) != 64 || digest != HashToken(raw) {
		t.Fatalf("digest mismatch or unexpected length: %q", digest)
	}
	if raw == digest {
		t.Fatal("raw invitation token must never equal its stored digest")
	}
}

func TestHashTokenIsDeterministic(t *testing.T) {
	if HashToken("example-token") != HashToken("example-token") {
		t.Fatal("same token must hash identically")
	}
	if HashToken("example-token") == HashToken("other-token") {
		t.Fatal("different tokens must not share a digest in this test")
	}
}
