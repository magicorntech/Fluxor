package httpapi

import (
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	clk := newTestClock()
	l := newRateLimiter(6, clk.Now) // burst of 6, then one every 10 seconds

	for i := 0; i < 6; i++ {
		if ok, _ := l.allow("a"); !ok {
			t.Fatalf("attempt %d of the burst was refused", i+1)
		}
	}
	ok, retry := l.allow("a")
	if ok {
		t.Fatal("the 7th attempt must be refused")
	}
	if retry < 9*time.Second || retry > 11*time.Second {
		t.Fatalf("retry-after should be about 10 s, got %s", retry)
	}

	if ok, _ := l.allow("b"); !ok {
		t.Fatal("another key has its own bucket")
	}

	clk.advance(10 * time.Second)
	if ok, _ := l.allow("a"); !ok {
		t.Fatal("one token has refilled")
	}
	if ok, _ := l.allow("a"); ok {
		t.Fatal("only one")
	}

	clk.advance(time.Hour)
	for i := 0; i < 6; i++ {
		if ok, _ := l.allow("a"); !ok {
			t.Fatalf("after a long pause the full burst is back (attempt %d)", i+1)
		}
	}
	if ok, _ := l.allow("a"); ok {
		t.Fatal("but never more than the burst")
	}
}

func TestRateLimiterOffAndPruning(t *testing.T) {
	var off *rateLimiter
	for i := 0; i < 1000; i++ {
		if ok, _ := off.allow("x"); !ok {
			t.Fatal("a nil limiter allows everything")
		}
	}
	if newRateLimiter(0, nil) != nil || newRateLimiter(-5, nil) != nil {
		t.Fatal("a non-positive rate means no limit")
	}

	clk := newTestClock()
	l := newRateLimiter(60, clk.Now)
	for i := 0; i < 500; i++ {
		l.allow(fmt.Sprintf("addr-%d", i))
	}
	if len(l.buckets) != 500 {
		t.Fatalf("buckets = %d", len(l.buckets))
	}
	clk.advance(30 * time.Minute)
	l.allow("fresh")
	if len(l.buckets) != 1 {
		t.Fatalf("idle keys must be forgotten so that distinct addresses cannot grow the map without bound: %d", len(l.buckets))
	}
}

func TestLoginIsRateLimitedPerAddress(t *testing.T) {
	clk := newTestClock()
	e := newEnv(t, testOpts{loginPerMinute: 4, clock: clk})
	login := func(headers map[string]string) result {
		return e.do(t, "POST", "/api/v1/auth/login", req{body: `{"username":"admin","password":"wrong-password-1"}`, headers: headers})
	}

	for i := 0; i < 4; i++ {
		if res := login(nil); res.StatusCode != 401 {
			t.Fatalf("attempt %d: %d", i+1, res.StatusCode)
		}
	}
	res := login(nil)
	if res.StatusCode != 429 || res.errorCode() != "rate_limited" {
		t.Fatalf("got %d %v", res.StatusCode, res.body)
	}
	if ra := res.Header.Get("Retry-After"); ra == "" || ra == "0" {
		t.Fatalf("Retry-After must tell when to try again, got %q", ra)
	}
	// The right password does not get past the limit either; the limit comes first.
	if res := e.login(t, adminPass); res.StatusCode != 429 {
		t.Fatalf("a correct login must be limited too: %d", res.StatusCode)
	}

	t.Run("a spoofed X-Forwarded-For does not get around it", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			if res := login(map[string]string{"X-Forwarded-For": fmt.Sprintf("10.9.9.%d", i)}); res.StatusCode != 429 {
				t.Fatalf("rotating the header must not help: %d", res.StatusCode)
			}
		}
	})

	clk.advance(time.Minute)
	if res := e.login(t, adminPass); res.StatusCode != 200 {
		t.Fatalf("the limit must lift with time: %d", res.StatusCode)
	}
}

func TestRateLimitBehindATrustedProxy(t *testing.T) {
	e := newEnv(t, testOpts{loginPerMinute: 2, clock: newTestClock(), trustedProxies: []string{"127.0.0.0/8"}})
	login := func(forwardedFor string) result {
		return e.do(t, "POST", "/api/v1/auth/login", req{
			body:    `{"username":"admin","password":"wrong-password-1"}`,
			headers: map[string]string{"X-Forwarded-For": forwardedFor},
		})
	}
	for i := 0; i < 2; i++ {
		login("203.0.113.7")
	}
	if res := login("203.0.113.7"); res.StatusCode != 429 {
		t.Fatalf("the client behind the proxy is limited: %d", res.StatusCode)
	}
	// A different client behind the same proxy has its own allowance: one noisy client must not lock out
	// everybody who shares the proxy's address.
	if res := login("203.0.113.8"); res.StatusCode != 401 {
		t.Fatalf("another client must not be affected: %d", res.StatusCode)
	}
	// What the client puts to the left of the proxy's own entry is ignored.
	if res := login("198.51.100.1, 203.0.113.7"); res.StatusCode != 429 {
		t.Fatalf("a forged leftmost entry must not give a fresh allowance: %d", res.StatusCode)
	}
}

func TestRefreshIsRateLimited(t *testing.T) {
	e := newEnv(t, testOpts{refreshPerMinute: 3, clock: newTestClock()})
	do := func() result {
		return e.do(t, "POST", "/api/v1/auth/refresh", req{headers: map[string]string{csrfHeader: csrfValue}})
	}
	for i := 0; i < 3; i++ {
		if res := do(); res.StatusCode != 401 {
			t.Fatalf("attempt %d: %d", i+1, res.StatusCode)
		}
	}
	if res := do(); res.StatusCode != 429 {
		t.Fatalf("got %d", res.StatusCode)
	}
}

func TestAuditRecordsTheRealClientAddress(t *testing.T) {
	trusted := newEnv(t, testOpts{trustedProxies: []string{"127.0.0.1"}})
	admin := trusted.admin(t)
	trusted.do(t, "POST", apiClusters, req{token: admin, body: `{"name":"alpha","apiServer":"https://a"}`, headers: map[string]string{"X-Forwarded-For": "203.0.113.50"}})
	events, _ := trusted.auditEvents(t, admin, "action=cluster.create&limit=1")
	if str(events[0], "ip") != "203.0.113.50" {
		t.Fatalf("behind a trusted proxy the forwarded address is recorded: %v", events[0])
	}

	direct := newEnv(t)
	admin = direct.admin(t)
	direct.do(t, "POST", apiClusters, req{token: admin, body: `{"name":"alpha","apiServer":"https://a"}`, headers: map[string]string{"X-Forwarded-For": "203.0.113.50"}})
	events, _ = direct.auditEvents(t, admin, "action=cluster.create&limit=1")
	if str(events[0], "ip") != "127.0.0.1" {
		t.Fatalf("a header from an untrusted peer must be ignored: %v", events[0])
	}
}

func TestRefreshTokenReuseIsDetectedAndAudited(t *testing.T) {
	clk := newTestClock()
	e := newEnv(t, testOpts{clock: clk})
	admin, cookie1 := e.ready(t)
	csrf := map[string]string{csrfHeader: csrfValue}

	second := e.do(t, "POST", "/api/v1/auth/refresh", req{cookie: cookie1, headers: csrf})
	if second.StatusCode != 200 {
		t.Fatalf("refresh: %d", second.StatusCode)
	}
	cookie2 := second.refreshCookie()

	// A moment later the old cookie shows up again: most likely two tabs, so only refused.
	clk.advance(2 * time.Second)
	if res := e.do(t, "POST", "/api/v1/auth/refresh", req{cookie: cookie1, headers: csrf}); res.StatusCode != 401 {
		t.Fatalf("got %d", res.StatusCode)
	}
	if res := e.do(t, "POST", "/api/v1/auth/refresh", req{cookie: cookie2, headers: csrf}); res.StatusCode != 200 {
		t.Fatalf("a quick repeat must not revoke the family: %d", res.StatusCode)
	}
	events, _ := e.auditEvents(t, admin, "action=auth.refresh_reuse&limit=10")
	if len(events) != 0 {
		t.Fatalf("no theft alarm for a racing repeat: %v", events)
	}

	// Much later, the original token is presented again: it must have leaked.
	clk.advance(time.Minute)
	if res := e.do(t, "POST", "/api/v1/auth/refresh", req{cookie: cookie1, headers: csrf}); res.StatusCode != 401 {
		t.Fatalf("got %d", res.StatusCode)
	}
	// The newest cookie, which the legitimate client holds, is dead as well.
	cookie3 := e.do(t, "POST", "/api/v1/auth/refresh", req{cookie: cookie2, headers: csrf})
	if cookie3.StatusCode != 401 {
		t.Fatalf("the whole family must be revoked: %d", cookie3.StatusCode)
	}

	events, _ = e.auditEvents(t, admin, "action=auth.refresh_reuse&limit=10")
	if len(events) != 1 || events[0]["result"] != "denied" || events[0]["status"] != float64(401) || detail(events[0], "effect") == "" {
		t.Fatalf("exactly one security event must be recorded: %v", events)
	}
	if str(events[0], "actorName") != adminName || str(events[0], "actorId") == "" {
		t.Fatalf("the event must say whose session was compromised: %v", events[0])
	}
}

func TestClientIP(t *testing.T) {
	trusted, err := ParseTrustedProxies([]string{"10.0.0.0/8", "192.168.1.5", " ", "fd00::/8"})
	if err != nil {
		t.Fatal(err)
	}
	mk := func(remote string, xff ...string) *http.Request {
		r, _ := http.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		for _, v := range xff {
			r.Header.Add("X-Forwarded-For", v)
		}
		return r
	}
	tests := []struct {
		name string
		r    *http.Request
		want string
	}{
		{"direct client", mk("203.0.113.9:5555"), "203.0.113.9"},
		{"header from an untrusted peer is ignored", mk("203.0.113.9:5555", "1.2.3.4"), "203.0.113.9"},
		{"trusted proxy, no header", mk("10.1.2.3:80"), "10.1.2.3"},
		{"trusted proxy, one client", mk("10.1.2.3:80", "198.51.100.4"), "198.51.100.4"},
		{"a chain of trusted proxies", mk("10.1.2.3:80", "198.51.100.4, 10.9.9.9, 192.168.1.5"), "198.51.100.4"},
		{"the rightmost untrusted entry wins over forged ones", mk("10.1.2.3:80", "6.6.6.6, 198.51.100.4"), "198.51.100.4"},
		{"several header lines", mk("10.1.2.3:80", "6.6.6.6", "198.51.100.4"), "198.51.100.4"},
		{"a garbled header is not believed", mk("10.1.2.3:80", "not-an-ip"), "10.1.2.3"},
		{"garbage mixed in is not believed either", mk("10.1.2.3:80", "198.51.100.4, junk"), "10.1.2.3"},
		{"only trusted addresses", mk("10.1.2.3:80", "10.5.5.5"), "10.1.2.3"},
		{"IPv6 client", mk("[fd00::1]:80", "2001:db8::7"), "2001:db8::7"},
		{"IPv4-mapped peer is matched", mk("[::ffff:10.0.0.9]:80", "198.51.100.4"), "198.51.100.4"},
		{"no port in RemoteAddr", mk("203.0.113.9"), "203.0.113.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clientIP(tt.r, trusted); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
	if got := clientIP(mk("10.1.2.3:80", "198.51.100.4"), nil); got != "10.1.2.3" {
		t.Fatalf("with no trusted proxies the header is never believed: %q", got)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	got, err := ParseTrustedProxies([]string{"10.0.0.0/8", "192.168.1.5", "10.1.2.3/8"})
	if err != nil || len(got) != 3 {
		t.Fatalf("%v %v", got, err)
	}
	if got[1] != netip.MustParsePrefix("192.168.1.5/32") {
		t.Fatalf("a single address is a /32: %v", got[1])
	}
	if got[2] != netip.MustParsePrefix("10.0.0.0/8") {
		t.Fatalf("host bits are masked off: %v", got[2])
	}
	for _, bad := range []string{"nope", "10.0.0.0/33", "300.1.1.1"} {
		if _, err := ParseTrustedProxies([]string{bad}); err == nil || !strings.Contains(err.Error(), bad) {
			t.Errorf("%q must be rejected with a message naming it: %v", bad, err)
		}
	}
}
