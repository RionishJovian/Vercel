package telegram

import (
	"strings"
	"testing"
)

func TestStoreApprovalMessageReadable(t *testing.T) {
	id := "1cf2c3ee-fe1f-4660-8366-f764d05da8b8"
	text := StoreApprovalMessage(id, "Mega Magic Development Inc", "Annual pass", 350000, "jj")
	for _, want := range []string{"Business: Mega Magic Development Inc", "Product: Annual pass", "Amount: 350,000 MMK", "Bank transfer reference: jj", "Order: #1CF2C3EEFE1F"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	if strings.Contains(text, id) {
		t.Fatal("internal UUID exposed in message")
	}
}

func TestFormatMMK(t *testing.T) {
	for amount, want := range map[int]string{0: "0 MMK", 50: "50 MMK", 1000: "1,000 MMK", 350000: "350,000 MMK", 123456789: "123,456,789 MMK", -1234: "-1,234 MMK"} {
		if got := FormatMMK(amount); got != want {
			t.Fatalf("FormatMMK(%d) = %q, want %q", amount, got, want)
		}
	}
}

func TestStoreMessageUserFieldsCannotInsertLines(t *testing.T) {
	text := StoreApprovalMessage("order", "Business\nAmount: 0", "Annual pass", 350000, "jj\r\nApproved")
	if strings.Contains(text, "\nAmount: 0") || strings.Contains(text, "\nApproved") {
		t.Fatal("user field injected a message line")
	}
}

func TestDecisionAckKeepsFullIDInternal(t *testing.T) {
	id := "1cf2c3ee-fe1f-4660-8366-f764d05da8b8"
	for _, approve := range []bool{true, false} {
		dec := &fakeDecider{resolved: map[string]string{id: "tenant"}}
		msg := &fakeMsg{}
		bridge := NewPurchaseBridge(nil, msg, "operator", dec)
		command := "reject "
		if approve {
			command = "approve "
		}
		bridge.handle(InboundMessage{Text: command + id})
		if len(dec.decisions) != 1 || !strings.Contains(dec.decisions[0], id) {
			t.Fatal("decision did not use full internal ID")
		}
		if len(msg.sent) != 1 || strings.Contains(msg.sent[0], id) || !strings.Contains(msg.sent[0], OrderReference(id)) {
			t.Fatalf("unexpected acknowledgement: %v", msg.sent)
		}
	}
}

func TestBridgeAuthorizationPreventsDecisions(t *testing.T) {
	for _, business := range []string{"tenant-A", "tenant-B"} {
		for _, sender := range []int64{123, 999} {
			dec := &fakeDecider{resolved: map[string]string{"order": business}}
			bridge := NewPurchaseBridge(nil, &fakeMsg{}, "operator", dec)
			bridge.SetAuthorization(func(m InboundMessage, resolved string) bool {
				return m.FromID == 123 && m.ChatID == 456 && (resolved == "" || resolved == "tenant-A")
			})
			bridge.handle(InboundMessage{Text: "approve order", FromID: sender, ChatID: 456})
			want := 0
			if business == "tenant-A" && sender == 123 {
				want = 1
			}
			if len(dec.decisions) != want {
				t.Fatalf("business %s sender %d: decisions %v", business, sender, dec.decisions)
			}
		}
	}
}
