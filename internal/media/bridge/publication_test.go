package bridge

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"strings"
	"testing"
)

func validDesign() MediaDesign {
	return MediaDesign{Name: "Test cinema", Headline: "Welcome", Accent: "#eeeeee", Theme: "charcoal", Price: 1500, Duration: 24, Layout: "shelves", PosterDensity: "comfortable", Movies: true, Payment: &MediaPayment{BankName: "Test Bank", BankAccount: "1234", BankHolder: "Test Owner"}}
}
func TestMediaDesignValidation(t *testing.T) {
	d := validDesign()
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
	var pngBytes bytes.Buffer
	png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	d.Background = "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes.Bytes())
	if e := d.Validate(); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{"data:image/png;base64,aGVsbG8=", "https://evil.test/image", "data:image/svg+xml;base64,PHN2Zz4=", strings.Repeat("x", 300001)} {
		badDesign := d
		badDesign.Background = bad
		if badDesign.Validate() == nil {
			t.Fatal("accepted invalid image")
		}
	}
	for _, change := range []func(*MediaDesign){func(d *MediaDesign) { d.Name = "" }, func(d *MediaDesign) { d.Price = 0 }, func(d *MediaDesign) { d.Duration = -1 }, func(d *MediaDesign) { d.Theme = "evil" }, func(d *MediaDesign) { d.Tiles = map[string]MediaTile{"secret": {}} }} {
		d := validDesign()
		change(&d)
		if d.Validate() == nil {
			t.Fatal("accepted invalid design")
		}
	}
	raw, _ := json.Marshal(validDesign())
	for _, bad := range []string{strings.TrimSuffix(string(raw), "}") + `,"api_key":"secret"}`, string(raw) + `{}`, strings.Replace(string(raw), `"price":1500`, `"price":1.5`, 1)} {
		if _, e := DecodeMediaDesign(strings.NewReader(bad)); e == nil {
			t.Fatal("accepted unknown field, trailing object or fractional price")
		}
	}
}
func TestPostgresPublicationIsolation(t *testing.T) {
	store, db := mediaTestStore(t)
	migration, e := os.ReadFile("../../../migrations/20261003120000_magic_media_publications.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(migration)); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	a := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	b := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	if _, e = db.Exec(`INSERT INTO businesses(business_id) VALUES($1),($2)`, a, b); e != nil {
		t.Fatal(e)
	}
	p, e := store.Publish(ctx, a, validDesign(), 0)
	if e != nil {
		t.Fatal(e)
	}
	if p.Revision != 1 || len(p.PublicID) != 64 {
		t.Fatal("missing publication result")
	}
	if _, e = store.Publication(ctx, b); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal("cross-tenant publication visible")
	}
	if _, e = store.Publish(ctx, a, validDesign(), 0); !errors.Is(e, ErrPublicationConflict) {
		t.Fatal("duplicate create allowed")
	}
	d := validDesign()
	d.Price = 2500
	p2, e := store.Publish(ctx, a, d, 1)
	if e != nil {
		t.Fatal(e)
	}
	if p2.Revision != 2 || p2.PublicID != p.PublicID || p2.Design.Price != 2500 {
		t.Fatal("update not persisted")
	}
	if _, e = store.Publish(ctx, a, d, 1); !errors.Is(e, ErrPublicationConflict) {
		t.Fatal("stale write accepted")
	}
	if e = store.Unpublish(ctx, b, 2); !errors.Is(e, ErrPublicationConflict) {
		t.Fatal("cross tenant deletion allowed")
	}
	public, e := store.PublicPublication(ctx, p.PublicID)
	if e != nil || public.Design.Price != 2500 {
		t.Fatal("public read failed", e)
	}
	if _, e = db.Exec(`UPDATE businesses SET is_active=false WHERE business_id=$1`, a); e != nil {
		t.Fatal(e)
	}
	if _, e = store.PublicPublication(ctx, p.PublicID); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal("suspended business publication visible")
	}
	if _, e = db.Exec(`UPDATE businesses SET is_active=true WHERE business_id=$1`, a); e != nil {
		t.Fatal(e)
	}
	if e = store.Unpublish(ctx, a, 2); e != nil {
		t.Fatal(e)
	}
	if _, e = store.PublicPublication(ctx, p.PublicID); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal("withdrawn page visible")
	}
	newPage, e := store.Publish(ctx, a, validDesign(), 0)
	if e != nil || newPage.PublicID == p.PublicID {
		t.Fatal("republish reused withdrawn link", e)
	}
}
