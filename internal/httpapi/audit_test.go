package httpapi

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

type event = map[string]any

func (e *testEnv) auditEvents(t *testing.T, token, query string) ([]event, result) {
	t.Helper()
	path := "/api/v1/audit"
	if query != "" {
		path += "?" + query
	}
	res := e.do(t, "GET", path, req{token: token})
	if res.StatusCode != 200 {
		t.Fatalf("audit: %d %v", res.StatusCode, res.body)
	}
	items, _ := res.body["items"].([]any)
	out := make([]event, 0, len(items))
	for _, it := range items {
		out = append(out, it.(map[string]any))
	}
	return out, res
}

func findEvent(events []event, action string) event {
	for _, ev := range events { // newest first
		if ev["action"] == action {
			return ev
		}
	}
	return nil
}

func detail(ev event, key string) string {
	d, _ := ev["details"].(map[string]any)
	s, _ := d[key].(string)
	return s
}

func at(t *testing.T, ev event) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, str(ev, "time"))
	if err != nil {
		t.Fatalf("bad time in %v: %v", ev, err)
	}
	return parsed
}

func str(ev event, key string) string {
	s, _ := ev[key].(string)
	return s
}

func TestAuditRecordsWhatHappened(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	me := e.do(t, "GET", "/api/v1/auth/me", req{token: admin})

	cluster := e.newCluster(t, admin, "alpha")
	uid := e.newUser(t, admin, "ayse")
	team := e.newTeam(t, admin, "payments", uid)
	grant := e.grant(t, admin, uid, "viewer", clusterScope(cluster))
	e.do(t, "PATCH", "/api/v1/users/"+uid, req{token: admin, body: `{"displayName":"Ayşe","disabled":false}`})
	e.do(t, "PUT", "/api/v1/teams/"+team+"/members", req{token: admin, body: `{"members":[]}`})
	e.do(t, "DELETE", apiClusters+"/"+cluster, req{token: admin})

	events, _ := e.auditEvents(t, admin, "limit=200")

	t.Run("who, what, about what, and the outcome", func(t *testing.T) {
		ev := findEvent(events, "user.create")
		if ev == nil {
			t.Fatalf("no user.create event in %d events", len(events))
		}
		if str(ev, "actorId") != me.str("id") || str(ev, "actorName") != adminName ||
			str(ev, "subject") != "user:"+uid || str(ev, "subjectName") != "ayse" ||
			str(ev, "result") != "success" || ev["status"] != float64(201) || str(ev, "ip") != "127.0.0.1" || str(ev, "id") == "" {
			t.Fatalf("unexpected event: %v", ev)
		}
		if str(ev, "time") == "" {
			t.Fatal("events carry a time")
		}
	})
	t.Run("a deleted thing keeps its name in the log", func(t *testing.T) {
		ev := findEvent(events, "cluster.delete")
		if ev == nil || str(ev, "subject") != "cluster:"+cluster || str(ev, "subjectName") != "alpha" {
			t.Fatalf("the cluster cannot be looked up any more, so its name must be in the event: %v", ev)
		}
	})
	t.Run("grants say who got which role where", func(t *testing.T) {
		ev := findEvent(events, "role_binding.create")
		if ev == nil || str(ev, "subject") != "role_binding:"+grant.str("id") ||
			detail(ev, "role") != "viewer" || detail(ev, "to") != "user:"+uid ||
			detail(ev, "scope") != "cluster=id:"+cluster+" application=any" {
			t.Fatalf("unexpected event: %v", ev)
		}
	})
	t.Run("updates list what was changed", func(t *testing.T) {
		ev := findEvent(events, "user.update")
		if ev == nil || detail(ev, "fields") != "displayName,disabled" || detail(ev, "disabled") != "false" || str(ev, "subjectName") != "ayse" {
			t.Fatalf("unexpected event: %v", ev)
		}
	})
	t.Run("team changes", func(t *testing.T) {
		if ev := findEvent(events, "team.create"); ev == nil || str(ev, "subjectName") != "payments" || detail(ev, "members") != "1" {
			t.Fatalf("team.create: %v", ev)
		}
		if ev := findEvent(events, "team.set_members"); ev == nil || str(ev, "subject") != "team:"+team || str(ev, "subjectName") != "payments" || detail(ev, "members") != "0" {
			t.Fatalf("team.set_members: %v", ev)
		}
	})
	t.Run("newest first", func(t *testing.T) {
		if str(events[0], "action") != "cluster.delete" {
			t.Fatalf("the latest action must come first, got %v", events[0]["action"])
		}
		for i := 1; i < len(events); i++ {
			if at(t, events[i]).After(at(t, events[i-1])) {
				t.Fatalf("events must be sorted newest first (%s before %s)", str(events[i-1], "time"), str(events[i], "time"))
			}
		}
	})
}

func TestReadsAreNotAudited(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	before, _ := e.auditEvents(t, admin, "limit=200")
	for _, p := range []string{apiClusters, "/api/v1/users", "/api/v1/teams", "/api/v1/resource-groups", "/api/v1/roles", "/api/v1/role-bindings", "/api/v1/auth/me", "/api/v1/tokens", "/api/v1/audit", "/healthz"} {
		e.do(t, "GET", p, req{token: admin})
	}
	after, _ := e.auditEvents(t, admin, "limit=200")
	if len(after) != len(before) {
		t.Fatalf("reading must not create events: %d before, %d after", len(before), len(after))
	}
}

func TestFailedLoginsAreAuditedWithTheirReason(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	e.newUser(t, admin, "ayse")
	gone := e.newUser(t, admin, "gone")
	e.do(t, "PATCH", "/api/v1/users/"+gone, req{token: admin, body: `{"disabled":true}`})
	attempt := func(user, pass string) result {
		return e.do(t, "POST", "/api/v1/auth/login", req{body: fmt.Sprintf(`{"username":%q,"password":%q}`, user, pass)})
	}

	responses := map[string]string{}
	for name, tt := range map[string][2]string{
		"unknown_user": {"ghost", "whatever-it-is-123"},
		"bad_password": {"ayse", "definitely-wrong-1"},
		"disabled":     {"gone", userPass},
	} {
		res := attempt(tt[0], tt[1])
		if res.StatusCode != 401 || res.errorCode() != "invalid_credentials" {
			t.Fatalf("%s: %d %v", name, res.StatusCode, res.body)
		}
		responses[name] = res.raw
	}
	// The client must not be able to tell the reasons apart.
	if responses["unknown_user"] != responses["bad_password"] || responses["bad_password"] != responses["disabled"] {
		t.Fatalf("every failure must look identical to the client: %v", responses)
	}

	for i := 0; i < 5; i++ { // lock ayse's account
		attempt("ayse", "definitely-wrong-1")
	}
	attempt("ayse", userPass+"-changed") // locked: even a correct-looking attempt is refused

	events, _ := e.auditEvents(t, admin, "action=auth.login&result=denied&limit=200")
	reasons := map[string]int{}
	for _, ev := range events {
		if ev["result"] != "denied" || ev["status"] != float64(401) {
			t.Fatalf("a failed login is a denied 401: %v", ev)
		}
		if str(ev, "actorId") != "" {
			t.Fatalf("nobody was signed in, so there is no actor id: %v", ev)
		}
		reasons[detail(ev, "reason")]++
	}
	for _, want := range []string{"unknown_user", "bad_password", "disabled", "locked"} {
		if reasons[want] == 0 {
			t.Errorf("no event with reason %q in %v", want, reasons)
		}
	}
	var tried bool
	for _, ev := range events {
		if str(ev, "actorName") == "ghost" {
			tried = true
		}
	}
	if !tried {
		t.Fatal("the name that was tried must be recorded")
	}
}

func TestSuccessfulLoginAndPasswordChangeAreAudited(t *testing.T) {
	e := newEnv(t)
	e.admin(t) // logs in and changes the password
	admin := e.login(t, adminPass2).str("accessToken")

	events, _ := e.auditEvents(t, admin, "limit=200")
	login := findEvent(events, "auth.login")
	if login == nil || login["result"] != "success" || str(login, "actorName") != adminName || str(login, "actorId") == "" {
		t.Fatalf("a successful login must name the user: %v", login)
	}
	pw := findEvent(events, "auth.password_change")
	if pw == nil || pw["result"] != "success" || str(pw, "actorName") != adminName {
		t.Fatalf("password change: %v", pw)
	}
}

func TestDeniedAndFailedRequestsAreAuditedButUnauthenticatedOnesAreNot(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	uid := e.newUser(t, admin, "ayse")
	tok := e.userToken(t, "ayse")
	before, _ := e.auditEvents(t, admin, "limit=200")

	// Not signed in: refused before any handler runs. Anyone on the internet can do this, so it must not
	// be able to fill the log.
	for i := 0; i < 5; i++ {
		e.do(t, "POST", apiClusters, req{body: `{"name":"x","apiServer":"https://x"}`})
	}
	// Signed in but not allowed.
	e.do(t, "POST", apiClusters, req{token: tok, body: `{"name":"x","apiServer":"https://x"}`})
	// Allowed but invalid.
	e.do(t, "POST", apiClusters, req{token: admin, body: `{"name":"Bad Name","apiServer":"https://x"}`})

	after, _ := e.auditEvents(t, admin, "limit=200")
	if len(after)-len(before) != 2 {
		t.Fatalf("exactly the two authenticated attempts must be recorded, got %d new events", len(after)-len(before))
	}
	denied := after[1] // the 403 came before the 400
	if denied["action"] != "cluster.create" || denied["result"] != "denied" || denied["status"] != float64(403) || str(denied, "actorId") != uid {
		t.Fatalf("a refused attempt must be recorded with its actor: %v", denied)
	}
	failed := after[0]
	if failed["action"] != "cluster.create" || failed["result"] != "failure" || failed["status"] != float64(400) {
		t.Fatalf("an invalid request is a failure: %v", failed)
	}
}

func TestAuditContainsNoSecrets(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	secrets := []string{"chosen-password-123", "reset-password-456", "Brand-New-Password-789", "wrong-attempt-pass-1"}

	id := e.do(t, "POST", "/api/v1/users", req{token: admin, body: `{"username":"ayse","password":"chosen-password-123"}`}).str("id")
	e.do(t, "POST", "/api/v1/users/"+id+"/reset-password", req{token: admin, body: `{"password":"reset-password-456"}`})
	gen := e.do(t, "POST", "/api/v1/users", req{token: admin, body: `{"username":"mehmet"}`})
	secrets = append(secrets, gen.str("temporaryPassword"))
	e.do(t, "POST", "/api/v1/auth/login", req{body: `{"username":"ayse","password":"wrong-attempt-pass-1"}`})
	ayse := e.do(t, "POST", "/api/v1/auth/login", req{body: `{"username":"ayse","password":"reset-password-456"}`})
	e.do(t, "PUT", "/api/v1/auth/password", req{token: ayse.str("accessToken"), body: `{"currentPassword":"reset-password-456","newPassword":"Brand-New-Password-789"}`})
	created := e.do(t, "POST", "/api/v1/tokens", req{token: admin, body: `{"name":"ci"}`})
	secrets = append(secrets, created.str("token"), strings.TrimPrefix(created.str("token"), "flx_"))
	secrets = append(secrets, ayse.str("accessToken"), admin)

	_, res := e.auditEvents(t, admin, "limit=200")
	for _, secret := range secrets {
		if secret != "" && strings.Contains(res.raw, secret) {
			t.Fatalf("the audit log contains a secret: %q", secret)
		}
	}
	for _, marker := range []string{"argon2", "passwordHash", "tokenHash", "accessToken", "refresh"} {
		if strings.Contains(res.raw, marker) {
			t.Fatalf("the audit log mentions %q", marker)
		}
	}
	if !strings.Contains(res.raw, "token.create") {
		t.Fatal("the token creation itself must be recorded")
	}
}

func TestAuditRequiresItsPermission(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	uid := e.newUser(t, admin, "ayse")
	e.grant(t, admin, uid, "maintainer", "")
	e.grant(t, admin, uid, "cluster-admin", "")
	tok := e.userToken(t, "ayse")
	if res := e.do(t, "GET", "/api/v1/audit", req{token: tok}); res.StatusCode != 403 {
		t.Fatalf("got %d", res.StatusCode)
	}
	if res := e.do(t, "GET", "/api/v1/audit", req{}); res.StatusCode != 401 {
		t.Fatalf("got %d", res.StatusCode)
	}
}

func TestAuditFiltersAndPaging(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	a := e.newUser(t, admin, "ayse")
	e.newUser(t, admin, "mehmet")
	e.newCluster(t, admin, "alpha")
	e.newCluster(t, admin, "beta")
	e.do(t, "POST", apiClusters, req{token: admin, body: `{"name":"Bad Name","apiServer":"x"}`}) // a failure
	tok := e.userToken(t, "ayse")
	e.do(t, "POST", apiClusters, req{token: tok, body: `{"name":"x","apiServer":"https://x"}`}) // a denial

	all, _ := e.auditEvents(t, admin, "limit=200")
	if len(all) < 8 {
		t.Fatalf("expected at least 8 events, got %d", len(all))
	}

	t.Run("by action", func(t *testing.T) {
		got, _ := e.auditEvents(t, admin, "action=cluster.create&limit=200")
		var successes int
		for _, ev := range got {
			if ev["action"] != "cluster.create" {
				t.Fatalf("wrong action: %v", ev)
			}
			if ev["result"] == "success" {
				successes++
			}
		}
		if len(got) != 4 || successes != 2 {
			t.Fatalf("2 created + 1 invalid + 1 denied, got %d events (%d successful)", len(got), successes)
		}
	})
	t.Run("by result", func(t *testing.T) {
		got, _ := e.auditEvents(t, admin, "result=denied&limit=200")
		if len(got) == 0 {
			t.Fatal("expected the denial")
		}
		for _, ev := range got {
			if ev["result"] != "denied" {
				t.Fatalf("wrong result: %v", ev)
			}
		}
	})
	t.Run("by actor", func(t *testing.T) {
		got, _ := e.auditEvents(t, admin, "actor="+a+"&limit=200")
		if len(got) == 0 {
			t.Fatal("ayse acted")
		}
		for _, ev := range got {
			if ev["actorId"] != a {
				t.Fatalf("wrong actor: %v", ev)
			}
		}
	})
	t.Run("by time: from is inclusive, to is exclusive", func(t *testing.T) {
		midEvent := all[len(all)/2]
		mid := at(t, midEvent)
		since, _ := e.auditEvents(t, admin, "from="+url.QueryEscape(str(midEvent, "time"))+"&limit=200")
		until, _ := e.auditEvents(t, admin, "to="+url.QueryEscape(str(midEvent, "time"))+"&limit=200")
		for _, ev := range since {
			if at(t, ev).Before(mid) {
				t.Fatalf("an event before 'from': %v", ev)
			}
		}
		for _, ev := range until {
			if !at(t, ev).Before(mid) {
				t.Fatalf("an event at or after 'to': %v", ev)
			}
		}
		if len(since)+len(until) != len(all) {
			t.Fatalf("the two halves must add up: %d + %d != %d", len(since), len(until), len(all))
		}
	})
	t.Run("paging walks the list exactly once", func(t *testing.T) {
		var ids []string
		cursor := ""
		for i := 0; i < 50; i++ {
			q := "limit=3"
			if cursor != "" {
				q += "&cursor=" + url.QueryEscape(cursor)
			}
			page, res := e.auditEvents(t, admin, q)
			if len(page) > 3 {
				t.Fatalf("a page must not exceed the limit: %d", len(page))
			}
			for _, ev := range page {
				ids = append(ids, str(ev, "id"))
			}
			next, _ := res.body["nextCursor"].(string)
			if next == "" {
				if len(page) == 0 {
					t.Fatal("an empty last page")
				}
				break
			}
			if len(page) != 3 {
				t.Fatalf("a page with a next cursor must be full: %d", len(page))
			}
			cursor = next
		}
		want := make([]string, 0, len(all))
		for _, ev := range all {
			want = append(want, str(ev, "id"))
		}
		if fmt.Sprint(ids) != fmt.Sprint(want) {
			t.Fatalf("paging must reproduce the full list in order:\n got %v\nwant %v", ids, want)
		}
	})
	t.Run("no cursor when everything fits", func(t *testing.T) {
		_, res := e.auditEvents(t, admin, "limit=200")
		if _, has := res.body["nextCursor"]; has {
			t.Fatal("nextCursor must be absent on the last page")
		}
	})
	t.Run("bad parameters", func(t *testing.T) {
		for _, q := range []string{"limit=0", "limit=201", "limit=x", "result=maybe", "from=yesterday", "to=2026-13-45", "cursor=garbage"} {
			if res := e.do(t, "GET", "/api/v1/audit?"+q, req{token: admin}); res.StatusCode != 400 {
				t.Errorf("%s: got %d, want 400", q, res.StatusCode)
			}
		}
	})
}

func TestRateLimitedRequestsAreNotAudited(t *testing.T) {
	clk := newTestClock()
	e := newEnv(t, testOpts{loginPerMinute: 3, clock: clk})
	var statuses []int
	for i := 0; i < 8; i++ {
		res := e.do(t, "POST", "/api/v1/auth/login", req{body: `{"username":"x","password":"y"}`})
		statuses = append(statuses, res.StatusCode)
	}
	if fmt.Sprint(statuses) != "[401 401 401 429 429 429 429 429]" {
		t.Fatalf("3 attempts get through, the rest are turned away: %v", statuses)
	}

	clk.advance(10 * time.Minute) // the limiter refills
	admin := e.admin(t)           // one real login, which is recorded too
	events, _ := e.auditEvents(t, admin, "action=auth.login&limit=200")
	if len(events) != 3+1 {
		t.Fatalf("the 5 refused attempts must leave no trace: want 3 failures + 1 login, got %d events", len(events))
	}
}

func TestAuditFailureDoesNotFailTheRequest(t *testing.T) {
	e := newEnv(t, testOpts{auditFails: true})
	admin := e.admin(t)
	res := e.do(t, "POST", apiClusters, req{token: admin, body: `{"name":"alpha","apiServer":"https://a:6443"}`})
	if res.StatusCode != 201 {
		t.Fatalf("an unwritable audit log must not stop the service: %d %v", res.StatusCode, res.body)
	}
}

func TestAuditIDsSortByCreationTime(t *testing.T) {
	base := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	var prev string
	for i := 0; i < 1000; i++ {
		// Events of the same millisecond differ only in the nanoseconds, so the order must survive that.
		id := newAuditID(base.Add(time.Duration(i) * 100 * time.Microsecond))
		if prev != "" && id <= prev {
			t.Fatalf("id %d (%s) must sort after the one before it (%s)", i, id, prev)
		}
		if len(id) != 24 {
			t.Fatalf("unexpected id length: %q", id)
		}
		prev = id
	}
	// Two events at the very same instant still get different IDs.
	if a, b := newAuditID(base), newAuditID(base); a == b {
		t.Fatalf("ids must be unique even for the same instant: %s", a)
	}
}
