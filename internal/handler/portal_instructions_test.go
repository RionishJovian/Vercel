package handler

import (
	"strings"
	"testing"
)

func TestValidPaymentInstructions(t *testing.T) {
	if !validPaymentInstructions("") || !validPaymentInstructions("Send to KBZ Pay and put your phone number in the note.") {
		t.Fatal("empty and ordinary instructions must be accepted")
	}
	if !validPaymentInstructions(strings.Repeat("က", 600)) {
		t.Fatal("600 Burmese characters must be accepted: the limit counts characters, not bytes")
	}
	if validPaymentInstructions(strings.Repeat("a", 601)) {
		t.Fatal("601 characters must be rejected")
	}
	if validPaymentInstructions("a\x00b") {
		t.Fatal("NUL must be rejected")
	}
}
