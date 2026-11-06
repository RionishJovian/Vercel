package discoverystore_test

import (
	"context"
	"encoding/binary"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/discovery"
	"mega-magic/internal/discoverystore"
	"mega-magic/internal/handler"
)

func mndp(mac [6]byte, board string) []byte {
	b := []byte{0, 0, 0, 1}
	add := func(t uint16, v []byte) {
		b = binary.BigEndian.AppendUint16(b, t)
		b = binary.BigEndian.AppendUint16(b, uint16(len(v)))
		b = append(b, v...)
	}
	add(1, mac[:])
	add(5, []byte("hAP"))
	add(7, []byte("7.25 2026-10-22 14:12:00"))
	add(8, []byte("MikroTik"))
	add(11, []byte("TEST-SOFTID"))
	add(12, []byte(board))
	add(16, []byte("bridge/wifi2"))
	add(17, []byte{192, 168, 88, 1})
	return b
}

// The real helper client, the real handlers and the real database, over TLS: the whole path except the owner's cookie login.
func TestHelperToServerEndToEnd(t *testing.T) {
	store, business, ok := discoverystore.E2EHarness()
	if !ok {
		t.Skip("set MM_TEST_DB_URL to run the end-to-end discovery test")
	}
	ctx := context.Background()
	created, err := store.Create(ctx, business, "11111111-1111-4111-8111-111111111111", "")
	if err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	h := handler.NewDiscoveryHandler(store)
	r := gin.New()
	r.POST("/api/v1/discovery/pair", h.Pair)
	r.POST("/api/v1/discovery/sessions/:id/observations", h.Observations)
	srv := httptest.NewTLSServer(r)
	defer srv.Close()
	up, err := discovery.NewUploader(srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := up.Pair(ctx, "AAAAAA-BBBBBBBB"); err == nil {
		t.Fatal("a wrong code must not pair")
	}
	sess, err := up.Pair(ctx, created.PairingCode)
	if err != nil || sess.SessionID != created.SessionID {
		t.Fatalf("pair: %+v %v", sess, err)
	}
	if _, err := up.Pair(ctx, created.PairingCode); err == nil {
		t.Fatal("the code is single use")
	}

	// A packet in the layout verified against a real hAP ax2 goes through the parser, the helper payload, the server and the database.
	now := time.Now()
	o1, err := discovery.MikroTik{}.Parse(mndp([6]byte{0x48, 0xA9, 0x8A, 0, 0, 6}, "C52iG-5HaxD2HaxD"), netip.MustParseAddrPort("192.168.88.1:5678"), now)
	if err != nil {
		t.Fatal(err)
	}
	accepted, dropped, err := up.Upload(ctx, sess, []discovery.Observation{o1})
	if err != nil || accepted != 1 || dropped != 0 {
		t.Fatalf("upload: %d %d %v", accepted, dropped, err)
	}
	if _, _, err := up.Upload(ctx, sess, []discovery.Observation{o1}); err != discovery.ErrSlowDown {
		t.Fatalf("an immediate second upload must be told to slow down: %v", err)
	}

	v, err := store.Get(ctx, business, created.SessionID)
	if err != nil || v.State != "paired" || len(v.Candidates) != 1 {
		t.Fatalf("the owner's view: %+v %v", v, err)
	}
	c := v.Candidates[0]
	if c.Board != "C52iG-5HaxD2HaxD" || c.Version != "7.25 2026-10-22 14:12:00" || c.SourceIP != "192.168.88.1" || c.Status != "nearby_unverified" || c.Identity != "hAP" {
		t.Fatalf("candidate: %+v", c)
	}
	if len(c.DeviceKey) != 19 || c.DeviceKey[:3] != "sw:" {
		t.Fatalf("the raw software id must never be stored, only its hash: %q", c.DeviceKey)
	}

	if err := store.Cancel(ctx, business, created.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := up.Upload(ctx, sess, []discovery.Observation{o1}); err != discovery.ErrSessionEnded {
		t.Fatalf("after cancel the helper must be told the session ended: %v", err)
	}
}
