package server

import (
	"mega-magic/internal/model"
	"mega-magic/internal/telegram"
	"testing"
)

func TestGuestApprovalAuthorization(t *testing.T) {
	started := model.BusinessTelegramBot{BusinessID: "business-A", BotToken: "token-A", TargetChatID: -100123, ApproverUserID: 123456, Enabled: true}
	m := telegram.InboundMessage{ChatID: started.TargetChatID, FromID: started.ApproverUserID}
	if !guestApprovalAuthorized(started, &started, m, "business-A") {
		t.Fatal("configured approver denied")
	}
	for _, tc := range []struct {
		name     string
		change   func(*model.BusinessTelegramBot, *telegram.InboundMessage)
		business string
	}{
		{"wrong person", func(_ *model.BusinessTelegramBot, m *telegram.InboundMessage) { m.FromID++ }, "business-A"},
		{"missing sender", func(_ *model.BusinessTelegramBot, m *telegram.InboundMessage) { m.FromID = 0 }, "business-A"},
		{"wrong chat", func(_ *model.BusinessTelegramBot, m *telegram.InboundMessage) { m.ChatID++ }, "business-A"},
		{"other tenant order", func(_ *model.BusinessTelegramBot, _ *telegram.InboundMessage) {}, "business-B"},
		{"disabled", func(b *model.BusinessTelegramBot, _ *telegram.InboundMessage) { b.Enabled = false }, "business-A"},
		{"token replaced", func(b *model.BusinessTelegramBot, _ *telegram.InboundMessage) { b.BotToken = "new-token" }, "business-A"},
		{"unconfigured approver", func(b *model.BusinessTelegramBot, _ *telegram.InboundMessage) { b.ApproverUserID = 0 }, "business-A"},
		{"approver changed", func(b *model.BusinessTelegramBot, _ *telegram.InboundMessage) { b.ApproverUserID++ }, "business-A"},
		{"chat changed", func(b *model.BusinessTelegramBot, _ *telegram.InboundMessage) { b.TargetChatID++ }, "business-A"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current, message := started, m
			tc.change(&current, &message)
			if guestApprovalAuthorized(started, &current, message, tc.business) {
				t.Fatal("unauthorized payment allowed")
			}
		})
	}
	if guestApprovalAuthorized(started, nil, m, "business-A") {
		t.Fatal("unbound bot allowed")
	}
}
