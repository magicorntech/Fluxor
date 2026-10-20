package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magicorntech/Fluxor/internal/auth"
	"github.com/magicorntech/Fluxor/internal/authz"
	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
	"github.com/magicorntech/Fluxor/internal/store/memory"
)

const (
	adminName   = "admin"
	adminPass   = "initial-password-1"
	adminPass2  = "a-brand-new-password"
	testSecret  = "0123456789abcdef0123456789abcdef"
	csrfValue   = "1"
	apiClusters = "/api/v1/clusters"
)

type testEnv struct {
	srv   *httptest.Server
	store *memory.Store
}

type testOpts struct {
	origins      []string
	cookieSecure bool
	// clock replaces the time of the auth service and the rate limiter.
	clock *testClock
	// loginPerMinute and refreshPerMinute set the rate limits; zero turns the limit off.
	loginPerMinute, refreshPerMinute int
	// trustedProxies are CIDR blocks or addresses whose X-Forwarded-For is believed.
	trustedProxies []string
	// auditFails makes every write to the audit log fail.
	auditFails bool
}

// testClock is a clock tests can move.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func newTestClock() *testClock { return &testClock{t: time.Now()} }

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// failingAudit is a store whose audit log cannot be written.
type failingAudit struct{ *memory.Store }

func (failingAudit) AddAudit(context.Context, domain.AuditEvent) error {
	return errors.New("audit log unavailable")
}

func newEnv(t *testing.T, o ...testOpts) *testEnv {
	t.Helper()
	var opt testOpts
	if len(o) > 0 {
		opt = o[0]
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st := memory.New()
	var now func() time.Time
	if opt.clock != nil {
		now = opt.clock.Now
	}
	svc, err := auth.NewService(auth.Options{
		Users: st, Sessions: st, Tokens: st, Secret: []byte(testSecret), Logger: log, Now: now,
		Params: auth.Params{Memory: 8, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Bootstrap(context.Background(), adminName, adminPass); err != nil {
		t.Fatal(err)
	}
	az := authz.New(st)
	if g, err := az.EnsureBootstrapAdmin(context.Background()); err != nil || !g.Created {
		t.Fatalf("the bootstrap user must become an administrator: %+v %v", g, err)
	}
	proxies, err := ParseTrustedProxies(opt.trustedProxies)
	if err != nil {
		t.Fatal(err)
	}
	// Most tests sign in many times from one address, so the limits are off unless a test asks for them.
	limit := func(n int) int {
		if n == 0 {
			return -1
		}
		return n
	}
	var apiStore store.Store = st
	if opt.auditFails {
		apiStore = failingAudit{st}
	}
	srv := httptest.NewServer(New(Options{
		Store: apiStore, Auth: svc, Authz: az, Logger: log,
		CORSOrigins: opt.origins, CookieSecure: opt.cookieSecure,
		TrustedProxies: proxies, Now: now,
		LoginPerMinute: limit(opt.loginPerMinute), RefreshPerMinute: limit(opt.refreshPerMinute),
	}))
	t.Cleanup(srv.Close)
	return &testEnv{srv: srv, store: st}
}

type req struct {
	token   string
	body    string
	cookie  *http.Cookie
	headers map[string]string
}

type result struct {
	*http.Response
	body map[string]any
	raw  string
}

func (e *testEnv) do(t *testing.T, method, path string, r req) result {
	t.Helper()
	hr, err := http.NewRequest(method, e.srv.URL+path, strings.NewReader(r.body))
	if err != nil {
		t.Fatal(err)
	}
	hr.Header.Set("Content-Type", "application/json")
	if r.token != "" {
		hr.Header.Set("Authorization", "Bearer "+r.token)
	}
	if r.cookie != nil {
		hr.AddCookie(r.cookie)
	}
	for k, v := range r.headers {
		hr.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(hr)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	return result{Response: resp, body: body, raw: string(raw)}
}

func (r result) errorCode() string {
	e, _ := r.body["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func (r result) refreshCookie() *http.Cookie {
	for _, c := range r.Cookies() {
		if c.Name == refreshCookie {
			return c
		}
	}
	return nil
}

func (r result) str(key string) string {
	s, _ := r.body[key].(string)
	return s
}

// login signs in as the bootstrap admin and returns the session response.
func (e *testEnv) login(t *testing.T, password string) result {
	t.Helper()
	return e.do(t, "POST", "/api/v1/auth/login", req{body: `{"username":"` + adminName + `","password":"` + password + `"}`})
}

// ready returns an access token of a fully usable session: it signs in and performs the
// mandatory first password change.
func (e *testEnv) ready(t *testing.T) (token string, cookie *http.Cookie) {
	t.Helper()
	res := e.login(t, adminPass)
	if res.StatusCode != 200 {
		t.Fatalf("login: %d %v", res.StatusCode, res.body)
	}
	res = e.do(t, "PUT", "/api/v1/auth/password", req{
		token: res.str("accessToken"),
		body:  `{"currentPassword":"` + adminPass + `","newPassword":"` + adminPass2 + `"}`,
	})
	if res.StatusCode != 200 {
		t.Fatalf("change password: %d %v", res.StatusCode, res.body)
	}
	return res.str("accessToken"), res.refreshCookie()
}

func TestHealthIsPublic(t *testing.T) {
	e := newEnv(t)
	res := e.do(t, "GET", "/healthz", req{})
	if res.StatusCode != 200 || res.str("status") != "ok" {
		t.Fatalf("unexpected response: %d %v", res.StatusCode, res.body)
	}
}

func TestEverythingElseRequiresAuthentication(t *testing.T) {
	e := newEnv(t)
	tests := []struct {
		name   string
		method string
		path   string
		token  string
	}{
		{"list clusters", "GET", apiClusters, ""},
		{"create cluster", "POST", apiClusters, ""},
		{"get cluster", "GET", apiClusters + "/x", ""},
		{"delete cluster", "DELETE", apiClusters + "/x", ""},
		{"me", "GET", "/api/v1/auth/me", ""},
		{"change password", "PUT", "/api/v1/auth/password", ""},
		{"unknown path", "GET", "/api/v1/nope", ""},
		{"garbage token", "GET", apiClusters, "not-a-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := e.do(t, tt.method, tt.path, req{token: tt.token})
			if res.StatusCode != 401 || res.errorCode() != "unauthenticated" {
				t.Fatalf("got %d %v, want 401 unauthenticated", res.StatusCode, res.body)
			}
			if res.Header.Get("WWW-Authenticate") != "Bearer" {
				t.Fatal("missing WWW-Authenticate header")
			}
		})
	}

	// A token scheme other than Bearer must not be accepted either.
	res := e.do(t, "GET", apiClusters, req{headers: map[string]string{"Authorization": "Basic YWRtaW46eA=="}})
	if res.StatusCode != 401 {
		t.Fatalf("basic auth accepted: %d", res.StatusCode)
	}
}

func TestLogin(t *testing.T) {
	e := newEnv(t)

	t.Run("wrong password and unknown user look the same", func(t *testing.T) {
		wrong := e.login(t, "definitely-wrong")
		unknown := e.do(t, "POST", "/api/v1/auth/login", req{body: `{"username":"ghost","password":"whatever-it-is"}`})
		for _, r := range []result{wrong, unknown} {
			if r.StatusCode != 401 || r.errorCode() != "invalid_credentials" {
				t.Fatalf("got %d %v", r.StatusCode, r.body)
			}
		}
		if wrong.body["error"].(map[string]any)["message"] != unknown.body["error"].(map[string]any)["message"] {
			t.Fatal("messages must not reveal whether the user exists")
		}
	})

	t.Run("bad requests", func(t *testing.T) {
		for _, body := range []string{`{}`, `{"username":"admin"}`, `{"password":"x"}`, `{`, `{"username":"a","password":"b","x":1}`} {
			if res := e.do(t, "POST", "/api/v1/auth/login", req{body: body}); res.StatusCode != 400 {
				t.Errorf("body %s: got %d %v, want 400", body, res.StatusCode, res.body)
			}
		}
	})

	t.Run("success", func(t *testing.T) {
		res := e.login(t, adminPass)
		if res.StatusCode != 200 {
			t.Fatalf("got %d %v", res.StatusCode, res.body)
		}
		if res.str("accessToken") == "" || res.str("expiresAt") == "" {
			t.Fatalf("missing token fields: %v", res.body)
		}
		user, _ := res.body["user"].(map[string]any)
		if user["username"] != adminName || user["mustChangePassword"] != true {
			t.Fatalf("unexpected user: %v", user)
		}
		for k := range user {
			if strings.Contains(strings.ToLower(k), "password") && k != "mustChangePassword" {
				t.Fatalf("user response leaks %q", k)
			}
		}
		if res.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("session responses must not be cached")
		}

		c := res.refreshCookie()
		if c == nil || c.Value == "" {
			t.Fatal("no refresh cookie")
		}
		if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/api/v1/auth" || c.Secure {
			t.Fatalf("unexpected cookie attributes: %+v", c)
		}
		if strings.Contains(res.Header.Get("Set-Cookie"), res.str("accessToken")) {
			t.Fatal("the access token must not be in a cookie")
		}
	})

	t.Run("username is case-insensitive", func(t *testing.T) {
		res := e.do(t, "POST", "/api/v1/auth/login", req{body: `{"username":"  ADMIN ","password":"` + adminPass + `"}`})
		if res.StatusCode != 200 {
			t.Fatalf("got %d %v", res.StatusCode, res.body)
		}
	})
}

func TestSecureCookieOption(t *testing.T) {
	e := newEnv(t, testOpts{cookieSecure: true})
	c := e.login(t, adminPass).refreshCookie()
	if c == nil || !c.Secure {
		t.Fatalf("cookie must be Secure when configured: %+v", c)
	}
}

func TestMustChangePassword(t *testing.T) {
	e := newEnv(t)
	token := e.login(t, adminPass).str("accessToken")

	res := e.do(t, "GET", apiClusters, req{token: token})
	if res.StatusCode != 403 || res.errorCode() != "password_change_required" {
		t.Fatalf("API must be locked until the password is changed: %d %v", res.StatusCode, res.body)
	}
	if res := e.do(t, "POST", apiClusters, req{token: token, body: `{"name":"a","apiServer":"https://x"}`}); res.StatusCode != 403 {
		t.Fatalf("write must be locked too: %d", res.StatusCode)
	}

	me := e.do(t, "GET", "/api/v1/auth/me", req{token: token})
	if me.StatusCode != 200 || me.body["mustChangePassword"] != true {
		t.Fatalf("me must work and report the flag: %d %v", me.StatusCode, me.body)
	}

	put := func(body string) result {
		return e.do(t, "PUT", "/api/v1/auth/password", req{token: token, body: body})
	}

	if r := put(`{"currentPassword":"wrong-current-pw","newPassword":"` + adminPass2 + `"}`); r.StatusCode != 400 || r.errorCode() != "invalid_current_password" {
		t.Fatalf("wrong current password: %d %v (must be 400, not 401, so the dashboard keeps the session)", r.StatusCode, r.body)
	}
	if r := put(`{"currentPassword":"` + adminPass + `","newPassword":"short"}`); r.StatusCode != 400 || r.errorCode() != "invalid_argument" {
		t.Fatalf("weak password: %d %v", r.StatusCode, r.body)
	}
	if r := put(`{"currentPassword":"` + adminPass + `","newPassword":"` + adminPass + `"}`); r.StatusCode != 400 {
		t.Fatalf("unchanged password: %d %v", r.StatusCode, r.body)
	}

	changed := put(`{"currentPassword":"` + adminPass + `","newPassword":"` + adminPass2 + `"}`)
	if changed.StatusCode != 200 {
		t.Fatalf("change: %d %v", changed.StatusCode, changed.body)
	}
	if user, _ := changed.body["user"].(map[string]any); user["mustChangePassword"] != false {
		t.Fatalf("flag not cleared: %v", user)
	}
	if res := e.do(t, "GET", apiClusters, req{token: changed.str("accessToken")}); res.StatusCode != 200 {
		t.Fatalf("API must be usable after the change: %d %v", res.StatusCode, res.body)
	}
	if res := e.login(t, adminPass); res.StatusCode != 401 {
		t.Fatalf("old password still works: %d", res.StatusCode)
	}
	if res := e.login(t, adminPass2); res.StatusCode != 200 {
		t.Fatalf("new password rejected: %d", res.StatusCode)
	}
}

func TestRefreshAndLogout(t *testing.T) {
	e := newEnv(t)
	token, cookie := e.ready(t)
	csrf := map[string]string{csrfHeader: csrfValue}

	t.Run("refresh needs the CSRF header", func(t *testing.T) {
		res := e.do(t, "POST", "/api/v1/auth/refresh", req{cookie: cookie})
		if res.StatusCode != 403 {
			t.Fatalf("got %d %v", res.StatusCode, res.body)
		}
	})
	t.Run("refresh needs the cookie", func(t *testing.T) {
		res := e.do(t, "POST", "/api/v1/auth/refresh", req{headers: csrf})
		if res.StatusCode != 401 {
			t.Fatalf("got %d %v", res.StatusCode, res.body)
		}
	})
	t.Run("refresh rotates the cookie and the old one stops working", func(t *testing.T) {
		res := e.do(t, "POST", "/api/v1/auth/refresh", req{cookie: cookie, headers: csrf})
		if res.StatusCode != 200 || res.str("accessToken") == "" {
			t.Fatalf("got %d %v", res.StatusCode, res.body)
		}
		next := res.refreshCookie()
		if next == nil || next.Value == "" || next.Value == cookie.Value {
			t.Fatalf("cookie was not rotated: %+v", next)
		}
		if res := e.do(t, "GET", apiClusters, req{token: res.str("accessToken")}); res.StatusCode != 200 {
			t.Fatalf("new access token rejected: %d", res.StatusCode)
		}

		reuse := e.do(t, "POST", "/api/v1/auth/refresh", req{cookie: cookie, headers: csrf})
		if reuse.StatusCode != 401 {
			t.Fatalf("a used refresh cookie must be rejected: %d", reuse.StatusCode)
		}
		if c := reuse.refreshCookie(); c == nil || c.MaxAge >= 0 {
			t.Fatalf("a rejected refresh must clear the cookie: %+v", c)
		}
		cookie = next
	})

	t.Run("logout needs the CSRF header", func(t *testing.T) {
		if res := e.do(t, "POST", "/api/v1/auth/logout", req{cookie: cookie}); res.StatusCode != 403 {
			t.Fatalf("got %d", res.StatusCode)
		}
	})
	t.Run("logout ends the session", func(t *testing.T) {
		res := e.do(t, "POST", "/api/v1/auth/logout", req{cookie: cookie, headers: csrf})
		if res.StatusCode != 204 {
			t.Fatalf("got %d", res.StatusCode)
		}
		if c := res.refreshCookie(); c == nil || c.MaxAge >= 0 {
			t.Fatalf("logout must clear the cookie: %+v", c)
		}
		if res := e.do(t, "POST", "/api/v1/auth/refresh", req{cookie: cookie, headers: csrf}); res.StatusCode != 401 {
			t.Fatalf("refresh worked after logout: %d", res.StatusCode)
		}
	})

	// Logging out does not revoke already-issued access tokens; they simply expire (15 minutes).
	_ = token
}

func TestDisabledUserLosesAccessImmediately(t *testing.T) {
	e := newEnv(t)
	token, _ := e.ready(t)
	if res := e.do(t, "GET", apiClusters, req{token: token}); res.StatusCode != 200 {
		t.Fatalf("precondition: %d", res.StatusCode)
	}

	u, err := e.store.GetUserByUsername(context.Background(), adminName)
	if err != nil {
		t.Fatal(err)
	}
	u.Disabled = true
	if err := e.store.UpdateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	if res := e.do(t, "GET", apiClusters, req{token: token}); res.StatusCode != 401 {
		t.Fatalf("disabled user still has access: %d", res.StatusCode)
	}
}

func TestClusterLifecycle(t *testing.T) {
	e := newEnv(t)
	token, _ := e.ready(t)
	authed := func(method, path, body string) result { return e.do(t, method, path, req{token: token, body: body}) }

	res := authed("POST", apiClusters, `{"name":"prod-eu","apiServer":"https://10.0.0.1:6443","labels":{"env":"prod"}}`)
	if res.StatusCode != 201 {
		t.Fatalf("create: %d %v", res.StatusCode, res.body)
	}
	id := res.str("id")
	if id == "" || res.str("status") != "Unknown" {
		t.Fatalf("create returned %v", res.body)
	}

	res = authed("GET", apiClusters, "")
	items, _ := res.body["items"].([]any)
	if res.StatusCode != 200 || len(items) != 1 {
		t.Fatalf("list: %d %v", res.StatusCode, res.body)
	}

	res = authed("GET", apiClusters+"/"+id, "")
	if res.StatusCode != 200 || res.str("name") != "prod-eu" {
		t.Fatalf("get: %d %v", res.StatusCode, res.body)
	}

	if res = authed("DELETE", apiClusters+"/"+id, ""); res.StatusCode != 204 {
		t.Fatalf("delete: %d", res.StatusCode)
	}
	if res = authed("GET", apiClusters+"/"+id, ""); res.StatusCode != 404 {
		t.Fatalf("get after delete: %d %v", res.StatusCode, res.body)
	}
}

func TestCreateClusterErrors(t *testing.T) {
	e := newEnv(t)
	token, _ := e.ready(t)
	post := func(body string) result { return e.do(t, "POST", apiClusters, req{token: token, body: body}) }

	tests := []struct {
		name string
		body string
		want int
	}{
		{"invalid name", `{"name":"Bad Name","apiServer":"https://x"}`, 400},
		{"invalid url", `{"name":"a","apiServer":"nope"}`, 400},
		{"unknown field", `{"name":"a","apiServer":"https://x","extra":1}`, 400},
		{"malformed json", `{`, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := post(tt.body)
			if res.StatusCode != tt.want {
				t.Fatalf("got %d %v, want %d", res.StatusCode, res.body, tt.want)
			}
			if _, ok := res.body["error"].(map[string]any); !ok {
				t.Fatalf("missing error envelope: %v", res.body)
			}
		})
	}

	if res := post(`{"name":"dup","apiServer":"https://x"}`); res.StatusCode != 201 {
		t.Fatalf("first create: %d", res.StatusCode)
	}
	if res := post(`{"name":"dup","apiServer":"https://y"}`); res.StatusCode != 409 {
		t.Fatalf("duplicate create: %d", res.StatusCode)
	}
}

func TestCORS(t *testing.T) {
	e := newEnv(t, testOpts{origins: []string{"http://localhost:5173"}})

	pre := e.do(t, "OPTIONS", apiClusters, req{headers: map[string]string{"Origin": "http://localhost:5173"}})
	if pre.StatusCode != 204 || pre.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("allowed origin: %d %v", pre.StatusCode, pre.Header)
	}
	if pre.Header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("the refresh cookie needs Access-Control-Allow-Credentials")
	}
	if !strings.Contains(pre.Header.Get("Access-Control-Allow-Headers"), csrfHeader) {
		t.Fatalf("the CSRF header must be allowed: %v", pre.Header.Get("Access-Control-Allow-Headers"))
	}

	evil := e.do(t, "GET", "/healthz", req{headers: map[string]string{"Origin": "http://evil.example"}})
	if evil.Header.Get("Access-Control-Allow-Origin") != "" || evil.Header.Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("CORS headers set for a disallowed origin")
	}
}
