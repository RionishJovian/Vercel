package hub

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestManagementMigrationSignedScopeAndResponseBinding(t *testing.T) {
	for _, action := range []string{"candidate", "stage", "commit", "finalize", "abort"} {
		t.Run(action, func(t *testing.T) {
			o := opts()
			o.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("x-mm-scope") != "peers:migrate" || !strings.HasSuffix(r.URL.Path, "/management-migration/"+action) {
					t.Fatal("wrong signed route")
				}
				raw, _ := io.ReadAll(r.Body)
				if action == "candidate" {
					if r.Method != "GET" || len(raw) != 0 {
						t.Fatal("candidate mutated")
					}
				} else if r.Method != "POST" || !strings.Contains(string(raw), "operationId") {
					t.Fatal("unbound mutation")
				}
				return response(200, `{"peerId":"peer","tenantId":"tenant-a","routerId":"router-a","routerPublicKey":"key","operationId":"operation","oldAddress":"10.77.0.2/32","newAddress":"10.77.0.3/32","phase":"candidate"}`), nil
			})
			c, _ := New(o)
			_, e := c.ManagementMigration(context.Background(), identity, action, ManagementMove{OperationID: "operation", OldAddress: "10.77.0.2/32", NewAddress: "10.77.0.3/32", RouterPublicKey: "key"})
			if e != nil {
				t.Fatal(e)
			}
		})
	}
	for _, bad := range []string{`"tenantId":"foreign"`, `"newAddress":"8.8.8.8/32"`, `"routerPublicKey":"other"`} {
		t.Run(bad, func(t *testing.T) {
			o := opts()
			o.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
				body := `{"peerId":"peer","tenantId":"tenant-a","routerId":"router-a","routerPublicKey":"key","operationId":"operation","oldAddress":"10.77.0.2/32","newAddress":"10.77.0.3/32","phase":"staged"}`
				switch {
				case strings.Contains(bad, "tenantId"):
					body = strings.Replace(body, `"tenantId":"tenant-a"`, bad, 1)
				case strings.Contains(bad, "newAddress"):
					body = strings.Replace(body, `"newAddress":"10.77.0.3/32"`, bad, 1)
				default:
					body = strings.Replace(body, `"routerPublicKey":"key"`, bad, 1)
				}
				return response(200, body), nil
			})
			c, _ := New(o)
			if _, e := c.ManagementMigration(context.Background(), identity, "stage", ManagementMove{OperationID: "operation", OldAddress: "10.77.0.2/32", NewAddress: "10.77.0.3/32", RouterPublicKey: "key"}); e == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
}
