package telegram

import (
	"strconv"
	"strings"
)

// OrderReference is a display label only. Decisions still use the full ID in
// callback data; this shortened label must never be used for authorization.
func OrderReference(id string) string {
	id = strings.ToUpper(strings.ReplaceAll(id, "-", ""))
	if len(id) > 12 {
		id = id[:12]
	}
	return "#" + id
}

func FormatMMK(amount int) string {
	n := strconv.Itoa(amount)
	sign := ""
	if strings.HasPrefix(n, "-") {
		sign, n = "-", n[1:]
	}
	for i := len(n) - 3; i > 0; i -= 3 {
		n = n[:i] + "," + n[i:]
	}
	return sign + n + " MMK"
}

// Keep user-entered names/references on one line so they cannot impersonate
// another field in an operator's approval message. Telegram uses plain text.
func messageLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func StoreApprovalMessage(id, business, product string, amount int, bankReference string) string {
	return "Mega Magic Store — payment approval needed\n\n" +
		"Business: " + messageLine(business) + "\n" +
		"Product: " + messageLine(product) + "\n" +
		"Amount: " + FormatMMK(amount) + "\n" +
		"Bank transfer reference: " + messageLine(bankReference) + "\n" +
		"Order: " + OrderReference(id) + "\n\n" +
		"Check that the payment has arrived before approving."
}
