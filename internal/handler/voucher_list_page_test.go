package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
)

func voucherPageContext(query string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/vouchers?"+query, nil)
	return c
}

func TestParseVoucherPageRequest(t *testing.T) {
	req, err := parseVoucherPageRequest(voucherPageContext("page=3&per_page=100&status=unused&q=%20abc%20"))
	if err != nil || req.filter.Offset != 200 || req.filter.Limit != 100 || req.filter.Status != "unused" || req.filter.Query != "abc" {
		t.Fatalf("parsed %+v, %v", req, err)
	}
	if req, err = parseVoucherPageRequest(voucherPageContext("page=1")); err != nil || req.filter.Limit != voucherPageDefaultSize {
		t.Fatalf("default size: %+v, %v", req, err)
	}
	for _, bad := range []string{
		"page=0", "page=x", "page=1&per_page=0", "page=1&per_page=201", "page=1&status=sold", "page=1&batch=not-a-uuid",
		"page=1&q=" + strings.Repeat("a", voucherQueryMaxLen+1),
		"page=1&codes=" + strings.Repeat("A,", voucherCodesMax) + "A",
	} {
		if _, err := parseVoucherPageRequest(voucherPageContext(bad)); err == nil {
			t.Errorf("accepted %q", bad[:min(len(bad), 40)])
		}
	}
}

type voucherPageBody struct {
	Vouchers []struct {
		ID, Code, Status string
		InstallState     string `json:"install_state"`
	} `json:"vouchers"`
	Total        int            `json:"total"`
	StatusCounts map[string]int `json:"status_counts"`
}

func callVoucherPage(t *testing.T, h *VoucherHandler, biz, query string) voucherPageBody {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/vouchers?"+query, nil)
	c.Set(auth.ContextKeyBusinessID, biz)
	h.ListVouchers(c)
	if w.Code != 200 {
		t.Fatalf("%s: %d %s", query, w.Code, w.Body.String())
	}
	var body voucherPageBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestVoucherPagesAreStableFilteredAndTenantScoped(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	other, _ := loginFixture(t, db)
	h := loginHandler(db)
	p := &uncertainBatchProvisioner{installed: map[string]VoucherLoginSpec{}}
	h.Provisioner = p
	id := batchTestID()
	if w := batchTestCall(h, "POST", biz, id, batchTestBody(id, device, 1000), h.Issue); w.Code >= 300 {
		t.Fatalf("issue: %d %s", w.Code, w.Body.String())
	}

	first := callVoucherPage(t, h.Vouchers, biz, "page=1&per_page=1")
	second := callVoucherPage(t, h.Vouchers, biz, "page=2&per_page=1")
	if first.Total != 2 || len(first.Vouchers) != 1 || len(second.Vouchers) != 1 || first.Vouchers[0].ID == second.Vouchers[0].ID {
		t.Fatalf("pages overlap: %+v %+v", first, second)
	}
	if first.Vouchers[0].InstallState != "verified" {
		t.Fatalf("page lost install state: %+v", first.Vouchers[0])
	}
	if got := callVoucherPage(t, h.Vouchers, biz, "page=1&batch="+id); got.Total != 2 {
		t.Fatalf("batch filter: %+v", got)
	}
	code := first.Vouchers[0].Code
	if got := callVoucherPage(t, h.Vouchers, biz, "page=1&q="+url.QueryEscape(code)); got.Total != 1 || got.Vouchers[0].Code != code {
		t.Fatalf("search: %+v", got)
	}
	if got := callVoucherPage(t, h.Vouchers, biz, "page=1&codes="+url.QueryEscape(code)); got.Total != 1 {
		t.Fatalf("codes: %+v", got)
	}
	used := callVoucherPage(t, h.Vouchers, biz, "page=1&status=used")
	if used.Total != 0 || len(used.Vouchers) != 0 || used.StatusCounts["used"] != 0 {
		t.Fatalf("status filter: %+v", used)
	}
	if got := callVoucherPage(t, h.Vouchers, biz, "page=1&q="+url.QueryEscape("%")); got.Total != 0 {
		t.Fatalf("wildcard treated as a pattern: %s", fmt.Sprint(got.Total))
	}
	if got := callVoucherPage(t, h.Vouchers, other, "page=1&codes="+url.QueryEscape(code)); got.Total != 0 {
		t.Fatalf("another tenant read the code: %+v", got)
	}
}
