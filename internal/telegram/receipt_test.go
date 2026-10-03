package telegram

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReceiptSentWithOwnerDecisionButtons(t *testing.T) {
	var photo bytes.Buffer
	_ = png.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	id := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/botTEST/sendDocument" || r.URL.RawQuery != "" {
			t.Error("wrong endpoint")
		}
		if e := r.ParseMultipartForm(6 << 20); e != nil {
			t.Fatal(e)
		}
		defer r.MultipartForm.RemoveAll()
		if r.FormValue("chat_id") != "123" || r.FormValue("caption") != "Review media receipt" {
			t.Error("wrong owner or caption")
		}
		var markup struct {
			Buttons [][]struct {
				Data string `json:"callback_data"`
			} `json:"inline_keyboard"`
		}
		if json.Unmarshal([]byte(r.FormValue("reply_markup")), &markup) != nil || len(markup.Buttons) != 1 || len(markup.Buttons[0]) != 2 {
			t.Fatal("bad buttons")
		}
		if markup.Buttons[0][0].Data != "approve "+id || markup.Buttons[0][1].Data != "reject "+id {
			t.Error("wrong decision id")
		}
		f, header, e := r.FormFile("document")
		if e != nil {
			t.Fatal(e)
		}
		defer f.Close()
		data, _ := io.ReadAll(f)
		if header.Filename != "receipt.png" || !bytes.Equal(data, photo.Bytes()) {
			t.Error("missing receipt")
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer s.Close()
	b := NewBot("TEST", WithBase(s.URL))
	if e := b.SendReceipt("123", "Review media receipt", id, photo.Bytes()); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{strings.Repeat("a", 64), "approve other", "id\nsecret", ""} {
		if e := b.SendReceipt("123", "Review", bad, photo.Bytes()); e == nil {
			t.Error("bad callback accepted")
		}
	}
	if e := b.SendReceipt("123", "Review", id, []byte("not an image")); e == nil {
		t.Error("bad proof accepted")
	}
	if calls != 1 {
		t.Fatal("invalid receipt contacted bot")
	}
}
