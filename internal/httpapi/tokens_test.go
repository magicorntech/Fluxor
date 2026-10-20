package httpapi

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func (e *testEnv) newToken(t *testing.T, sessionToken, name string, days int) result {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q`, name)
	if days != 0 {
		body += fmt.Sprintf(`,"expiresInDays":%d`, days)
	}
	res := e.do(t, "POST", "/api/v1/tokens", req{token: sessionToken, body: body + `}`})
	if res.StatusCode != 201 {
		t.Fatalf("create token: %d %v", res.StatusCode, res.body)
	}
	return res
}

func TestCreatingAndUsingAnAPIToken(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	uid := e.newUser(t, admin, "ayse")
	cluster := e.newCluster(t, admin, "alpha")
	e.newCluster(t, admin, "beta")
	e.grant(t, admin, uid, "viewer", clusterScope(cluster))
	session := e.userToken(t, "ayse")

	created := e.newToken(t, session, "ci pipeline", 0)
	secret := created.str("token")
	if !strings.HasPrefix(secret, "flx_") || len(secret) != 47 {
		t.Fatalf("unexpected secret: %q", secret)
	}
	if created.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("a response holding a secret must not be cached")
	}
	if created.str("name") != "ci pipeline" || created.str("prefix") != secret[:8] || created.str("userId") != uid {
		t.Fatalf("unexpected token: %v", created.body)
	}
	expires, err := time.Parse(time.RFC3339, created.str("expiresAt"))
	if err != nil || time.Until(expires) < 29*24*time.Hour || time.Until(expires) > 31*24*time.Hour {
		t.Fatalf("the default lifetime is 30 days: %v %v", created.str("expiresAt"), err)
	}
	for _, leak := range []string{"tokenHash", "hash"} {
		if strings.Contains(created.raw, leak) {
			t.Fatalf("the response leaks %q: %s", leak, created.raw)
		}
	}

	t.Run("the token acts as its owner, with exactly the owner's permissions", func(t *testing.T) {
		list := e.do(t, "GET", apiClusters, req{token: secret})
		if list.StatusCode != 200 || !sameSet(itemIDs(list), []string{cluster}) {
			t.Fatalf("it must see what ayse sees: %d %v", list.StatusCode, list.body)
		}
		if res := e.do(t, "POST", apiClusters, req{token: secret, body: `{"name":"x","apiServer":"https://x"}`}); res.StatusCode != 403 {
			t.Fatalf("it must not do more than ayse may: %d", res.StatusCode)
		}
		if res := e.do(t, "GET", "/api/v1/users", req{token: secret}); res.StatusCode != 403 {
			t.Fatalf("%d", res.StatusCode)
		}
		me := e.do(t, "GET", "/api/v1/auth/me", req{token: secret})
		if me.StatusCode != 200 || me.str("username") != "ayse" {
			t.Fatalf("%d %v", me.StatusCode, me.body)
		}
	})

	t.Run("it cannot create or delete tokens or change the password", func(t *testing.T) {
		for _, tt := range []struct{ method, path, body string }{
			{"POST", "/api/v1/tokens", `{"name":"more"}`},
			{"GET", "/api/v1/tokens", ""},
			{"DELETE", "/api/v1/tokens/" + created.str("id"), ""},
			{"PUT", "/api/v1/auth/password", fmt.Sprintf(`{"currentPassword":%q,"newPassword":"another-long-password-1"}`, userPass+"-changed")},
		} {
			res := e.do(t, tt.method, tt.path, req{token: secret, body: tt.body})
			if res.StatusCode != 403 || res.errorCode() != "forbidden" {
				t.Errorf("%s %s: got %d %v, want 403", tt.method, tt.path, res.StatusCode, res.body)
			}
		}
		// None of it may have had an effect.
		if res := e.do(t, "GET", "/api/v1/tokens", req{token: session}); len(itemIDs(res)) != 1 {
			t.Fatalf("the token list changed: %v", res.body)
		}
		if res := e.do(t, "GET", apiClusters, req{token: secret}); res.StatusCode != 200 {
			t.Fatalf("the token must still work: %d", res.StatusCode)
		}
	})

	t.Run("the list shows prefix and last use but never the secret", func(t *testing.T) {
		list := e.do(t, "GET", "/api/v1/tokens", req{token: session})
		items, _ := list.body["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("%v", list.body)
		}
		item := items[0].(map[string]any)
		if item["prefix"] != secret[:8] || item["lastUsedAt"] == nil || item["name"] != "ci pipeline" {
			t.Fatalf("unexpected item: %v", item)
		}
		if strings.Contains(list.raw, secret) || strings.Contains(list.raw, secret[8:]) {
			t.Fatal("the list must not contain the secret")
		}
	})

	t.Run("actions taken with a token are marked as such in the audit log", func(t *testing.T) {
		e.do(t, "POST", apiClusters, req{token: secret, body: `{"name":"x","apiServer":"https://x"}`})
		events, _ := e.auditEvents(t, admin, "action=cluster.create&actor="+uid+"&limit=10")
		if len(events) == 0 || detail(events[0], "via") != "api_token" {
			t.Fatalf("the audit log must say the token was used: %v", events)
		}
	})
}

func TestAPITokenValidationAndLimit(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	post := func(body string) result { return e.do(t, "POST", "/api/v1/tokens", req{token: admin, body: body}) }

	for name, body := range map[string]string{
		"empty name":      `{"name":""}`,
		"missing name":    `{}`,
		"long name":       `{"name":"` + strings.Repeat("a", 65) + `"}`,
		"negative life":   `{"name":"x","expiresInDays":-1}`,
		"too long a life": `{"name":"x","expiresInDays":366}`,
		"unknown field":   `{"name":"x","scopes":["all"]}`,
		"malformed":       `{`,
	} {
		if res := post(body); res.StatusCode != 400 || res.errorCode() != "invalid_argument" {
			t.Errorf("%s: got %d %v, want 400", name, res.StatusCode, res.body)
		}
	}
	for _, days := range []int{1, 365} {
		if res := post(fmt.Sprintf(`{"name":"edge","expiresInDays":%d}`, days)); res.StatusCode != 201 {
			t.Errorf("%d days must be accepted: %d %v", days, res.StatusCode, res.body)
		}
	}

	for i := 0; i < 18; i++ { // 2 exist already, 20 is the limit
		if res := post(`{"name":"filler"}`); res.StatusCode != 201 {
			t.Fatalf("token %d: %d %v", i, res.StatusCode, res.body)
		}
	}
	if res := post(`{"name":"one too many"}`); res.StatusCode != 409 || res.errorCode() != "limit_reached" {
		t.Fatalf("got %d %v", res.StatusCode, res.body)
	}
}

func TestRevokingAnAPIToken(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	e.newUser(t, admin, "ayse")
	other := e.userToken(t, "ayse")
	mine := e.newToken(t, admin, "mine", 0)
	secret := mine.str("token")

	if res := e.do(t, "GET", apiClusters, req{token: secret}); res.StatusCode != 200 {
		t.Fatalf("precondition: %d", res.StatusCode)
	}
	// Someone else's token looks exactly like one that does not exist.
	foreign := e.do(t, "DELETE", "/api/v1/tokens/"+mine.str("id"), req{token: other})
	missing := e.do(t, "DELETE", "/api/v1/tokens/does-not-exist", req{token: other})
	if foreign.StatusCode != 404 || missing.StatusCode != 404 || foreign.raw != missing.raw {
		t.Fatalf("existence must not leak: %d %q vs %d %q", foreign.StatusCode, foreign.raw, missing.StatusCode, missing.raw)
	}
	if res := e.do(t, "GET", apiClusters, req{token: secret}); res.StatusCode != 200 {
		t.Fatalf("the foreign delete must have had no effect: %d", res.StatusCode)
	}

	if res := e.do(t, "DELETE", "/api/v1/tokens/"+mine.str("id"), req{token: admin}); res.StatusCode != 204 {
		t.Fatalf("got %d", res.StatusCode)
	}
	if res := e.do(t, "GET", apiClusters, req{token: secret}); res.StatusCode != 401 {
		t.Fatalf("a revoked token must stop working at once: %d", res.StatusCode)
	}
	if res := e.do(t, "DELETE", "/api/v1/tokens/"+mine.str("id"), req{token: admin}); res.StatusCode != 404 {
		t.Fatalf("second delete: %d", res.StatusCode)
	}
	events, _ := e.auditEvents(t, admin, "action=token.revoke&limit=10")
	if len(events) != 4 { // the refused foreign attempt, the unknown id, the real one, and the repeat
		t.Fatalf("every attempt is audited, successful or not: got %d events", len(events))
	}
	if first := events[len(events)-1]; str(first, "subject") != "token:"+mine.str("id") || first["result"] != "failure" || str(first, "actorName") != "ayse" {
		t.Fatalf("the attempt on someone else's token must be on record: %v", first)
	}
	if ok := events[1]; ok["result"] != "success" || ok["status"] != float64(204) || str(ok, "subjectName") != "mine" {
		t.Fatalf("the real revocation must carry the token's name: %v", ok)
	}
	if first := events[len(events)-1]; str(first, "subjectName") != "" {
		t.Fatalf("a foreign token's name must not be revealed through the log: %v", first)
	}
}

func TestAPITokensAreRevokedWhenAccessMustEnd(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	uid := e.newUser(t, admin, "ayse")
	session := e.userToken(t, "ayse")

	t.Run("an administrator's password reset revokes them", func(t *testing.T) {
		secret := e.newToken(t, session, "ci", 0).str("token")
		if res := e.do(t, "GET", apiClusters, req{token: secret}); res.StatusCode != 200 {
			t.Fatalf("precondition: %d", res.StatusCode)
		}
		e.do(t, "POST", "/api/v1/users/"+uid+"/reset-password", req{token: admin, body: fmt.Sprintf(`{"password":%q}`, userPass)})
		if res := e.do(t, "GET", apiClusters, req{token: secret}); res.StatusCode != 401 {
			t.Fatalf("got %d", res.StatusCode)
		}
	})

	t.Run("disabling the account revokes them for good, even if it is enabled again", func(t *testing.T) {
		tok := e.userToken(t, "ayse") // the reset above forced a new first-login change
		secret := e.newToken(t, tok, "ci", 0).str("token")
		e.do(t, "PATCH", "/api/v1/users/"+uid, req{token: admin, body: `{"disabled":true}`})
		if res := e.do(t, "GET", apiClusters, req{token: secret}); res.StatusCode != 401 {
			t.Fatalf("a disabled user's token must not work: %d", res.StatusCode)
		}
		e.do(t, "PATCH", "/api/v1/users/"+uid, req{token: admin, body: `{"disabled":false}`})
		if res := e.do(t, "GET", apiClusters, req{token: secret}); res.StatusCode != 401 {
			t.Fatalf("re-enabling must not bring old credentials back: %d", res.StatusCode)
		}
	})
}

func TestChangingYourOwnPasswordKeepsYourTokens(t *testing.T) {
	e := newEnv(t)
	admin, _ := e.ready(t)
	secret := e.newToken(t, admin, "ci", 0).str("token")
	res := e.do(t, "PUT", "/api/v1/auth/password", req{token: admin, body: fmt.Sprintf(`{"currentPassword":%q,"newPassword":"yet-another-password-3"}`, adminPass2)})
	if res.StatusCode != 200 {
		t.Fatalf("%d %v", res.StatusCode, res.body)
	}
	if r := e.do(t, "GET", apiClusters, req{token: secret}); r.StatusCode != 200 {
		t.Fatalf("a CI token must survive its owner changing their own password: %d", r.StatusCode)
	}
}

func TestAnAPITokenExpires(t *testing.T) {
	clk := newTestClock()
	e := newEnv(t, testOpts{clock: clk})
	admin := e.admin(t)
	secret := e.newToken(t, admin, "short", 1).str("token")
	if res := e.do(t, "GET", apiClusters, req{token: secret}); res.StatusCode != 200 {
		t.Fatalf("%d", res.StatusCode)
	}
	clk.advance(24*time.Hour + time.Minute)
	if res := e.do(t, "GET", apiClusters, req{token: secret}); res.StatusCode != 401 {
		t.Fatalf("an expired token must be rejected: %d", res.StatusCode)
	}
}

func TestTokensNeedASetUpAccount(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	e.newUser(t, admin, "fresh")
	// First login, password not changed yet: the account is still locked down.
	login := e.do(t, "POST", "/api/v1/auth/login", req{body: fmt.Sprintf(`{"username":"fresh","password":%q}`, userPass)})
	res := e.do(t, "POST", "/api/v1/tokens", req{token: login.str("accessToken"), body: `{"name":"early"}`})
	if res.StatusCode != 403 || res.errorCode() != "password_change_required" {
		t.Fatalf("got %d %v", res.StatusCode, res.body)
	}
}

func TestTokensRequireAuthentication(t *testing.T) {
	e := newEnv(t)
	for _, tt := range []struct{ method, path string }{{"GET", "/api/v1/tokens"}, {"POST", "/api/v1/tokens"}, {"DELETE", "/api/v1/tokens/x"}} {
		if res := e.do(t, tt.method, tt.path, req{}); res.StatusCode != 401 {
			t.Errorf("%s %s: %d", tt.method, tt.path, res.StatusCode)
		}
	}
	// A string that looks like a token but is not one is just an invalid credential.
	for _, bad := range []string{"flx_", "flx_" + strings.Repeat("A", 43), "flx_not-a-real-token"} {
		if res := e.do(t, "GET", apiClusters, req{token: bad}); res.StatusCode != 401 {
			t.Errorf("%q: %d", bad, res.StatusCode)
		}
	}
}
