package httpapi

import (
	"slices"
	"strings"
	"testing"
)

func (r result) strings(key string) []string {
	list, _ := r.body[key].([]any)
	out := make([]string, 0, len(list))
	for _, v := range list {
		out = append(out, v.(string))
	}
	return out
}

func meOf(t *testing.T, e *testEnv, token string) result {
	t.Helper()
	res := e.do(t, "GET", "/api/v1/auth/me", req{token: token})
	if res.StatusCode != 200 {
		t.Fatalf("me: %d %v", res.StatusCode, res.body)
	}
	return res
}

var allSystemPermissions = []string{"user:manage", "team:manage", "group:manage", "rbac:manage", "audit:view"}

func TestMeReportsWhatTheUserMayDo(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)

	t.Run("administrator", func(t *testing.T) {
		me := meOf(t, e, admin)
		if got := me.strings("permissions"); !slices.Equal(got, allSystemPermissions) {
			t.Fatalf("permissions = %v, want %v", got, allSystemPermissions)
		}
		if me.body["canCreateClusters"] != true {
			t.Fatalf("an administrator can create clusters: %v", me.body)
		}
		if !strings.Contains(me.raw, `"teams":[]`) {
			t.Fatalf("teams must be an empty list, not null: %s", me.raw)
		}
	})

	t.Run("a user without grants", func(t *testing.T) {
		e.newUser(t, admin, "plain")
		me := meOf(t, e, e.userToken(t, "plain"))
		if !strings.Contains(me.raw, `"permissions":[]`) || me.body["canCreateClusters"] != false {
			t.Fatalf("nothing granted, nothing offered: %s", me.raw)
		}
	})

	t.Run("cluster-admin over all clusters", func(t *testing.T) {
		id := e.newUser(t, admin, "ca-all")
		e.grant(t, admin, id, "cluster-admin", "")
		me := meOf(t, e, e.userToken(t, "ca-all"))
		if me.body["canCreateClusters"] != true || len(me.strings("permissions")) != 0 {
			t.Fatalf("can register clusters but holds no system permission: %v", me.body)
		}
	})

	t.Run("cluster-admin over one cluster cannot create clusters", func(t *testing.T) {
		c := e.newCluster(t, admin, "only-this")
		id := e.newUser(t, admin, "ca-one")
		e.grant(t, admin, id, "cluster-admin", clusterScope(c))
		me := meOf(t, e, e.userToken(t, "ca-one"))
		if me.body["canCreateClusters"] != false {
			t.Fatalf("a grant over one cluster must not offer cluster creation: %v", me.body)
		}
	})

	t.Run("teams are listed, and an admin team grants system permissions", func(t *testing.T) {
		uid := e.newUser(t, admin, "teamed")
		team := e.newTeam(t, admin, "Operations", uid)
		e.newTeam(t, admin, "Unrelated")
		tok := e.userToken(t, "teamed")

		me := meOf(t, e, tok)
		teams, _ := me.body["teams"].([]any)
		if len(teams) != 1 || teams[0].(map[string]any)["id"] != team || teams[0].(map[string]any)["name"] != "Operations" {
			t.Fatalf("teams = %v", me.body["teams"])
		}
		if len(me.strings("permissions")) != 0 {
			t.Fatalf("not yet an administrator: %v", me.body)
		}

		if res := e.grantTeam(t, admin, team, "admin", ""); res.StatusCode != 201 {
			t.Fatalf("grant: %d %v", res.StatusCode, res.body)
		}
		me = meOf(t, e, tok)
		if got := me.strings("permissions"); !slices.Equal(got, allSystemPermissions) {
			t.Fatalf("a member of an admin team holds the system permissions: %v", got)
		}
		if me.body["canCreateClusters"] != true {
			t.Fatalf("%v", me.body)
		}
	})
}

func TestSessionResponsesCarryThePermissionSummary(t *testing.T) {
	e := newEnv(t)
	res := e.login(t, adminPass)
	user, _ := res.body["user"].(map[string]any)
	perms, _ := user["permissions"].([]any)
	if len(perms) != len(allSystemPermissions) || user["canCreateClusters"] != true {
		t.Fatalf("login must carry the summary so the dashboard needs no extra call: %v", user)
	}
	if _, ok := user["teams"].([]any); !ok {
		t.Fatalf("login must carry the teams: %v", user)
	}
	for _, leak := range []string{"passwordHash", "argon2"} {
		if strings.Contains(res.raw, leak) {
			t.Fatalf("session response leaks %q", leak)
		}
	}
}

func TestClusterResponsesTellWhatTheUserMayDoWithEach(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	alpha := e.newCluster(t, admin, "alpha")
	beta := e.newCluster(t, admin, "beta")
	uid := e.newUser(t, admin, "ayse")
	e.grant(t, admin, uid, "viewer", "")
	e.grant(t, admin, uid, "cluster-admin", clusterScope(alpha))
	tok := e.userToken(t, "ayse")

	perms := func(res result, id string) []string {
		items, _ := res.body["items"].([]any)
		for _, it := range items {
			m := it.(map[string]any)
			if m["id"] == id {
				var out []string
				for _, p := range m["permissions"].([]any) {
					out = append(out, p.(string))
				}
				return out
			}
		}
		t.Fatalf("cluster %s not in %v", id, res.body)
		return nil
	}

	list := e.do(t, "GET", apiClusters, req{token: tok})
	if got := perms(list, alpha); !slices.Equal(got, []string{"cluster:view", "cluster:manage"}) {
		t.Fatalf("alpha: %v", got)
	}
	if got := perms(list, beta); !slices.Equal(got, []string{"cluster:view"}) {
		t.Fatalf("beta: %v", got)
	}

	one := e.do(t, "GET", apiClusters+"/"+beta, req{token: tok})
	if got := one.strings("permissions"); !slices.Equal(got, []string{"cluster:view"}) {
		t.Fatalf("get beta: %v", got)
	}
	if one.str("name") != "beta" || one.str("apiServer") == "" {
		t.Fatalf("the cluster's own fields must still be at the top level: %v", one.body)
	}

	adminList := e.do(t, "GET", apiClusters, req{token: admin})
	if got := perms(adminList, beta); !slices.Equal(got, []string{"cluster:view", "cluster:manage"}) {
		t.Fatalf("admin on beta: %v", got)
	}

	created := e.do(t, "POST", apiClusters, req{token: admin, body: `{"name":"gamma","apiServer":"https://g:6443"}`})
	if got := created.strings("permissions"); !slices.Equal(got, []string{"cluster:view", "cluster:manage"}) {
		t.Fatalf("create response: %v", got)
	}
}
