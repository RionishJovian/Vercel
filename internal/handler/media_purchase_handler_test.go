package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/media/bridge"
)

type fakeMediaPurchases struct {
	calls     int
	submitted bool
	pass      bridge.MediaPass
	deny      bool
}

func (f *fakeMediaPurchases) ResolveMediaPublication(context.Context, string) (string, error) {
	f.calls++
	return "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", nil
}
func (f *fakeMediaPurchases) PublicPublication(context.Context, string) (bridge.Publication, error) {
	return bridge.Publication{Design: bridge.MediaDesign{Payment: &bridge.MediaPayment{BankName: "Bank", BankAccount: "123", BankHolder: "Owner"}}}, nil
}
func (f *fakeMediaPurchases) ResolveGuestMediaPass(context.Context, string, string, string) (string, error) {
	f.calls++
	if f.deny {
		return "", bridge.ErrPassDenied
	}
	return "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", nil
}
func (f *fakeMediaPurchases) CreateMediaPass(context.Context, string, string, string, string) (bridge.MediaPass, error) {
	f.calls++
	return f.pass, nil
}
func (f *fakeMediaPurchases) MediaPass(context.Context, string, string, string) (bridge.MediaPass, error) {
	return f.pass, nil
}
func (f *fakeMediaPurchases) SubmitMediaReceipt(_ context.Context, _, _, _, path, hash string) (bridge.MediaReceipt, error) {
	f.submitted = true
	return bridge.MediaReceipt{Path: path, Hash: hash, ReviewID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}, nil
}
func (f *fakeMediaPurchases) MediaReceiptSubmitted(context.Context, string, string, string) (bool, error) {
	return f.submitted, nil
}

func TestMediaPurchaseGuestBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	f := &fakeMediaPurchases{pass: bridge.MediaPass{ID: strings.Repeat("a", 64), Amount: 1500, Currency: "MMK", State: "pending", PaymentDeadline: time.Now().Add(time.Minute)}}
	h := &MediaPurchaseHandler{Store: f, Ready: true, ReceiptRoot: t.TempDir(), ReviewReady: func(context.Context, string) bool { return true }, Notify: func(context.Context, string, bridge.MediaPass, bridge.MediaReceipt, []byte) error {
		return errors.New("telegram unavailable")
	}}
	r := gin.New()
	r.POST("/:id/checkout", h.Checkout)
	r.POST("/:id/passes/:passId/receipt", h.Receipt)
	r.GET("/:id/passes/:passId", h.Status)
	pub := strings.Repeat("b", 64)
	guest := strings.Repeat("c", 64)
	post := func(body string, secret string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/"+pub+"/checkout", strings.NewReader(body))
		req.Header.Set("Cookie", "mm_access=staff-session")
		if secret != "" {
			req.Header.Set("Authorization", "Bearer "+secret)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := post(`{"checkout_key":"`+guest+`"}`, ""); w.Code != 401 || f.calls != 0 {
		t.Fatal("staff cookie authenticated guest")
	}
	if w := post(`{"checkout_key":"`+guest+`","business_id":"evil"}`, guest); w.Code != 400 || f.calls != 0 {
		t.Fatal("business override accepted")
	}
	if w := post(`{"checkout_key":"`+guest+`"}`, guest); w.Code != 200 {
		t.Fatal("checkout failed", w.Code, w.Body.String())
	}
	var picture bytes.Buffer
	_ = png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, _ := form.CreateFormFile("receipt", "proof.png")
	_, _ = part.Write(picture.Bytes())
	_ = form.Close()
	req := httptest.NewRequest("POST", "/"+pub+"/passes/"+f.pass.ID+"/receipt", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+guest)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var result map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if w.Code != 200 || result["receipt_submitted"] != true || result["owner_notified"] != false || !f.submitted || f.pass.State != "pending" || strings.Contains(w.Body.String(), h.ReceiptRoot) {
		t.Fatal("receipt error handling or privacy", w.Code, w.Body.String())
	}
	f.deny = true
	req = httptest.NewRequest("GET", "/"+pub+"/passes/"+f.pass.ID, nil)
	req.Header.Set("Authorization", "Bearer "+guest)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatal("wrong guest not denied")
	}
	h.Ready = false
	if w := post(`{"checkout_key":"`+guest+`"}`, guest); w.Code != 503 {
		t.Fatal("unverified sales enabled")
	}
}
