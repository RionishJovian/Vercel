// Local UI/collector integration harness. Synthetic RouterOS output only; this
// program never connects to a router or a production backend. No write routes.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/routertelemetry"
)

const device = "telemetry-preview"

type target struct{}

func (target) TargetForDevice(_ context.Context, b, d string) (*routeros.DeviceTarget, error) {
	if b != "preview" || d != device {
		return nil, routeros.ErrNoTarget
	}
	return &routeros.DeviceTarget{DeviceID: d, Addr: "192.0.2.1:22", User: "fixture", Kind: "password", Secret: "fixture-only", Fingerprint: "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}, nil
}

type runner struct{ started time.Time }

func (r runner) Run(_ context.Context, command string) (string, error) {
	elapsed := uint64(time.Since(r.started).Seconds())
	switch command {
	case routeros.SystemResourceCommand:
		return fmt.Sprintf("uptime: 2d4h%ds\ncpu-load: %d%%\ntotal-memory: 1GiB\nfree-memory: 640MiB\nboard-name: hAP ax2 (fixture)\nversion: 7.24.2 (fixture)\n", elapsed, 18+elapsed%8), nil
	case routeros.TelemetryInterfacesCommand:
		return fmt.Sprintf("id: *1\nname: ether1\ntype: ether\nrunning: true\ndisabled: false\nrx-byte: %d\ntx-byte: %d\nid: *2\nname: bridge-guests\ntype: bridge\nrunning: true\ndisabled: false\nrx-byte: %d\ntx-byte: %d\nid: *3\nname: wifi2\ntype: wifi\nrunning: true\ndisabled: false\nrx-byte: %d\ntx-byte: %d\n", 100000000+elapsed*1250000, 20000000+elapsed*250000, 20000000+elapsed*250000, 100000000+elapsed*1250000, 3000000+elapsed*125000, 500000+elapsed*25000), nil
	case routeros.TelemetryHotspotCountCommand:
		return "3", nil
	case routeros.TelemetryDHCPCountCommand:
		return "8", nil
	}
	return "", fmt.Errorf("command outside read-only fixture")
}
func main() {
	address := flag.String("listen", "127.0.0.1:9183", "Loopback preview listener")
	web := flag.String("web", "web", "Web root")
	flag.Parse()
	if !strings.HasPrefix(*address, "127.0.0.1:") {
		log.Fatal("preview must stay on loopback")
	}
	started := time.Now()
	reader := routeros.NewDeviceApplier(target{}, time.Second, func(routeros.SSHConfig) routeros.CommandRunner { return runner{started} }, "", 0, 0)
	service := routertelemetry.New(reader)
	jsonReply := func(w http.ResponseWriter, status int, data any) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(data)
	}
	files := http.FileServer(http.Dir(*web))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			jsonReply(w, 405, map[string]any{"error": "Preview has no write operations"})
			return
		}
		path := r.URL.Path
		switch path {
		case "/api/v1/me":
			jsonReply(w, 200, map[string]any{"user": map[string]any{"user_role": "OWNER", "full_name": "Preview Owner"}, "business": map[string]any{"company_name": "Telemetry preview"}, "subscription": map[string]any{"mode": "subscription_only"}})
			return
		case "/api/v1/me/profile":
			jsonReply(w, 200, map[string]any{"full_name": "Preview Owner", "user_role": "OWNER", "language": "en"})
			return
		case "/api/v1/sites":
			jsonReply(w, 200, map[string]any{"sites": []any{map[string]any{"site_id": "preview-site", "name": "Cafe · synthetic preview"}}})
			return
		case "/api/v1/routers":
			jsonReply(w, 200, map[string]any{"routers": []any{map[string]any{"device_id": device, "site_id": "preview-site", "hardware_name": "hAP ax² telemetry preview", "vendor": "mikrotik", "deployment_layer": "MAIN_GATEWAY"}}})
			return
		case "/api/v1/fleet/health":
			jsonReply(w, 200, map[string]any{"peers": []any{map[string]any{"device_id": device, "state": "up"}}})
			return
		case "/api/v1/routers/" + device + "/capabilities":
			jsonReply(w, 200, map[string]any{"capabilities": map[string]any{"board_name": "hAP ax² · fixture", "routeros_version": "7.24.2 · fixture"}})
			return
		case "/api/v1/routers/" + device + "/network/inventory":
			jsonReply(w, 200, map[string]any{"inventory": map[string]any{"interfaces": []any{}, "objects": map[string]any{}, "available": map[string]any{}, "write_enabled": false}})
			return
		case "/api/v1/routers/" + device + "/telemetry":
			view, err := service.Read(r.Context(), "preview", device)
			if err != nil {
				jsonReply(w, 503, map[string]any{"error": "Fixture failed"})
				return
			}
			jsonReply(w, 200, map[string]any{"telemetry": view, "read_only": true, "router_changed": false, "fixture": true})
			return
		case "/", "/app.html":
			content, err := os.ReadFile(filepath.Join(*web, "app.html"))
			if err != nil {
				http.Error(w, "Preview source missing", 500)
				return
			}
			banner := `<div style="padding:12px 16px;text-align:center;border-bottom:1px solid var(--border);background:var(--surface-elevated);color:var(--foreground);font:13px system-ui">LOCAL TELEMETRY PREVIEW · Synthetic RouterOS readings · Production collector and UI code · No router connected</div>`
			source := strings.Replace(string(content), `<body class="page-app">`, `<body class="page-app">`+banner, 1)
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Cache-Control", "no-store")
			fmt.Fprint(w, source)
			return
		}
		if strings.HasPrefix(path, "/api/") {
			jsonReply(w, 404, map[string]any{"error": "API outside telemetry preview"})
			return
		}
		files.ServeHTTP(w, r)
	})
	log.Printf("Synthetic telemetry preview: http://%s/app.html#internet", *address)
	server := http.Server{Addr: *address, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.ListenAndServe())
}
