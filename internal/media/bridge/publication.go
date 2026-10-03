package bridge

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// A publication is a hosted design, NOT an entitlement or a playback grant.
// Typed fields prevent storing browser-supplied Jellyfin credentials or URLs.
type MediaTile struct {
	Title   string `json:"title"`
	Caption string `json:"caption"`
	Image   string `json:"image"`
}
type MediaPoster struct {
	Title string `json:"title"`
	Image string `json:"image"`
}
type MediaDesign struct {
	Libraries     []string               `json:"libraries,omitempty"`
	Payment       *MediaPayment          `json:"payment,omitempty"`
	Name          string                 `json:"name"`
	Headline      string                 `json:"headline"`
	Subtitle      string                 `json:"subtitle"`
	Accent        string                 `json:"accent"`
	Theme         string                 `json:"theme"`
	Price         int                    `json:"price"`
	Duration      int                    `json:"duration"`
	Movies        bool                   `json:"movies"`
	Series        bool                   `json:"series"`
	Music         bool                   `json:"music"`
	Featured      bool                   `json:"featured"`
	PosterDensity string                 `json:"posterDensity"`
	Layout        string                 `json:"layout"`
	Background    string                 `json:"background"`
	Tiles         map[string]MediaTile   `json:"tiles"`
	Posters       map[string]MediaPoster `json:"posters"`
}

// MediaPayment contains only public receiving details supplied by the owner.
// Bank/MMQR payment is manually reviewed; there are no gateway credentials.
type MediaPayment struct {
	BankName    string `json:"bank_name"`
	BankAccount string `json:"bank_account"`
	BankHolder  string `json:"bank_holder"`
	MMQR        string `json:"mmqr"`
}

func (p MediaPayment) Validate() error {
	if !designText(p.BankName, 100) || !designText(p.BankAccount, 100) || !designText(p.BankHolder, 160) || !designImage(p.MMQR) {
		return errors.New("invalid payment details")
	}
	bank := strings.TrimSpace(p.BankName) != "" || strings.TrimSpace(p.BankAccount) != "" || strings.TrimSpace(p.BankHolder) != ""
	if bank && (strings.TrimSpace(p.BankName) == "" || strings.TrimSpace(p.BankAccount) == "" || strings.TrimSpace(p.BankHolder) == "") {
		return errors.New("enter bank name, account number and account holder together")
	}
	if !bank && p.MMQR == "" {
		return errors.New("add your MMQR image or bank details")
	}
	return nil
}

var designColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var libraryID = regexp.MustCompile(`^(?:[0-9a-f]{32}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

func designText(s string, n int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= n && !strings.ContainsRune(s, 0)
}
func designImage(s string) bool {
	if s == "" {
		return true
	}
	if len(s) > 300000 {
		return false
	}
	var format string
	if strings.HasPrefix(s, "data:image/jpeg;base64,") {
		format = "jpeg"
	} else if strings.HasPrefix(s, "data:image/png;base64,") {
		format = "png"
	} else {
		return false
	}
	b, e := base64.StdEncoding.Strict().DecodeString(strings.SplitN(s, ",", 2)[1])
	if e != nil {
		return false
	}
	c, f, e := image.DecodeConfig(bytes.NewReader(b))
	return e == nil && f == format && c.Width > 0 && c.Height > 0 && c.Width <= 1200 && c.Height <= 1200
}
func (d MediaDesign) Validate() error {
	if len(d.Libraries) > 16 {
		return errors.New("select at most 16 libraries")
	}
	seen := map[string]bool{}
	for _, id := range d.Libraries {
		if !libraryID.MatchString(id) || seen[id] {
			return errors.New("invalid selected library")
		}
		seen[id] = true
	}
	invalid := errors.New("invalid media design; use bounded text, whole-number pricing and resized PNG/JPEG pictures")
	if d.Payment != nil {
		if e := d.Payment.Validate(); e != nil {
			return e
		}
	}
	if strings.TrimSpace(d.Name) == "" || !designText(d.Name, 160) || !designText(d.Headline, 160) || !designText(d.Subtitle, 160) || !designColour.MatchString(d.Accent) {
		return invalid
	}
	if d.Theme != "charcoal" && d.Theme != "paper" && d.Theme != "slate" {
		return invalid
	}
	if d.Layout != "shelves" && d.Layout != "lounge" {
		return invalid
	}
	if d.PosterDensity != "comfortable" && d.PosterDensity != "compact" {
		return invalid
	}
	if d.Price < 1 || d.Price > 1000000 || d.Duration < 1 || d.Duration > 1000000 || !designImage(d.Background) || len(d.Tiles) > 3 || len(d.Posters) > 4 {
		return invalid
	}
	for k, t := range d.Tiles {
		if (k != "movies" && k != "series" && k != "music") || !designText(t.Title, 80) || !designText(t.Caption, 160) || !designImage(t.Image) {
			return invalid
		}
	}
	for k, p := range d.Posters {
		if (k != "0" && k != "1" && k != "2" && k != "3") || !designText(p.Title, 80) || !designImage(p.Image) {
			return invalid
		}
	}
	return nil
}
func DecodeMediaDesign(r io.Reader) (MediaDesign, error) {
	var d MediaDesign
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if e := decoder.Decode(&d); e != nil {
		return d, e
	}
	var extra any
	if e := decoder.Decode(&extra); e != io.EOF {
		return d, errors.New("expected one design")
	}
	return d, d.Validate()
}

type Publication struct {
	Design      MediaDesign `json:"design"`
	PublicID    string      `json:"public_id"`
	Revision    int64       `json:"revision"`
	PublishedAt time.Time   `json:"published_at"`
}
type PublicationStore interface {
	Publication(context.Context, string) (Publication, error)
	Publish(context.Context, string, MediaDesign, int64) (Publication, error)
	Unpublish(context.Context, string, int64) error
	PublicPublication(context.Context, string) (Publication, error)
}

var ErrPublicationConflict = errors.New("publication changed; reload before publishing")

func scanPublication(row interface{ Scan(...any) error }) (Publication, error) {
	var p Publication
	var raw []byte
	e := row.Scan(&raw, &p.PublicID, &p.Revision, &p.PublishedAt)
	if e == nil {
		e = json.Unmarshal(raw, &p.Design)
	}
	return p, e
}
func (p *Postgres) Publication(ctx context.Context, bid string) (out Publication, e error) {
	e = p.tenant(ctx, bid, func(tx *sql.Tx) error {
		var err error
		out, err = scanPublication(tx.QueryRowContext(ctx, `SELECT design,public_id,revision,published_at FROM magic_media_publications WHERE business_id=$1`, bid))
		return err
	})
	return
}
func (p *Postgres) Publish(ctx context.Context, bid string, d MediaDesign, revision int64) (out Publication, e error) {
	if e = d.Validate(); e != nil {
		return
	}
	id, err := Secret()
	if err != nil {
		return out, err
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return out, err
	}
	e = p.tenant(ctx, bid, func(tx *sql.Tx) error {
		// Zero means create only. CAS avoids overwriting another operator's edits.
		var err error
		out, err = scanPublication(tx.QueryRowContext(ctx, `INSERT INTO magic_media_publications(business_id,public_id,design) SELECT $1,$2,$3 WHERE $4::bigint=0
 ON CONFLICT(business_id) DO UPDATE SET design=$3,revision=magic_media_publications.revision+1,published_at=now()
 WHERE magic_media_publications.revision=$4
 RETURNING design,public_id,revision,published_at`, bid, id, string(raw), revision))
		// Existing rows need a direct update when revision is nonzero.
		if errors.Is(err, sql.ErrNoRows) && revision > 0 {
			out, err = scanPublication(tx.QueryRowContext(ctx, `UPDATE magic_media_publications SET design=$2,revision=revision+1,published_at=now() WHERE business_id=$1 AND revision=$3 RETURNING design,public_id,revision,published_at`, bid, string(raw), revision))
		}
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPublicationConflict
		}
		return err
	})
	return
}
func (p *Postgres) Unpublish(ctx context.Context, bid string, revision int64) error {
	return p.tenant(ctx, bid, func(tx *sql.Tx) error {
		res, e := tx.ExecContext(ctx, `DELETE FROM magic_media_publications WHERE business_id=$1 AND revision=$2`, bid, revision)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return ErrPublicationConflict
		}
		return nil
	})
}
func (p *Postgres) PublicPublication(ctx context.Context, id string) (Publication, error) {
	// Resolve a random public sharing ID through the owner pool, then read under
	// tenant RLS. No internal business ID or connector secret is accepted publicly.
	var bid string
	e := p.db.QueryRowContext(ctx, `SELECT m.business_id FROM magic_media_publications m JOIN businesses b USING(business_id) WHERE public_id=$1 AND b.is_active`, id).Scan(&bid)
	if e != nil {
		return Publication{}, e
	}
	out, e := p.Publication(ctx, bid)
	if e == nil && out.PublicID != id {
		return Publication{}, sql.ErrNoRows
	}
	return out, e
}
