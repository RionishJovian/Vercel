package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/media/bridge"
	"mega-magic/internal/model"
)

type catalogueDevices struct {
	business string
	err      error
}

func (d catalogueDevices) GetDeviceByID(string) (*model.Device, error) {
	if d.err != nil {
		return nil, d.err
	}
	if d.business == "" {
		return nil, nil
	}
	return &model.Device{BusinessID: d.business}, nil
}

// catalogueStore records which business the handler asked for, so the test can
// prove the tenant is derived from the DEVICE and never from client input.
type catalogueStore struct {
	publications map[string]bridge.Publication
	asked        string
	err          error
}

func (s *catalogueStore) Publication(_ context.Context, businessID string) (bridge.Publication, error) {
	s.asked = businessID
	if s.err != nil {
		return bridge.Publication{}, s.err
	}
	p, ok := s.publications[businessID]
	if !ok {
		return bridge.Publication{}, sql.ErrNoRows
	}
	return p, nil
}

func (s *catalogueStore) Publish(context.Context, string, bridge.MediaDesign, int64) (bridge.Publication, error) {
	return bridge.Publication{}, errors.New("not used")
}
func (s *catalogueStore) Unpublish(context.Context, string, int64) error { return errors.New("not used") }
func (s *catalogueStore) PublicPublication(context.Context, string) (bridge.Publication, error) {
	return bridge.Publication{}, sql.ErrNoRows
}

func catalogueRequest(deviceID string) (*httptest.ResponseRecorder, *gin.Context) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/portal/"+deviceID+"/media", nil)
	c.Params = gin.Params{{Key: "deviceId", Value: deviceID}}
	return rec, c
}

// statusOf reads the status the handler wrote. Recorder.Code only reflects it
// after WriteHeader, and Status() on the context is what c.JSON/c.Status set.
func statusOf(rec *httptest.ResponseRecorder, c *gin.Context) int {
	if code := c.Writer.Status(); code != 0 {
		return code
	}
	return statusOf(rec, c)
}

func published() bridge.Publication {
	return bridge.Publication{
		Design: bridge.MediaDesign{
			Libraries: []string{"Films"},
			Tiles:     map[string]bridge.MediaTile{"a": {Title: "A Film", Caption: "cap"}},
		},
		Revision: 3,
	}
}

func TestCatalogueServesTheOwningBusinessOnly(t *testing.T) {
	store := &catalogueStore{publications: map[string]bridge.Publication{"biz-1": published()}}
	h := &MediaCatalogueHandler{Store: store, DeviceIndex: catalogueDevices{business: "biz-1"}, Ready: true}

	rec, c := catalogueRequest("dev-1")
	h.PublicCatalogue(c)

	if statusOf(rec, c) != http.StatusOK {
		t.Fatalf("want 200, got %d", statusOf(rec, c))
	}
	if store.asked != "biz-1" {
		t.Fatalf("tenant must come from the device, asked %q", store.asked)
	}
	if body := rec.Body.String(); !contains(body, "A Film") {
		t.Fatalf("catalogue missing tile: %s", body)
	}
}

func TestCatalogueCannotCrossTenants(t *testing.T) {
	// The device belongs to biz-2, which published nothing. biz-1 has a library
	// and must NOT leak into that guest's response.
	store := &catalogueStore{publications: map[string]bridge.Publication{"biz-1": published()}}
	h := &MediaCatalogueHandler{Store: store, DeviceIndex: catalogueDevices{business: "biz-2"}, Ready: true}

	rec, c := catalogueRequest("dev-2")
	h.PublicCatalogue(c)

	if statusOf(rec, c) != http.StatusNotFound {
		t.Fatalf("want 404 for a tenant with no publication, got %d", statusOf(rec, c))
	}
	if store.asked != "biz-2" {
		t.Fatalf("must read only the device's own tenant, asked %q", store.asked)
	}
	if contains(rec.Body.String(), "A Film") {
		t.Fatal("another business's catalogue leaked")
	}
}

func TestCatalogueHidesUnknownDeviceAndUnpublishedBusiness(t *testing.T) {
	store := &catalogueStore{publications: map[string]bridge.Publication{"biz-1": published()}}
	h := &MediaCatalogueHandler{Store: store, DeviceIndex: catalogueDevices{business: ""}, Ready: true}

	rec, c := catalogueRequest("dev-unknown")
	h.PublicCatalogue(c)

	if statusOf(rec, c) != http.StatusNotFound {
		t.Fatalf("want 404 for an unknown device, got %d", statusOf(rec, c))
	}
	if store.asked != "" {
		t.Fatal("must not read any tenant for an unknown device")
	}
}

func TestCatalogueIsHiddenWhenTableNotReady(t *testing.T) {
	store := &catalogueStore{publications: map[string]bridge.Publication{"biz-1": published()}}
	h := &MediaCatalogueHandler{Store: store, DeviceIndex: catalogueDevices{business: "biz-1"}, Ready: false}

	rec, c := catalogueRequest("dev-1")
	h.PublicCatalogue(c)

	if statusOf(rec, c) != http.StatusNotFound {
		t.Fatalf("want 404 when RLS is not verified, got %d", statusOf(rec, c))
	}
	if store.asked != "" {
		t.Fatal("must not read the table when it is not known to be protected")
	}
}

func TestCatalogueHidesBusinessWithNoLibraries(t *testing.T) {
	empty := bridge.Publication{Design: bridge.MediaDesign{}}
	store := &catalogueStore{publications: map[string]bridge.Publication{"biz-1": empty}}
	h := &MediaCatalogueHandler{Store: store, DeviceIndex: catalogueDevices{business: "biz-1"}, Ready: true}

	rec, c := catalogueRequest("dev-1")
	h.PublicCatalogue(c)

	if statusOf(rec, c) != http.StatusNotFound {
		t.Fatalf("want 404 when no libraries are published, got %d", statusOf(rec, c))
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
