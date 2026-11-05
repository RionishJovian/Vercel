package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/ai"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/hub"
	"mega-magic/internal/model"
)

// The prompt's page names must be the ones people actually see in the console.
func TestMagicDudePagesMatchTheApp(t *testing.T) {
	var labels strings.Builder
	for _, f := range []string{"dict.json", "extra.ts", "broadband.ts", "hubConnection.ts", "tables.ts"} {
		b, err := os.ReadFile("../../app/src/i18n/" + f)
		if err != nil {
			t.Fatal(err)
		}
		labels.Write(b)
	}
	source := func(path string) string {
		b, err := os.ReadFile("../../app/src/" + path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	more, shell, jellyfin := source("pages/More.tsx"), source("components/Shell.tsx"), source("pages/Jellyfin.tsx")
	check := func(pages []magicDudePage, shownIn func(key string) bool) {
		for _, p := range pages {
			pattern := `"` + regexp.QuoteMeta(p.Key) + `":\s*"` + regexp.QuoteMeta(p.Label) + `"`
			if !regexp.MustCompile(pattern).MatchString(labels.String()) {
				t.Errorf("%s is no longer labelled %q", p.Key, p.Label)
			}
			if !shownIn(p.Key) {
				t.Errorf("%s (%q) is no longer in the app's navigation", p.Key, p.Label)
			}
		}
	}
	inMore := func(key string) bool { return strings.Contains(more, `"`+key+`"`) }
	check(magicDudeMore, inMore)
	check(magicDudeStaff, inMore)
	check(magicDudeDeveloper, inMore)
	check(magicDudeTabs, func(key string) bool { return strings.Contains(shell, `"`+strings.TrimPrefix(key, "tab.")+`"`) })
	check(magicDudeJellyfin, func(key string) bool { return strings.Contains(jellyfin, `"`+strings.TrimPrefix(key, "j.ws.")+`"`) })
}

func TestMagicDudeLengthsCountCharacters(t *testing.T) {
	burmese := strings.Repeat("မ", 1400) // 4,200 bytes
	if err := validateMagicDudeRequest(&magicDudeRequest{Question: burmese}); err != nil {
		t.Fatalf("a 1,400-character Burmese question was refused: %v", err)
	}
	if err := validateMagicDudeRequest(&magicDudeRequest{Question: strings.Repeat("မ", magicDudeMaxQuestion+1)}); err == nil {
		t.Fatal("an over-long question was accepted")
	}
	req := magicDudeRequest{Question: "ok", History: []ai.ChatMessage{{Role: "assistant", Content: strings.Repeat("မ", magicDudeMaxTurn+50)}}}
	if err := validateMagicDudeRequest(&req); err != nil {
		t.Fatalf("a long earlier answer blocked the next question: %v", err)
	}
	if got := len([]rune(req.History[0].Content)); got != magicDudeMaxTurn {
		t.Fatalf("long answer kept %d characters", got)
	}
	if err := validateMagicDudeRequest(&magicDudeRequest{Question: "ok", History: []ai.ChatMessage{{Role: "user", Content: strings.Repeat("a", magicDudeMaxTurn+1)}}}); err == nil {
		t.Fatal("an over-long user turn was accepted")
	}
}

func TestMagicDudeSignalText(t *testing.T) {
	now := time.Now()
	recent, old := now.Add(-time.Minute), now.Add(-time.Hour)
	for _, tc := range []struct {
		peer hub.Peer
		want string
	}{
		{hub.Peer{State: "active", LastHandshakeAt: &recent}, "within the last 5 minutes"},
		{hub.Peer{State: "active", LastHandshakeAt: &old}, "no Magic Hub tunnel contact"},
		{hub.Peer{State: "active"}, "no Magic Hub tunnel contact"},
		{hub.Peer{State: "removed", LastHandshakeAt: &recent}, "not connected"},
	} {
		if got := magicDudeSignalText(tc.peer, now); !strings.Contains(got, tc.want) {
			t.Errorf("%+v: got %q", tc.peer, got)
		}
	}
}

type magicDudeFakeClient struct {
	calls  int
	prompt string
	err    error
}

func (f *magicDudeFakeClient) Chat(_ context.Context, prompt string, _ []ai.ChatMessage, _ string) (*ai.ChatReply, error) {
	f.calls++
	f.prompt = prompt
	if f.err != nil {
		return nil, f.err
	}
	return &ai.ChatReply{Answer: "အဆင်ပြေပါတယ်။", Model: "fake", TotalTokens: 5}, nil
}

func TestDBMagicDudeChat(t *testing.T) {
	db := harness_testdb(t)
	business, other := insertBusiness(db), insertBusiness(db)
	gateway := insertGatewayNoSite(db, business)
	foreign := insertGatewayNoSite(db, other)
	if _, err := db.Exec(`UPDATE devices SET hardware_name='Foreign router' WHERE device_id=$1`, foreign); err != nil {
		t.Fatal(err)
	}
	// The stored flag says offline; the Hub's live state must win.
	if _, err := db.Exec(`UPDATE devices SET is_online=false WHERE device_id=$1`, gateway); err != nil {
		t.Fatal(err)
	}
	var userID string
	if err := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Fixture','fixture','OWNER')RETURNING user_id`, business, "md-"+randHex(4)+"@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	newHandler := func(c *magicDudeFakeClient) *MagicDudeHandler {
		return NewMagicDudeHandler(db, database.NewDeviceRepository(db), database.NewIncidentRepository(db), database.NewAIUsageRepository(db), c)
	}
	ask := func(h *MagicDudeHandler, role, question string) *httptest.ResponseRecorder {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(auth.ContextKeyBusinessID, business)
			c.Set(auth.ContextKeyUserID, userID)
			c.Set(auth.ContextKeyUserRole, role)
		})
		r.POST("/chat", h.Chat)
		body, _ := json.Marshal(magicDudeRequest{Question: question})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/chat", bytes.NewReader(body)))
		return w
	}
	owner := string(model.RoleOwner)

	t.Run("status comes from the Hub and stays inside the account", func(t *testing.T) {
		client := &magicDudeFakeClient{}
		h := newHandler(client)
		now := time.Now()
		var seen []string
		h.ConfigureHubSignal(hubInspectorFunc(func(_ context.Context, id hub.Identity) (hub.Peer, error) {
			seen = append(seen, id.RouterID)
			return hub.Peer{TenantID: id.TenantID, RouterID: id.RouterID, State: "active", LastHandshakeAt: &now}, nil
		}))
		w := ask(h, owner, "How are my devices doing?")
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body)
		}
		if !strings.Contains(client.prompt, "tunnel contact within the last 5 minutes") || strings.Contains(client.prompt, ", offline") || strings.Contains(client.prompt, "Foreign router") {
			t.Fatalf("snapshot: %s", client.prompt)
		}
		if len(seen) != 1 || seen[0] != gateway {
			t.Fatalf("inspected %v", seen)
		}
	})
	t.Run("an unreadable Hub is reported as unknown", func(t *testing.T) {
		client := &magicDudeFakeClient{}
		h := newHandler(client)
		h.ConfigureHubSignal(hubInspectorFunc(func(context.Context, hub.Identity) (hub.Peer, error) { return hub.Peer{}, errors.New("down") }))
		if w := ask(h, owner, "How are my devices doing?"); w.Code != http.StatusOK || !strings.Contains(client.prompt, "connection status unknown") {
			t.Fatalf("status %d, prompt %s", w.Code, client.prompt)
		}
	})
	t.Run("stored directions use no provider, limit or usage", func(t *testing.T) {
		client := &magicDudeFakeClient{err: errors.New("provider down")}
		h := newHandler(client)
		h.ConfigureDailyLimit(func(context.Context, string) (bool, error) {
			t.Error("stored directions used the limit")
			return false, nil
		})
		var before, after int
		_ = db.QueryRow(`SELECT count(*) FROM ai_usage_events WHERE business_id=$1`, business).Scan(&before)
		w := ask(h, owner, "Where do I connect the gateway?")
		_ = db.QueryRow(`SELECT count(*) FROM ai_usage_events WHERE business_id=$1`, business).Scan(&after)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Magic Hub") || client.calls != 0 || after != before {
			t.Fatalf("status %d, calls %d, usage %d->%d: %s", w.Code, client.calls, before, after, w.Body)
		}
	})
	t.Run("stored directions still need access", func(t *testing.T) {
		client := &magicDudeFakeClient{}
		if w := ask(newHandler(client), string(model.RoleViewer), "Where do I connect the gateway?"); w.Code != http.StatusForbidden {
			t.Fatalf("status %d", w.Code)
		}
	})
	t.Run("the daily limit stops the provider call", func(t *testing.T) {
		client := &magicDudeFakeClient{}
		h := newHandler(client)
		h.ConfigureDailyLimit(func(_ context.Context, id string) (bool, error) {
			if id != business {
				t.Errorf("limit checked for %s", id)
			}
			return false, nil
		})
		w := ask(h, owner, "How are my devices doing?")
		if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "MAGIC_DUDE_DAILY_LIMIT") || client.calls != 0 {
			t.Fatalf("status %d, calls %d: %s", w.Code, client.calls, w.Body)
		}
	})
	t.Run("a broken limit check does not stop the chat", func(t *testing.T) {
		client := &magicDudeFakeClient{}
		h := newHandler(client)
		h.ConfigureDailyLimit(func(context.Context, string) (bool, error) { return false, errors.New("redis down") })
		if w := ask(h, owner, "How are my devices doing?"); w.Code != http.StatusOK || client.calls != 1 {
			t.Fatalf("status %d, calls %d", w.Code, client.calls)
		}
	})
	t.Run("a provider failure is logged without the question", func(t *testing.T) {
		var logged bytes.Buffer
		log.SetOutput(&logged)
		defer log.SetOutput(os.Stderr)
		w := ask(newHandler(&magicDudeFakeClient{err: errors.New("ai: Gemini request timed out")}), owner, "SECRET-QUESTION")
		if w.Code != http.StatusBadGateway || !strings.Contains(logged.String(), "timed out") || strings.Contains(logged.String(), "SECRET-QUESTION") {
			t.Fatalf("status %d, log %q", w.Code, logged.String())
		}
	})
}
