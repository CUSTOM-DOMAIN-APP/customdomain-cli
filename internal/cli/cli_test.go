package cli

// cli_test.go drives the CLI against a fake API: login's device flow (pending, slow_down, then tokens) and where the
// tokens are kept, the refresh of an expired access token, an API key taking precedence, the domain list and doctor,
// logout's revoke, and the message when the server has device sign-in switched off.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeAPI struct {
	mu        sync.Mutex
	polls     int
	refreshed int
	revoked   string
	auth      []string
	deviceOff bool
	wantScope string
	apps      []map[string]any
	keyBodies []string
}

func (f *fakeAPI) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		writeJSON := func(status int, v any) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(v)
		}
		if a := r.Header.Get("Authorization"); a != "" {
			f.auth = append(f.auth, a)
		}
		switch r.URL.Path {
		case "/oauth/agent/register":
			writeJSON(201, map[string]any{"client_id": "cli_1"})
		case "/oauth/agent/device_authorization":
			if f.deviceOff {
				http.NotFound(w, r)
				return
			}
			_ = r.ParseForm()
			if r.PostForm.Get("client_id") != "cli_1" || r.PostForm.Get("scope") != f.scope() {
				t.Errorf("device_authorization form = %v", r.PostForm)
			}
			writeJSON(200, map[string]any{"device_code": "dev_1", "user_code": "BCDF-GHJK", "verification_uri": "https://app.example/device",
				"verification_uri_complete": "https://app.example/device?user_code=BCDF-GHJK", "expires_in": 600, "interval": 5})
		case "/oauth/agent/token":
			_ = r.ParseForm()
			switch r.PostForm.Get("grant_type") {
			case "urn:ietf:params:oauth:grant-type:device_code":
				f.polls++
				switch f.polls {
				case 1:
					writeJSON(400, map[string]any{"error": "authorization_pending"})
				case 2:
					writeJSON(400, map[string]any{"error": "slow_down"})
				default:
					writeJSON(200, map[string]any{"access_token": "acc_1", "refresh_token": "ref_1", "expires_in": 600, "scope": f.scope()})
				}
			case "refresh_token":
				f.refreshed++
				writeJSON(200, map[string]any{"access_token": "acc_2", "refresh_token": "ref_2", "expires_in": 600})
			}
		case "/oauth/agent/revoke":
			_ = r.ParseForm()
			f.revoked = r.PostForm.Get("token")
			w.WriteHeader(200)
		case "/v1/connections":
			writeJSON(200, map[string]any{"connections": []map[string]any{{"domain": "b.example.com", "status": "live"}, {"domain": "a.example.com", "status": "pending"}}, "count": 2})
		case "/v1/domains:check":
			writeJSON(200, map[string]any{"provider": "Cloudflare", "setup_type": "oauth", "supports_automatic": true,
				"record_conflicts": []map[string]any{{"kind": "value-mismatch", "host": "shop.example.com", "type": "A", "existing": "1.2.3.4"}}})
		case "/v1/applications":
			writeJSON(200, map[string]any{"applications": f.apps})
		case "/v1/applications/app_test/keys":
			b, _ := io.ReadAll(r.Body)
			f.keyBodies = append(f.keyBodies, string(b))
			writeJSON(201, map[string]any{"id": "key_9", "prefix": "cd_test_ab", "key": "cd_test_SECRETVALUE"})
		case "/v1/me":
			writeJSON(200, map[string]any{"tenant_id": "ten_1", "role": "admin", "credential": "member", "member": map[string]any{"email": "ana@example.com"}})
		default:
			http.NotFound(w, r)
		}
	})
}

func (f *fakeAPI) scope() string {
	if f.wantScope != "" {
		return f.wantScope
	}
	return DefaultScope
}

type rig struct {
	app   *App
	api   *fakeAPI
	out   *bytes.Buffer
	now   time.Time
	slept []time.Duration
}

func newRig(t *testing.T) *rig {
	t.Helper()
	f := &fakeAPI{}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	r := &rig{api: f, out: &bytes.Buffer{}, now: time.Unix(1_700_000_000, 0)}
	r.app = &App{
		API: srv.URL, CredPath: filepath.Join(t.TempDir(), "customdomain", "credentials.json"),
		HTTP: srv.Client(), Out: r.out, Err: r.out,
		Now: func() time.Time { return r.now },
		Sleep: func(_ context.Context, d time.Duration) error {
			r.slept = append(r.slept, d)
			r.now = r.now.Add(d)
			return nil
		},
	}
	return r
}

func TestLoginRunsTheDeviceFlowAndKeepsTheTokensPrivate(t *testing.T) {
	r := newRig(t)
	if code := r.app.Run(context.Background(), []string{"login"}); code != 0 {
		t.Fatalf("login = %d: %s", code, r.out)
	}
	if !strings.Contains(r.out.String(), "https://app.example/device?user_code=BCDF-GHJK") || !strings.Contains(r.out.String(), "BCDF-GHJK") {
		t.Fatalf("login did not show where to approve: %s", r.out)
	}
	if got := r.slept; len(got) != 3 || got[0] != 5*time.Second || got[2] != 10*time.Second {
		t.Fatalf("poll waits = %v, want 5s, 5s, then 10s after slow_down", got)
	}
	fi, err := os.Stat(r.app.CredPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("credentials mode = %v, want 0600", fi.Mode().Perm())
	}
	c, _ := r.app.load()
	if c.AccessToken != "acc_1" || c.RefreshToken != "ref_1" || c.ClientID != "cli_1" {
		t.Fatalf("saved = %+v", c)
	}
}

func TestAnExpiredAccessTokenIsRefreshedAndSaved(t *testing.T) {
	r := newRig(t)
	if err := r.app.save(&Credentials{API: r.app.API, ClientID: "cli_1", AccessToken: "acc_old", RefreshToken: "ref_old", ExpiresAt: r.now.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if code := r.app.Run(context.Background(), []string{"domains"}); code != 0 {
		t.Fatalf("domains = %d: %s", code, r.out)
	}
	if r.api.refreshed != 1 || r.api.auth[len(r.api.auth)-1] != "Bearer acc_2" {
		t.Fatalf("refreshed=%d auth=%v", r.api.refreshed, r.api.auth)
	}
	if c, _ := r.app.load(); c.RefreshToken != "ref_2" {
		t.Fatalf("the rotated refresh token was not saved: %+v", c)
	}
	out := r.out.String()
	if strings.Index(out, "a.example.com") > strings.Index(out, "b.example.com") || !strings.Contains(out, "STATUS") {
		t.Fatalf("domains output = %s", out)
	}
}

func TestAnAPIKeyIsUsedInsteadOfTheSignIn(t *testing.T) {
	r := newRig(t)
	r.app.APIKey = "cd_live_x"
	if code := r.app.Run(context.Background(), []string{"whoami"}); code != 0 {
		t.Fatalf("whoami = %d: %s", code, r.out)
	}
	if r.api.auth[0] != "Bearer cd_live_x" || !strings.Contains(r.out.String(), "ana@example.com") || !strings.Contains(r.out.String(), "admin") {
		t.Fatalf("whoami with a key: auth=%v out=%s", r.api.auth, r.out)
	}
}

func TestDoctorSaysWhereTheDNSIsAndWhatIsInTheWay(t *testing.T) {
	r := newRig(t)
	r.app.APIKey = "cd_live_x"
	if code := r.app.Run(context.Background(), []string{"doctor", "shop.example.com"}); code != 0 {
		t.Fatalf("doctor = %d: %s", code, r.out)
	}
	out := r.out.String()
	for _, want := range []string{"Cloudflare", "oauth (automatic)", "In the way:  value-mismatch on shop.example.com (A 1.2.3.4)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output lacks %q:\n%s", want, out)
		}
	}
}

func TestLogoutRevokesAndForgets(t *testing.T) {
	r := newRig(t)
	_ = r.app.save(&Credentials{ClientID: "cli_1", AccessToken: "a", RefreshToken: "ref_9", ExpiresAt: r.now.Add(time.Hour)})
	if code := r.app.Run(context.Background(), []string{"logout"}); code != 0 {
		t.Fatalf("logout = %d: %s", code, r.out)
	}
	if r.api.revoked != "ref_9" {
		t.Fatalf("revoked %q, want the refresh token", r.api.revoked)
	}
	if _, err := os.Stat(r.app.CredPath); !os.IsNotExist(err) {
		t.Fatal("the credentials file is still there")
	}
}

func TestLoginExplainsWhenDeviceSignInIsOff(t *testing.T) {
	r := newRig(t)
	r.api.deviceOff = true
	if code := r.app.Run(context.Background(), []string{"login"}); code != 1 || !strings.Contains(r.out.String(), "CUSTOMDOMAIN_API_KEY") {
		t.Fatalf("login with device sign-in off = %d: %s", code, r.out)
	}
}

func TestCommandsNeedASignIn(t *testing.T) {
	r := newRig(t)
	if code := r.app.Run(context.Background(), []string{"domains"}); code != 1 || !strings.Contains(r.out.String(), "customdomain login") {
		t.Fatalf("domains without a sign in = %d: %s", code, r.out)
	}
	if code := r.app.Run(context.Background(), nil); code != 2 {
		t.Fatalf("no command = %d, want 2", code)
	}
}
