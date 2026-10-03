package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// ReceiptMessenger sends the receipt itself to the business owner's bot, rather
// than publishing a receipt URL or asking an owner to approve unseen evidence.
type ReceiptMessenger interface {
	Messenger
	SendReceipt(target, caption, reviewID string, photo []byte) error
}

func (b *Bot) SendReceipt(target, caption, reviewID string, photo []byte) error {
	failed := errors.New("receipt could not be delivered to the owner bot")
	if target == "" || len(caption) > 900 || len(reviewID) == 0 || len(reviewID) > 56 || strings.ContainsAny(reviewID, "\x00\r\n\"\\ ") || len(photo) == 0 || len(photo) > 5<<20 {
		return failed
	}
	var filename string
	switch http.DetectContentType(photo) {
	case "image/png":
		filename = "receipt.png"
	case "image/jpeg":
		filename = "receipt.jpg"
	case "image/webp":
		filename = "receipt.webp"
	default:
		return failed
	}
	markup, _ := json.Marshal(map[string]any{"inline_keyboard": [][]map[string]string{{{"text": "Approve", "callback_data": "approve " + reviewID}, {"text": "Reject", "callback_data": "reject " + reviewID}}}})
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for key, value := range map[string]string{"chat_id": target, "caption": caption, "reply_markup": string(markup)} {
		if form.WriteField(key, value) != nil {
			return failed
		}
	}
	part, e := form.CreateFormFile("document", filename)
	if e != nil {
		return failed
	}
	if _, e = part.Write(photo); e != nil {
		return failed
	}
	if form.Close() != nil {
		return failed
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "POST", b.base+"/bot"+b.token+"/sendDocument", &body)
	if e != nil {
		return failed
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	client := *b.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, e := client.Do(req)
	if e != nil {
		return failed
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return failed
	}
	var result struct {
		OK bool `json:"ok"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&result) != nil || !result.OK {
		return failed
	}
	return nil
}
