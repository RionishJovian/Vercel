package jellyfin

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var mediaID = regexp.MustCompile(`^(?:[0-9a-f]{32}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)
var singleRange = regexp.MustCompile(`^bytes=([0-9]*)-([0-9]*)$`)

// CatalogItem is deliberately not a Jellyfin BaseItemDto: source paths,
// credentials, user playback history and arbitrary artwork URLs are excluded.
type CatalogItem struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	Type         string `json:"Type"`
	RunTimeTicks int64  `json:"RunTimeTicks"`
}

func playableType(t string) bool {
	return t == "Movie" || t == "Episode" || t == "Audio" || t == "Video"
}
func validCatalogItem(i CatalogItem) bool {
	return mediaID.MatchString(i.ID) && i.Name != "" && len(i.Name) <= 512 && !strings.ContainsAny(i.Name, "\x00\r\n") && playableType(i.Type) && i.RunTimeTicks >= 0
}

func ValidateCatalog(items []CatalogItem) bool {
	if len(items) > 100 {
		return false
	}
	for _, item := range items {
		if !validCatalogItem(item) {
			return false
		}
	}
	return true
}

// ValidateStreamMetadata is also used at the cloud boundary: a paired computer
// cannot make the guest forward an arbitrary range or non-media response.
func ValidateStreamMetadata(status int, length int64, requested, returned string) bool {
	if length < 0 || !validRange(requested) {
		return false
	}
	if status == 200 {
		return requested == "" && returned == ""
	}
	return status == 206 && validContentRange(returned, length) && matchesRequestedRange(requested, returned)
}

// Catalog reads only an owner-selected library, at most 100 items per page.
// Callers must bind that selection to the business before a guest can browse.
func (c *Client) Catalog(ctx context.Context, library string, start int) ([]CatalogItem, error) {
	if !mediaID.MatchString(library) || start < 0 || start > 100000 {
		return nil, errors.New("invalid media catalog request")
	}
	query := url.Values{"ParentId": {library}, "Recursive": {"true"}, "Limit": {"100"}, "StartIndex": {strconv.Itoa(start)}, "IncludeItemTypes": {"Movie,Episode,Audio,Video"}, "EnableUserData": {"false"}, "EnableImages": {"false"}}
	var body struct {
		Items []CatalogItem `json:"Items"`
	}
	if e := c.request(ctx, http.MethodGet, "/Items", query, nil, true, &body); e != nil {
		return nil, e
	}
	if len(body.Items) > 100 {
		return nil, errors.New("invalid local catalog response")
	}
	for _, i := range body.Items {
		if !validCatalogItem(i) {
			return nil, errors.New("invalid local catalog response")
		}
	}
	if body.Items == nil {
		body.Items = []CatalogItem{}
	}
	return body.Items, nil
}

// MediaStream exposes only safe streaming metadata, never the upstream URL,
// headers containing credentials, source paths or Jellyfin's error body.
type MediaStream struct {
	Body          io.ReadCloser
	Status        int
	ContentType   string
	ContentLength int64
	ContentRange  string
}
type streamBody struct {
	io.ReadCloser
	client *http.Client
}

func (b *streamBody) Close() error {
	e := b.ReadCloser.Close()
	b.client.CloseIdleConnections()
	return e
}

func validRange(s string) bool {
	if s == "" {
		return true
	}
	if len(s) > 64 {
		return false
	}
	match := singleRange.FindStringSubmatch(s)
	if match == nil || match[1] == "" && match[2] == "" {
		return false
	}
	var a, b uint64
	var e error
	if match[1] != "" {
		a, e = strconv.ParseUint(match[1], 10, 63)
		if e != nil {
			return false
		}
	}
	if match[2] != "" {
		b, e = strconv.ParseUint(match[2], 10, 63)
		if e != nil {
			return false
		}
		if match[1] == "" {
			return b > 0
		}
		return b >= a
	}
	return true
}

// OpenMedia is local-only. A guest pass must be authorized before calling it,
// and access.Copy enforces expiry while transferring the returned body. A
// caller cannot inject a path/URL. Ancestors are verified rather than trusting
// that Jellyfin's Items IDs filter also applies the parent filter.
func (c *Client) OpenMedia(ctx context.Context, library, item, byteRange string) (MediaStream, error) {
	denied := errors.New("local media stream unavailable")
	var out MediaStream
	deadline, ok := ctx.Deadline()
	if !ok || !deadline.After(time.Now()) || deadline.After(time.Now().Add(30*24*time.Hour)) || !mediaID.MatchString(library) || !mediaID.MatchString(item) || !validRange(byteRange) || c.token == "" {
		return out, denied
	}
	var parents []struct {
		ID string `json:"Id"`
	}
	if e := c.request(ctx, http.MethodGet, "/Items/"+item+"/Ancestors", nil, nil, true, &parents); e != nil {
		return out, denied
	}
	if len(parents) > 64 {
		return out, denied
	}
	allowed := false
	for _, parent := range parents {
		if parent.ID == library {
			allowed = true
		}
	}
	if !allowed {
		return out, denied
	}
	var info CatalogItem
	if e := c.request(ctx, http.MethodGet, "/Items/"+item, nil, nil, true, &info); e != nil || info.ID != item || !validCatalogItem(info) {
		return out, denied
	}
	path := "/Videos/" + item + "/stream"
	if info.Type == "Audio" {
		path = "/Audio/" + item + "/stream"
	}
	endpoint := c.base
	endpoint.Path = path
	endpoint.RawQuery = url.Values{"Static": {"true"}}.Encode()
	request, e := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if e != nil {
		return out, denied
	}
	request.Header.Set("Authorization", `MediaBrowser Token="`+url.PathEscape(c.token)+`"`)
	if byteRange != "" {
		request.Header.Set("Range", byteRange)
	}
	// Metadata requests keep their short total timeout; actual streams use the
	// pass deadline with a bounded header wait, not a 15-second movie timeout.
	transport := c.http.Transport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 5 * time.Second
	streamClient := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := streamClient.Do(request)
	if e != nil {
		streamClient.CloseIdleConnections()
		return out, denied
	}
	failed := func() (MediaStream, error) {
		response.Body.Close()
		streamClient.CloseIdleConnections()
		return MediaStream{}, denied
	}
	contentType, _, e := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if e != nil || !(strings.HasPrefix(contentType, "video/") || strings.HasPrefix(contentType, "audio/")) || response.StatusCode != 200 && response.StatusCode != 206 {
		return failed()
	}
	contentRange := response.Header.Get("Content-Range")
	if len(contentRange) > 100 || strings.ContainsAny(contentRange, "\r\n") {
		return failed()
	}
	if byteRange != "" && response.StatusCode != 206 {
		return failed()
	}
	if response.StatusCode == 206 && (!validContentRange(contentRange, response.ContentLength) || !matchesRequestedRange(byteRange, contentRange)) {
		return failed()
	}
	return MediaStream{Body: &streamBody{ReadCloser: response.Body, client: streamClient}, Status: response.StatusCode, ContentType: contentType, ContentLength: response.ContentLength, ContentRange: contentRange}, nil
}

var contentRangePattern = regexp.MustCompile(`^bytes ([0-9]+)-([0-9]+)/([0-9]+)$`)

// A structurally valid upstream range must also answer the requested seek.
// Ends beyond EOF and suffixes longer than the file are clipped to its size.
func matchesRequestedRange(requested, returned string) bool {
	r := singleRange.FindStringSubmatch(requested)
	m := contentRangePattern.FindStringSubmatch(returned)
	if r == nil || m == nil || !validRange(requested) {
		return false
	}
	first, e1 := strconv.ParseInt(m[1], 10, 64)
	last, e2 := strconv.ParseInt(m[2], 10, 64)
	total, e3 := strconv.ParseInt(m[3], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil || total <= 0 {
		return false
	}
	wantFirst, wantLast := int64(0), total-1
	if r[1] == "" {
		suffix, _ := strconv.ParseInt(r[2], 10, 64)
		if suffix < total {
			wantFirst = total - suffix
		}
	} else {
		wantFirst, _ = strconv.ParseInt(r[1], 10, 64)
		if r[2] != "" {
			end, _ := strconv.ParseInt(r[2], 10, 64)
			if end < wantLast {
				wantLast = end
			}
		}
	}
	return first == wantFirst && last == wantLast && first <= last
}

func validContentRange(s string, length int64) bool {
	m := contentRangePattern.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	first, e := strconv.ParseInt(m[1], 10, 64)
	if e != nil {
		return false
	}
	last, e := strconv.ParseInt(m[2], 10, 64)
	if e != nil {
		return false
	}
	total, e := strconv.ParseInt(m[3], 10, 64)
	return e == nil && first >= 0 && last >= first && total > last && length == last-first+1
}
