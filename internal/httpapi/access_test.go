package httpapi

import (
	"fmt"
	"strings"
	"testing"
)

const userPass = "user-password-12345"

// admin returns the token of the fully set-up bootstrap administrator.
func (e *testEnv) admin(t *testing.T) string {
	t.Helper()
	token, _ := e.ready(t)
	return token
}

// newUser creates a user as the admin and returns its ID. The password is known to the test.
func (e *testEnv) newUser(t *testing.T, adminToken, username string) string {
	t.Helper()
	res := e.do(t, "POST", "/api/v1/users", req{token: adminToken, body: fmt.Sprintf(`{"username":%q,"password":%q}`, username, userPass)})
	if res.StatusCode != 201 {
		t.Fatalf("create user %s: %d %v", username, res.StatusCode, res.body)
	}
	return res.str("id")
}

// userToken signs the user in and performs the mandatory first password change.
func (e *testEnv) userToken(t *testing.T, username string) string {
	t.Helper()
	res := e.do(t, "POST", "/api/v1/auth/login", req{body: fmt.Sprintf(`{"username":%q,"password":%q}`, username, userPass)})
	if res.StatusCode != 200 {
		t.Fatalf("login %s: %d %v", username, res.StatusCode, res.body)
	}
	res = e.do(t, "PUT", "/api/v1/auth/password", req{
		token: res.str("accessToken"),
		body:  fmt.Sprintf(`{"currentPassword":%q,"newPassword":%q}`, userPass, userPass+"-changed"),
	})
	if res.StatusCode != 200 {
		t.Fatalf("first password change %s: %d %v", username, res.StatusCode, res.body)
	}
	return res.str("accessToken")
}

func (e *testEnv) newCluster(t *testing.T, adminToken, name string) string {
	t.Helper()
	res := e.do(t, "POST", apiClusters, req{token: adminToken, body: fmt.Sprintf(`{"name":%q,"apiServer":"https://%s:6443"}`, name, name)})
	if res.StatusCode != 201 {
		t.Fatalf("create cluster %s: %d %v", name, res.StatusCode, res.body)
	}
	return res.str("id")
}

func (e *testEnv) grant(t *testing.T, adminToken, userID, role, scopeJSON string) result {
	t.Helper()
	body := fmt.Sprintf(`{"subject":{"kind":"user","id":%q},"roleId":%q`, userID, role)
	if scopeJSON != "" {
		body += `,"scope":` + scopeJSON
	}
	return e.do(t, "POST", "/api/v1/role-bindings", req{token: adminToken, body: body + `}`})
}

func clusterScope(id string) string {
	return fmt.Sprintf(`{"cluster":{"kind":"id","id":%q}}`, id)
}

func itemIDs(res result) []string {
	items, _ := res.body["items"].([]any)
	out := make([]string, 0, len(items))
	for _, it := range items {
		m, _ := it.(map[string]any)
		id, _ := m["id"].(string)
		out = append(out, id)
	}
	return out
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]bool{}
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			return false
		}
	}
	return true
}

func TestClusterAccessControl(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	alpha := e.newCluster(t, admin, "alpha")
	beta := e.newCluster(t, admin, "beta")
	uid := e.newUser(t, admin, "ayse")
	tok := e.userToken(t, "ayse")

	call := func(method, path, body string) result { return e.do(t, method, path, req{token: tok, body: body}) }

	t.Run("a user without grants sees nothing and can do nothing", func(t *testing.T) {
		if res := call("GET", apiClusters, ""); res.StatusCode != 200 || len(itemIDs(res)) != 0 {
			t.Fatalf("list must be an empty 200, not an error: %d %v", res.StatusCode, res.body)
		}
		if res := call("GET", apiClusters+"/"+alpha, ""); res.StatusCode != 404 {
			t.Fatalf("get: %d", res.StatusCode)
		}
		if res := call("DELETE", apiClusters+"/"+alpha, ""); res.StatusCode != 404 {
			t.Fatalf("delete: %d", res.StatusCode)
		}
		if res := call("POST", apiClusters, `{"name":"gamma","apiServer":"https://g"}`); res.StatusCode != 403 || res.errorCode() != "forbidden" {
			t.Fatalf("create: %d %v", res.StatusCode, res.body)
		}
	})

	t.Run("a viewer of one cluster sees only that cluster", func(t *testing.T) {
		if res := e.grant(t, admin, uid, "viewer", clusterScope(alpha)); res.StatusCode != 201 {
			t.Fatalf("grant: %d %v", res.StatusCode, res.body)
		}
		if res := call("GET", apiClusters, ""); !sameSet(itemIDs(res), []string{alpha}) {
			t.Fatalf("list must contain only alpha: %v", res.body)
		}
		if res := call("GET", apiClusters+"/"+alpha, ""); res.StatusCode != 200 || res.str("name") != "alpha" {
			t.Fatalf("get alpha: %d %v", res.StatusCode, res.body)
		}

		// A cluster the user may not see is indistinguishable from one that does not exist.
		hidden := call("GET", apiClusters+"/"+beta, "")
		missing := call("GET", apiClusters+"/does-not-exist", "")
		if hidden.StatusCode != 404 || missing.StatusCode != 404 || hidden.raw != missing.raw {
			t.Fatalf("existence must not leak: %d %q vs %d %q", hidden.StatusCode, hidden.raw, missing.StatusCode, missing.raw)
		}

		// Seeing a cluster is not the right to change it.
		if res := call("DELETE", apiClusters+"/"+alpha, ""); res.StatusCode != 403 {
			t.Fatalf("a viewer must not delete: %d", res.StatusCode)
		}
		if res := call("DELETE", apiClusters+"/"+beta, ""); res.StatusCode != 404 {
			t.Fatalf("delete of a hidden cluster: %d", res.StatusCode)
		}
	})

	t.Run("cluster-admin on one cluster manages it but cannot register new ones", func(t *testing.T) {
		if res := e.grant(t, admin, uid, "cluster-admin", clusterScope(alpha)); res.StatusCode != 201 {
			t.Fatalf("grant: %d %v", res.StatusCode, res.body)
		}
		if res := call("POST", apiClusters, `{"name":"gamma","apiServer":"https://g"}`); res.StatusCode != 403 {
			t.Fatalf("a grant over one cluster must not allow creating clusters: %d", res.StatusCode)
		}
		if res := call("DELETE", apiClusters+"/"+beta, ""); res.StatusCode != 404 {
			t.Fatalf("beta is still out of scope: %d", res.StatusCode)
		}

		bindingsBefore := e.do(t, "GET", "/api/v1/role-bindings?subjectKind=user&subjectId="+uid, req{token: admin})
		if len(itemIDs(bindingsBefore)) != 2 {
			t.Fatalf("precondition: want 2 bindings, got %v", bindingsBefore.body)
		}
		if res := call("DELETE", apiClusters+"/"+alpha, ""); res.StatusCode != 204 {
			t.Fatalf("delete alpha: %d", res.StatusCode)
		}
		// Grants that pointed at the removed cluster are cleaned up with it.
		after := e.do(t, "GET", "/api/v1/role-bindings?subjectKind=user&subjectId="+uid, req{token: admin})
		if len(itemIDs(after)) != 0 {
			t.Fatalf("bindings of the deleted cluster must be removed: %v", after.body)
		}
	})

	t.Run("cluster-admin on all clusters can register and sees everything", func(t *testing.T) {
		if res := e.grant(t, admin, uid, "cluster-admin", ""); res.StatusCode != 201 {
			t.Fatalf("grant: %d %v", res.StatusCode, res.body)
		}
		if res := call("POST", apiClusters, `{"name":"gamma","apiServer":"https://g"}`); res.StatusCode != 201 {
			t.Fatalf("create: %d %v", res.StatusCode, res.body)
		}
		if res := call("GET", apiClusters, ""); len(itemIDs(res)) != 2 {
			t.Fatalf("must see beta and gamma: %v", res.body)
		}
		if res := call("DELETE", apiClusters+"/"+beta, ""); res.StatusCode != 204 {
			t.Fatalf("delete beta: %d", res.StatusCode)
		}
	})
}

func TestAdminHasFullAccess(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	c := e.newCluster(t, admin, "alpha")
	if res := e.do(t, "GET", apiClusters+"/"+c, req{token: admin}); res.StatusCode != 200 {
		t.Fatalf("admin get: %d", res.StatusCode)
	}
	for _, path := range []string{"/api/v1/users", "/api/v1/roles", "/api/v1/role-bindings"} {
		if res := e.do(t, "GET", path, req{token: admin}); res.StatusCode != 200 {
			t.Fatalf("admin GET %s: %d %v", path, res.StatusCode, res.body)
		}
	}
}

func TestSystemEndpointsRequireTheirPermission(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	e.newUser(t, admin, "ayse")
	tok := e.userToken(t, "ayse")

	// Even the most powerful non-admin roles carry no system permission.
	ayse, _ := e.store.GetUserByUsername(t.Context(), "ayse")
	for _, role := range []string{"maintainer", "cluster-admin"} {
		if res := e.grant(t, admin, ayse.ID, role, ""); res.StatusCode != 201 {
			t.Fatalf("grant %s: %d %v", role, res.StatusCode, res.body)
		}
	}

	adminUser, _ := e.store.GetUserByUsername(t.Context(), adminName)
	tests := []struct{ method, path, body string }{
		{"GET", "/api/v1/users", ""},
		{"POST", "/api/v1/users", `{"username":"newbie"}`},
		{"GET", "/api/v1/users/" + adminUser.ID, ""},
		{"PATCH", "/api/v1/users/" + adminUser.ID, `{"displayName":"x"}`},
		{"POST", "/api/v1/users/" + adminUser.ID + "/reset-password", `{}`},
		{"GET", "/api/v1/roles", ""},
		{"GET", "/api/v1/role-bindings", ""},
		{"POST", "/api/v1/role-bindings", `{"subject":{"kind":"user","id":"` + ayse.ID + `"},"roleId":"admin"}`},
		{"DELETE", "/api/v1/role-bindings/" + "bootstrap-admin", ""},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			res := e.do(t, tt.method, tt.path, req{token: tok, body: tt.body})
			if res.StatusCode != 403 || res.errorCode() != "forbidden" {
				t.Fatalf("got %d %v, want 403 forbidden", res.StatusCode, res.body)
			}
		})
	}

	// Nothing above may have changed anything.
	if res := e.do(t, "GET", "/api/v1/role-bindings", req{token: admin}); len(itemIDs(res)) != 3 {
		t.Fatalf("the forbidden calls must have no effect: %v", res.body)
	}
	if u, _ := e.store.GetUserByUsername(t.Context(), "newbie"); u.ID != "" {
		t.Fatal("a forbidden user creation went through")
	}
}

func TestUsersAPI(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	post := func(body string) result { return e.do(t, "POST", "/api/v1/users", req{token: admin, body: body}) }

	t.Run("create with a generated temporary password", func(t *testing.T) {
		res := post(`{"username":"  Ayse.Yilmaz ","email":"Ayse@Example.com","displayName":"Ayşe Yılmaz"}`)
		if res.StatusCode != 201 {
			t.Fatalf("got %d %v", res.StatusCode, res.body)
		}
		pw := res.str("temporaryPassword")
		if len(pw) != 20 || res.str("username") != "ayse.yilmaz" || res.str("email") != "ayse@example.com" ||
			res.body["mustChangePassword"] != true || res.body["disabled"] != false {
			t.Fatalf("unexpected response: %v", res.body)
		}
		if res.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("a response holding a password must not be cached")
		}
		login := e.do(t, "POST", "/api/v1/auth/login", req{body: fmt.Sprintf(`{"username":"ayse.yilmaz","password":%q}`, pw)})
		if login.StatusCode != 200 {
			t.Fatalf("the temporary password must work: %d", login.StatusCode)
		}
		if user, _ := login.body["user"].(map[string]any); user["mustChangePassword"] != true {
			t.Fatal("the user must be told to change the password")
		}
	})

	t.Run("create with a chosen password does not echo it", func(t *testing.T) {
		res := post(`{"username":"mehmet","password":"chosen-password-123"}`)
		if res.StatusCode != 201 || res.str("temporaryPassword") != "" || strings.Contains(res.raw, "chosen-password-123") {
			t.Fatalf("got %d %s", res.StatusCode, res.raw)
		}
	})

	t.Run("validation", func(t *testing.T) {
		for name, tt := range map[string]struct {
			body string
			want int
		}{
			"bad username":   {`{"username":"a b"}`, 400},
			"bad email":      {`{"username":"valid.name","email":"nope"}`, 400},
			"weak password":  {`{"username":"valid.name","password":"short"}`, 400},
			"unknown field":  {`{"username":"valid.name","admin":true}`, 400},
			"malformed":      {`{`, 400},
			"duplicate name": {`{"username":"MEHMET"}`, 409},
			"duplicate mail": {`{"username":"other","email":"ayse@example.com"}`, 409},
		} {
			if res := post(tt.body); res.StatusCode != tt.want {
				t.Errorf("%s: got %d %v, want %d", name, res.StatusCode, res.body, tt.want)
			}
		}
	})

	t.Run("list and get never expose password hashes", func(t *testing.T) {
		list := e.do(t, "GET", "/api/v1/users", req{token: admin})
		if list.StatusCode != 200 || len(itemIDs(list)) != 3 {
			t.Fatalf("list: %d %s", list.StatusCode, list.raw)
		}
		first := list.body["items"].([]any)[0].(map[string]any)
		if first["username"] != "admin" {
			t.Fatalf("users must be sorted by username: %v", first)
		}
		id := itemIDs(list)[1]
		get := e.do(t, "GET", "/api/v1/users/"+id, req{token: admin})
		for _, raw := range []string{list.raw, get.raw} {
			for _, leak := range []string{"passwordHash", "argon2", "failedLogins", "lockedUntil"} {
				if strings.Contains(raw, leak) {
					t.Fatalf("response leaks %q: %s", leak, raw)
				}
			}
		}
		if res := e.do(t, "GET", "/api/v1/users/missing", req{token: admin}); res.StatusCode != 404 {
			t.Fatalf("unknown user: %d", res.StatusCode)
		}
	})
}

func TestUpdateUser(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	id := e.newUser(t, admin, "ayse")
	e.newUser(t, admin, "mehmet")
	patch := func(userID, body string) result {
		return e.do(t, "PATCH", "/api/v1/users/"+userID, req{token: admin, body: body})
	}

	if res := patch(id, `{"displayName":"  Ayşe  ","email":"ayse@example.com"}`); res.StatusCode != 200 || res.str("displayName") != "Ayşe" || res.str("email") != "ayse@example.com" {
		t.Fatalf("profile update: %d %v", res.StatusCode, res.body)
	}
	if res := patch(id, `{"email":""}`); res.StatusCode != 200 || res.str("email") != "" {
		t.Fatalf("clearing the email: %d %v", res.StatusCode, res.body)
	}
	mehmet, _ := e.store.GetUserByUsername(t.Context(), "mehmet")
	patch(mehmet.ID, `{"email":"taken@example.com"}`)
	if res := patch(id, `{"email":"TAKEN@example.com"}`); res.StatusCode != 409 {
		t.Fatalf("duplicate email: %d", res.StatusCode)
	}
	for name, body := range map[string]string{
		"bad email":      `{"email":"nope"}`,
		"long name":      `{"displayName":"` + strings.Repeat("x", 101) + `"}`,
		"unknown field":  `{"username":"renamed"}`,
		"cannot set pwd": `{"password":"x"}`,
	} {
		if res := patch(id, body); res.StatusCode != 400 {
			t.Errorf("%s: got %d, want 400", name, res.StatusCode)
		}
	}
	if res := patch("missing", `{"displayName":"x"}`); res.StatusCode != 404 {
		t.Fatalf("unknown user: %d", res.StatusCode)
	}
}

func TestDisablingAUserEndsTheirAccess(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	id := e.newUser(t, admin, "ayse")
	tok := e.userToken(t, "ayse")
	e.grant(t, admin, id, "viewer", "")
	if res := e.do(t, "GET", apiClusters, req{token: tok}); res.StatusCode != 200 {
		t.Fatalf("precondition: %d", res.StatusCode)
	}

	if res := e.do(t, "PATCH", "/api/v1/users/"+id, req{token: admin, body: `{"disabled":true}`}); res.StatusCode != 200 || res.body["disabled"] != true {
		t.Fatalf("disable: %d %v", res.StatusCode, res.body)
	}
	if res := e.do(t, "GET", apiClusters, req{token: tok}); res.StatusCode != 401 {
		t.Fatalf("an existing token must stop working: %d", res.StatusCode)
	}
	login := e.do(t, "POST", "/api/v1/auth/login", req{body: fmt.Sprintf(`{"username":"ayse","password":%q}`, userPass+"-changed")})
	if login.StatusCode != 401 {
		t.Fatalf("a disabled user must not log in: %d", login.StatusCode)
	}

	e.do(t, "PATCH", "/api/v1/users/"+id, req{token: admin, body: `{"disabled":false}`})
	login = e.do(t, "POST", "/api/v1/auth/login", req{body: fmt.Sprintf(`{"username":"ayse","password":%q}`, userPass+"-changed")})
	if login.StatusCode != 200 {
		t.Fatalf("an enabled user can log in again: %d", login.StatusCode)
	}
}

func TestAdminCannotDisableThemselves(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	me := e.do(t, "GET", "/api/v1/auth/me", req{token: admin})
	res := e.do(t, "PATCH", "/api/v1/users/"+me.str("id"), req{token: admin, body: `{"disabled":true}`})
	if res.StatusCode != 400 {
		t.Fatalf("got %d %v", res.StatusCode, res.body)
	}
	if res := e.do(t, "GET", apiClusters, req{token: admin}); res.StatusCode != 200 {
		t.Fatalf("the admin must still be active: %d", res.StatusCode)
	}
}

func TestResetUserPassword(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	id := e.newUser(t, admin, "ayse")
	e.userToken(t, "ayse") // finish the first-login password change so the old password is a real one
	reset := func(userID, body string) result {
		return e.do(t, "POST", "/api/v1/users/"+userID+"/reset-password", req{token: admin, body: body})
	}

	res := reset(id, `{}`)
	if res.StatusCode != 200 || len(res.str("temporaryPassword")) != 20 || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("reset: %d %v", res.StatusCode, res.body)
	}
	pw := res.str("temporaryPassword")
	login := e.do(t, "POST", "/api/v1/auth/login", req{body: fmt.Sprintf(`{"username":"ayse","password":%q}`, pw)})
	if login.StatusCode != 200 {
		t.Fatalf("the new password must work: %d", login.StatusCode)
	}
	if user, _ := login.body["user"].(map[string]any); user["mustChangePassword"] != true {
		t.Fatal("a reset password must be changed at next login")
	}
	if res := e.do(t, "POST", "/api/v1/auth/login", req{body: fmt.Sprintf(`{"username":"ayse","password":%q}`, userPass+"-changed")}); res.StatusCode != 401 {
		t.Fatalf("the old password must stop working: %d", res.StatusCode)
	}

	if res := reset(id, `{"password":"short"}`); res.StatusCode != 400 {
		t.Fatalf("weak password: %d", res.StatusCode)
	}
	if res := reset("missing", `{}`); res.StatusCode != 404 {
		t.Fatalf("unknown user: %d", res.StatusCode)
	}
	me := e.do(t, "GET", "/api/v1/auth/me", req{token: admin})
	if res := reset(me.str("id"), `{}`); res.StatusCode != 400 {
		t.Fatalf("an admin must use the change-password endpoint for themselves: %d", res.StatusCode)
	}
}

func TestRoleBindingAPI(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	uid := e.newUser(t, admin, "ayse")
	cluster := e.newCluster(t, admin, "alpha")

	t.Run("roles", func(t *testing.T) {
		res := e.do(t, "GET", "/api/v1/roles", req{token: admin})
		if res.StatusCode != 200 || len(itemIDs(res)) != 5 {
			t.Fatalf("got %d %s", res.StatusCode, res.raw)
		}
		if !strings.Contains(res.raw, `"permissions":["*"]`) {
			t.Fatalf("the admin role must list the wildcard: %s", res.raw)
		}
	})

	t.Run("a missing scope means all", func(t *testing.T) {
		res := e.grant(t, admin, uid, "viewer", "")
		if res.StatusCode != 201 || !strings.Contains(res.raw, `"scope":{"cluster":{"kind":"any"},"application":{"kind":"any"}}`) {
			t.Fatalf("got %d %s", res.StatusCode, res.raw)
		}
		me := e.do(t, "GET", "/api/v1/auth/me", req{token: admin})
		if res.body["createdBy"] != me.str("id") {
			t.Fatalf("createdBy must be the granting admin: %v", res.body)
		}
	})

	t.Run("duplicates are refused", func(t *testing.T) {
		if res := e.grant(t, admin, uid, "viewer", ""); res.StatusCode != 409 {
			t.Fatalf("got %d %v", res.StatusCode, res.body)
		}
		// The same role in another scope is a different grant.
		if res := e.grant(t, admin, uid, "viewer", clusterScope(cluster)); res.StatusCode != 201 {
			t.Fatalf("got %d %v", res.StatusCode, res.body)
		}
	})

	t.Run("invalid bindings", func(t *testing.T) {
		post := func(body string) result {
			return e.do(t, "POST", "/api/v1/role-bindings", req{token: admin, body: body})
		}
		sub := fmt.Sprintf(`{"kind":"user","id":%q}`, uid)
		for name, body := range map[string]string{
			"unknown role":      `{"subject":` + sub + `,"roleId":"superuser"}`,
			"unknown user":      `{"subject":{"kind":"user","id":"ghost"},"roleId":"viewer"}`,
			"unknown cluster":   `{"subject":` + sub + `,"roleId":"viewer","scope":` + clusterScope("nope") + `}`,
			"admin narrowed":    `{"subject":` + sub + `,"roleId":"admin","scope":` + clusterScope(cluster) + `}`,
			"unknown team":      `{"subject":{"kind":"team","id":"t1"},"roleId":"viewer"}`,
			"unknown group":     `{"subject":` + sub + `,"roleId":"viewer","scope":{"cluster":{"kind":"group","id":"g"}}}`,
			"application scope": `{"subject":` + sub + `,"roleId":"deployer","scope":{"application":{"kind":"id","id":"a"}}}`,
			"no subject":        `{"roleId":"viewer"}`,
			"any with an id":    `{"subject":` + sub + `,"roleId":"viewer","scope":{"cluster":{"kind":"any","id":"x"}}}`,
			"unknown field":     `{"subject":` + sub + `,"roleId":"viewer","scope":{},"extra":1}`,
			"malformed":         `{`,
		} {
			if res := post(body); res.StatusCode != 400 || res.errorCode() != "invalid_argument" {
				t.Errorf("%s: got %d %v, want 400 invalid_argument", name, res.StatusCode, res.body)
			}
		}
	})

	t.Run("list and filter", func(t *testing.T) {
		all := e.do(t, "GET", "/api/v1/role-bindings", req{token: admin})
		if len(itemIDs(all)) != 3 { // bootstrap admin + two for ayse
			t.Fatalf("all: %v", all.body)
		}
		mine := e.do(t, "GET", "/api/v1/role-bindings?subjectKind=user&subjectId="+uid, req{token: admin})
		if len(itemIDs(mine)) != 2 {
			t.Fatalf("filtered: %v", mine.body)
		}
		if res := e.do(t, "GET", "/api/v1/role-bindings?subjectKind=user", req{token: admin}); res.StatusCode != 400 {
			t.Fatalf("half a filter: %d", res.StatusCode)
		}
	})

	t.Run("delete", func(t *testing.T) {
		id := itemIDs(e.do(t, "GET", "/api/v1/role-bindings?subjectKind=user&subjectId="+uid, req{token: admin}))[0]
		if res := e.do(t, "DELETE", "/api/v1/role-bindings/"+id, req{token: admin}); res.StatusCode != 204 {
			t.Fatalf("got %d", res.StatusCode)
		}
		if res := e.do(t, "DELETE", "/api/v1/role-bindings/"+id, req{token: admin}); res.StatusCode != 404 {
			t.Fatalf("second delete: %d", res.StatusCode)
		}
	})
}

func TestLastAdministratorCannotBeRemoved(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	removeOwn := func(token string) result {
		return e.do(t, "DELETE", "/api/v1/role-bindings/bootstrap-admin", req{token: token})
	}

	// The only administrator cannot remove their own role.
	if res := removeOwn(admin); res.StatusCode != 409 || res.errorCode() != "last_admin" {
		t.Fatalf("got %d %v", res.StatusCode, res.body)
	}

	// A second administrator who is disabled is no fallback.
	id2 := e.newUser(t, admin, "second")
	if res := e.grant(t, admin, id2, "admin", ""); res.StatusCode != 201 {
		t.Fatalf("grant admin: %d %v", res.StatusCode, res.body)
	}
	if res := e.do(t, "PATCH", "/api/v1/users/"+id2, req{token: admin, body: `{"disabled":true}`}); res.StatusCode != 200 {
		t.Fatalf("disable second admin: %d %v", res.StatusCode, res.body)
	}
	if res := removeOwn(admin); res.StatusCode != 409 {
		t.Fatalf("a disabled administrator must not count as a fallback: %d", res.StatusCode)
	}

	// With an active second administrator it is fine.
	e.do(t, "PATCH", "/api/v1/users/"+id2, req{token: admin, body: `{"disabled":false}`})
	if res := removeOwn(admin); res.StatusCode != 204 {
		t.Fatalf("got %d %v", res.StatusCode, res.body)
	}
	// ... and the former admin is now an ordinary user.
	if res := e.do(t, "GET", "/api/v1/users", req{token: admin}); res.StatusCode != 403 {
		t.Fatalf("the former administrator must lose access: %d", res.StatusCode)
	}

	// The remaining administrator is now the last one.
	second := e.userToken(t, "second")
	secondBindings := e.do(t, "GET", "/api/v1/role-bindings?subjectKind=user&subjectId="+id2, req{token: second})
	bid := itemIDs(secondBindings)[0]
	if res := e.do(t, "DELETE", "/api/v1/role-bindings/"+bid, req{token: second}); res.StatusCode != 409 || res.errorCode() != "last_admin" {
		t.Fatalf("got %d %v", res.StatusCode, res.body)
	}
}
