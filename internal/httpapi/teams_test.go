package httpapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func jsonList(ids ...string) string {
	if ids == nil {
		ids = []string{}
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

func (e *testEnv) newTeam(t *testing.T, adminToken, name string, members ...string) string {
	t.Helper()
	res := e.do(t, "POST", "/api/v1/teams", req{token: adminToken, body: fmt.Sprintf(`{"name":%q,"members":%s}`, name, jsonList(members...))})
	if res.StatusCode != 201 {
		t.Fatalf("create team %s: %d %v", name, res.StatusCode, res.body)
	}
	return res.str("id")
}

func (e *testEnv) newGroup(t *testing.T, adminToken, name string, members ...string) string {
	t.Helper()
	res := e.do(t, "POST", "/api/v1/resource-groups", req{token: adminToken, body: fmt.Sprintf(`{"kind":"cluster","name":%q,"members":%s}`, name, jsonList(members...))})
	if res.StatusCode != 201 {
		t.Fatalf("create group %s: %d %v", name, res.StatusCode, res.body)
	}
	return res.str("id")
}

func (e *testEnv) grantTeam(t *testing.T, adminToken, teamID, role, scopeJSON string) result {
	t.Helper()
	body := fmt.Sprintf(`{"subject":{"kind":"team","id":%q},"roleId":%q`, teamID, role)
	if scopeJSON != "" {
		body += `,"scope":` + scopeJSON
	}
	return e.do(t, "POST", "/api/v1/role-bindings", req{token: adminToken, body: body + `}`})
}

func groupScopeJSON(id string) string {
	return fmt.Sprintf(`{"cluster":{"kind":"group","id":%q}}`, id)
}

func members(res result) []string {
	list, _ := res.body["members"].([]any)
	out := make([]string, 0, len(list))
	for _, m := range list {
		out = append(out, m.(string))
	}
	return out
}

func TestTeamsCRUD(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	ayse := e.newUser(t, admin, "ayse")
	mehmet := e.newUser(t, admin, "mehmet")
	post := func(body string) result { return e.do(t, "POST", "/api/v1/teams", req{token: admin, body: body}) }

	res := post(fmt.Sprintf(`{"name":"Payments","description":"Ödeme ekibi","members":%s}`, jsonList(mehmet, ayse, ayse)))
	if res.StatusCode != 201 || res.str("name") != "Payments" || res.str("description") != "Ödeme ekibi" {
		t.Fatalf("create: %d %v", res.StatusCode, res.body)
	}
	if m := members(res); len(m) != 2 {
		t.Fatalf("members must be de-duplicated: %v", m)
	}
	id := res.str("id")
	me := e.do(t, "GET", "/api/v1/auth/me", req{token: admin})
	if res.body["createdBy"] != me.str("id") {
		t.Fatalf("createdBy must be the creating admin: %v", res.body)
	}

	t.Run("a team may start empty", func(t *testing.T) {
		res := post(`{"name":"Empty"}`)
		if res.StatusCode != 201 || !strings.Contains(res.raw, `"members":[]`) {
			t.Fatalf("got %d %s", res.StatusCode, res.raw)
		}
	})

	t.Run("validation", func(t *testing.T) {
		for name, tt := range map[string]struct {
			body string
			want int
		}{
			"empty name":      {`{"name":""}`, 400},
			"long name":       {`{"name":"` + strings.Repeat("a", 65) + `"}`, 400},
			"padded name":     {`{"name":" x "}`, 400},
			"long desc":       {`{"name":"x","description":"` + strings.Repeat("d", 501) + `"}`, 400},
			"unknown member":  {`{"name":"x","members":["ghost"]}`, 400},
			"empty member id": {`{"name":"x","members":[""]}`, 400},
			"unknown field":   {`{"name":"x","admin":true}`, 400},
			"malformed":       {`{`, 400},
			"duplicate name":  {`{"name":"PAYMENTS"}`, 409},
		} {
			if res := post(tt.body); res.StatusCode != tt.want {
				t.Errorf("%s: got %d %v, want %d", name, res.StatusCode, res.body, tt.want)
			}
		}
	})

	t.Run("list is sorted by name ignoring case", func(t *testing.T) {
		post(`{"name":"alpha"}`)
		res := e.do(t, "GET", "/api/v1/teams", req{token: admin})
		var names []string
		for _, it := range res.body["items"].([]any) {
			names = append(names, it.(map[string]any)["name"].(string))
		}
		if strings.Join(names, ",") != "alpha,Empty,Payments" {
			t.Fatalf("got %v", names)
		}
	})

	t.Run("get and update", func(t *testing.T) {
		if res := e.do(t, "GET", "/api/v1/teams/"+id, req{token: admin}); res.StatusCode != 200 || res.str("name") != "Payments" {
			t.Fatalf("get: %d %v", res.StatusCode, res.body)
		}
		patch := func(body string) result { return e.do(t, "PATCH", "/api/v1/teams/"+id, req{token: admin, body: body}) }
		if res := patch(`{"name":"Payments & Billing","description":""}`); res.StatusCode != 200 || res.str("name") != "Payments & Billing" || res.str("description") != "" || len(members(res)) != 2 {
			t.Fatalf("rename keeps members: %d %v", res.StatusCode, res.body)
		}
		if res := patch(`{"name":"ALPHA"}`); res.StatusCode != 409 {
			t.Fatalf("rename to a taken name: %d", res.StatusCode)
		}
		if res := patch(`{"name":""}`); res.StatusCode != 400 {
			t.Fatalf("empty name: %d", res.StatusCode)
		}
		if res := patch(`{"members":[]}`); res.StatusCode != 400 {
			t.Fatalf("members cannot be changed through PATCH: %d", res.StatusCode)
		}
		if res := e.do(t, "PATCH", "/api/v1/teams/missing", req{token: admin, body: `{"name":"x"}`}); res.StatusCode != 404 {
			t.Fatalf("unknown team: %d", res.StatusCode)
		}
		if res := e.do(t, "GET", "/api/v1/teams/missing", req{token: admin}); res.StatusCode != 404 {
			t.Fatalf("unknown team: %d", res.StatusCode)
		}
	})

	t.Run("delete", func(t *testing.T) {
		if res := e.do(t, "DELETE", "/api/v1/teams/"+id, req{token: admin}); res.StatusCode != 204 {
			t.Fatalf("got %d", res.StatusCode)
		}
		if res := e.do(t, "DELETE", "/api/v1/teams/"+id, req{token: admin}); res.StatusCode != 404 {
			t.Fatalf("second delete: %d", res.StatusCode)
		}
	})
}

func TestTeamMembers(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	ayse := e.newUser(t, admin, "ayse")
	mehmet := e.newUser(t, admin, "mehmet")
	team := e.newTeam(t, admin, "payments", ayse)
	put := func(id, body string) result {
		return e.do(t, "PUT", "/api/v1/teams/"+id+"/members", req{token: admin, body: body})
	}

	if res := put(team, fmt.Sprintf(`{"members":%s}`, jsonList(mehmet, ayse, mehmet))); res.StatusCode != 200 || len(members(res)) != 2 {
		t.Fatalf("replace: %d %v", res.StatusCode, res.body)
	}
	if res := put(team, `{"members":[]}`); res.StatusCode != 200 || len(members(res)) != 0 || !strings.Contains(res.raw, `"members":[]`) {
		t.Fatalf("clear: %d %s", res.StatusCode, res.raw)
	}
	for name, body := range map[string]string{
		"field required": `{}`,
		"null":           `{"members":null}`,
		"unknown user":   `{"members":["ghost"]}`,
		"empty id":       `{"members":[""]}`,
		"unknown field":  `{"members":[],"extra":1}`,
		"wrong type":     `{"members":"ayse"}`,
	} {
		if res := put(team, body); res.StatusCode != 400 {
			t.Errorf("%s: got %d %v, want 400", name, res.StatusCode, res.body)
		}
	}
	// Nothing above may have changed the team.
	if res := e.do(t, "GET", "/api/v1/teams/"+team, req{token: admin}); len(members(res)) != 0 {
		t.Fatalf("rejected requests must not change the team: %v", res.body)
	}
	if res := put("missing", `{"members":[]}`); res.StatusCode != 404 {
		t.Fatalf("unknown team: %d", res.StatusCode)
	}
}

func TestTeamGrantsAccessToItsMembers(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	prod1 := e.newCluster(t, admin, "prod-1")
	prod2 := e.newCluster(t, admin, "prod-2")
	dev := e.newCluster(t, admin, "dev")
	ayse := e.newUser(t, admin, "ayse")
	e.newUser(t, admin, "outsider")
	prodGroup := e.newGroup(t, admin, "prod", prod1, prod2)
	team := e.newTeam(t, admin, "payments", ayse)
	if res := e.grantTeam(t, admin, team, "viewer", groupScopeJSON(prodGroup)); res.StatusCode != 201 {
		t.Fatalf("grant: %d %v", res.StatusCode, res.body)
	}
	aysetok := e.userToken(t, "ayse")
	outsider := e.userToken(t, "outsider")
	list := func(tok string) []string { return itemIDs(e.do(t, "GET", apiClusters, req{token: tok})) }

	if got := list(aysetok); !sameSet(got, []string{prod1, prod2}) {
		t.Fatalf("a team member sees exactly the clusters of the group: %v", got)
	}
	if res := e.do(t, "GET", apiClusters+"/"+dev, req{token: aysetok}); res.StatusCode != 404 {
		t.Fatalf("a cluster outside the group is hidden: %d", res.StatusCode)
	}
	if res := e.do(t, "GET", apiClusters+"/"+prod1, req{token: aysetok}); res.StatusCode != 200 {
		t.Fatalf("a cluster in the group is visible: %d", res.StatusCode)
	}
	if got := list(outsider); len(got) != 0 {
		t.Fatalf("a non-member sees nothing: %v", got)
	}

	t.Run("group membership changes apply immediately", func(t *testing.T) {
		put := e.do(t, "PUT", "/api/v1/resource-groups/"+prodGroup+"/members", req{token: admin, body: fmt.Sprintf(`{"members":%s}`, jsonList(prod1, dev))})
		if put.StatusCode != 200 {
			t.Fatalf("%d %v", put.StatusCode, put.body)
		}
		if got := list(aysetok); !sameSet(got, []string{prod1, dev}) {
			t.Fatalf("after the group changed: %v", got)
		}
	})

	t.Run("team membership changes apply immediately", func(t *testing.T) {
		out := e.userID(t, admin, "outsider")
		if res := e.do(t, "PUT", "/api/v1/teams/"+team+"/members", req{token: admin, body: fmt.Sprintf(`{"members":%s}`, jsonList(ayse, out))}); res.StatusCode != 200 {
			t.Fatalf("%d %v", res.StatusCode, res.body)
		}
		if got := list(outsider); len(got) != 2 {
			t.Fatalf("a new member gets the team's access at once: %v", got)
		}
		if res := e.do(t, "PUT", "/api/v1/teams/"+team+"/members", req{token: admin, body: fmt.Sprintf(`{"members":%s}`, jsonList(ayse))}); res.StatusCode != 200 {
			t.Fatalf("%d", res.StatusCode)
		}
		if got := list(outsider); len(got) != 0 {
			t.Fatalf("a removed member loses it at once: %v", got)
		}
	})

	t.Run("a user's own grants and the team's add up", func(t *testing.T) {
		if res := e.grant(t, admin, ayse, "cluster-admin", clusterScope(prod2)); res.StatusCode != 201 {
			t.Fatalf("%d %v", res.StatusCode, res.body)
		}
		// prod-2 left the group above, but her own cluster-admin grant still covers it.
		if got := list(aysetok); !sameSet(got, []string{prod1, dev, prod2}) {
			t.Fatalf("got %v", got)
		}
		if res := e.do(t, "DELETE", apiClusters+"/"+prod2, req{token: aysetok}); res.StatusCode != 204 {
			t.Fatalf("her own cluster-admin grant lets her delete prod-2: %d", res.StatusCode)
		}
		if res := e.do(t, "DELETE", apiClusters+"/"+prod1, req{token: aysetok}); res.StatusCode != 403 {
			t.Fatalf("the team's viewer grant does not allow deleting: %d", res.StatusCode)
		}
	})
}

func (e *testEnv) userID(t *testing.T, adminToken, username string) string {
	t.Helper()
	u, err := e.store.GetUserByUsername(t.Context(), username)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestDeletingTeamsAndGroupsRemovesTheirGrants(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	c := e.newCluster(t, admin, "alpha")
	u := e.newUser(t, admin, "ayse")
	team := e.newTeam(t, admin, "payments", u)
	group := e.newGroup(t, admin, "prod", c)
	tok := e.userToken(t, "ayse")

	// Two grants: the team on all clusters, and the user on the group.
	teamGrant := e.grantTeam(t, admin, team, "viewer", "")
	userGrant := e.grant(t, admin, u, "viewer", groupScopeJSON(group))
	if teamGrant.StatusCode != 201 || userGrant.StatusCode != 201 {
		t.Fatalf("grants: %d %d", teamGrant.StatusCode, userGrant.StatusCode)
	}
	bindingCount := func() int {
		return len(itemIDs(e.do(t, "GET", "/api/v1/role-bindings", req{token: admin})))
	}
	before := bindingCount()

	if res := e.do(t, "DELETE", "/api/v1/resource-groups/"+group, req{token: admin}); res.StatusCode != 204 {
		t.Fatalf("delete group: %d", res.StatusCode)
	}
	if bindingCount() != before-1 {
		t.Fatalf("the grant on the deleted group must be removed")
	}
	if got := itemIDs(e.do(t, "GET", apiClusters, req{token: tok})); len(got) != 1 {
		t.Fatalf("the team's own grant still applies: %v", got)
	}

	if res := e.do(t, "DELETE", "/api/v1/teams/"+team, req{token: admin}); res.StatusCode != 204 {
		t.Fatalf("delete team: %d", res.StatusCode)
	}
	if bindingCount() != before-2 {
		t.Fatalf("the grants of the deleted team must be removed")
	}
	if got := itemIDs(e.do(t, "GET", apiClusters, req{token: tok})); len(got) != 0 {
		t.Fatalf("a deleted team grants nothing: %v", got)
	}
}

func TestGroupsCRUD(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	c1 := e.newCluster(t, admin, "c1")
	c2 := e.newCluster(t, admin, "c2")
	post := func(body string) result {
		return e.do(t, "POST", "/api/v1/resource-groups", req{token: admin, body: body})
	}

	res := post(fmt.Sprintf(`{"kind":"cluster","name":"Prod","description":"production","members":%s}`, jsonList(c2, c1, c1)))
	if res.StatusCode != 201 || res.str("kind") != "cluster" || len(members(res)) != 2 {
		t.Fatalf("create: %d %v", res.StatusCode, res.body)
	}
	id := res.str("id")

	t.Run("validation", func(t *testing.T) {
		for name, tt := range map[string]struct {
			body string
			want int
		}{
			"missing kind":                {`{"name":"x"}`, 400},
			"unknown kind":                {`{"kind":"nodes","name":"x"}`, 400},
			"application groups: not yet": {`{"kind":"application","name":"x"}`, 400},
			"empty name":                  {`{"kind":"cluster","name":""}`, 400},
			"unknown cluster member":      {`{"kind":"cluster","name":"x","members":["nope"]}`, 400},
			"unknown field":               {`{"kind":"cluster","name":"x","extra":1}`, 400},
			"duplicate, ignoring case":    {`{"kind":"cluster","name":"PROD"}`, 409},
		} {
			if res := post(tt.body); res.StatusCode != tt.want {
				t.Errorf("%s: got %d %v, want %d", name, res.StatusCode, res.body, tt.want)
			}
		}
	})

	t.Run("list and filter", func(t *testing.T) {
		post(`{"kind":"cluster","name":"dev"}`)
		all := e.do(t, "GET", "/api/v1/resource-groups", req{token: admin})
		if len(itemIDs(all)) != 2 {
			t.Fatalf("all: %v", all.body)
		}
		if res := e.do(t, "GET", "/api/v1/resource-groups?kind=cluster", req{token: admin}); len(itemIDs(res)) != 2 {
			t.Fatalf("clusters: %v", res.body)
		}
		if res := e.do(t, "GET", "/api/v1/resource-groups?kind=application", req{token: admin}); res.StatusCode != 200 || len(itemIDs(res)) != 0 {
			t.Fatalf("applications: %d %v", res.StatusCode, res.body)
		}
		if res := e.do(t, "GET", "/api/v1/resource-groups?kind=nodes", req{token: admin}); res.StatusCode != 400 {
			t.Fatalf("bad kind filter: %d", res.StatusCode)
		}
	})

	t.Run("update", func(t *testing.T) {
		patch := func(body string) result {
			return e.do(t, "PATCH", "/api/v1/resource-groups/"+id, req{token: admin, body: body})
		}
		if res := patch(`{"name":"Production"}`); res.StatusCode != 200 || res.str("name") != "Production" || len(members(res)) != 2 || res.str("kind") != "cluster" {
			t.Fatalf("rename: %d %v", res.StatusCode, res.body)
		}
		if res := patch(`{"name":"DEV"}`); res.StatusCode != 409 {
			t.Fatalf("taken name: %d", res.StatusCode)
		}
		if res := patch(`{"kind":"application"}`); res.StatusCode != 400 {
			t.Fatalf("the kind cannot be changed: %d", res.StatusCode)
		}
		if res := e.do(t, "PATCH", "/api/v1/resource-groups/missing", req{token: admin, body: `{"name":"x"}`}); res.StatusCode != 404 {
			t.Fatalf("unknown group: %d", res.StatusCode)
		}
	})

	t.Run("members", func(t *testing.T) {
		put := func(gid, body string) result {
			return e.do(t, "PUT", "/api/v1/resource-groups/"+gid+"/members", req{token: admin, body: body})
		}
		if res := put(id, fmt.Sprintf(`{"members":%s}`, jsonList(c1))); res.StatusCode != 200 || len(members(res)) != 1 {
			t.Fatalf("replace: %d %v", res.StatusCode, res.body)
		}
		if res := put(id, `{"members":[]}`); res.StatusCode != 200 || !strings.Contains(res.raw, `"members":[]`) {
			t.Fatalf("clear: %d %s", res.StatusCode, res.raw)
		}
		for name, body := range map[string]string{"required": `{}`, "unknown cluster": `{"members":["nope"]}`} {
			if res := put(id, body); res.StatusCode != 400 {
				t.Errorf("%s: got %d, want 400", name, res.StatusCode)
			}
		}
		if res := put("missing", `{"members":[]}`); res.StatusCode != 404 {
			t.Fatalf("unknown group: %d", res.StatusCode)
		}
	})

	t.Run("delete", func(t *testing.T) {
		if res := e.do(t, "DELETE", "/api/v1/resource-groups/"+id, req{token: admin}); res.StatusCode != 204 {
			t.Fatalf("got %d", res.StatusCode)
		}
		if res := e.do(t, "GET", "/api/v1/resource-groups/"+id, req{token: admin}); res.StatusCode != 404 {
			t.Fatalf("get after delete: %d", res.StatusCode)
		}
	})
}

func TestDeletingAClusterRemovesItFromItsGroups(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	c1 := e.newCluster(t, admin, "c1")
	c2 := e.newCluster(t, admin, "c2")
	g := e.newGroup(t, admin, "all", c1, c2)

	if res := e.do(t, "DELETE", apiClusters+"/"+c1, req{token: admin}); res.StatusCode != 204 {
		t.Fatalf("delete cluster: %d", res.StatusCode)
	}
	res := e.do(t, "GET", "/api/v1/resource-groups/"+g, req{token: admin})
	if m := members(res); len(m) != 1 || m[0] != c2 {
		t.Fatalf("the deleted cluster must leave the group: %v", m)
	}
}

func TestBindingsWithTeamsAndGroups(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	c := e.newCluster(t, admin, "alpha")
	team := e.newTeam(t, admin, "payments")
	group := e.newGroup(t, admin, "prod", c)

	if res := e.grantTeam(t, admin, team, "deployer", groupScopeJSON(group)); res.StatusCode != 201 {
		t.Fatalf("a team on a cluster group: %d %v", res.StatusCode, res.body)
	}
	if res := e.grantTeam(t, admin, team, "deployer", groupScopeJSON(group)); res.StatusCode != 409 {
		t.Fatalf("duplicate: %d", res.StatusCode)
	}
	list := e.do(t, "GET", "/api/v1/role-bindings?subjectKind=team&subjectId="+team, req{token: admin})
	if len(itemIDs(list)) != 1 {
		t.Fatalf("the filter works for teams: %v", list.body)
	}

	// An application group is not a valid cluster scope, and application scopes stay unavailable.
	appGroupBody := fmt.Sprintf(`{"subject":{"kind":"team","id":%q},"roleId":"viewer","scope":{"application":{"kind":"group","id":%q}}}`, team, group)
	for name, body := range map[string]string{
		"application scope not available yet": appGroupBody,
		"unknown team":                        `{"subject":{"kind":"team","id":"nope"},"roleId":"viewer"}`,
		"admin for a team cannot be narrowed": fmt.Sprintf(`{"subject":{"kind":"team","id":%q},"roleId":"admin","scope":%s}`, team, groupScopeJSON(group)),
	} {
		if res := e.do(t, "POST", "/api/v1/role-bindings", req{token: admin, body: body}); res.StatusCode != 400 {
			t.Errorf("%s: got %d %v, want 400", name, res.StatusCode, res.body)
		}
	}
}

func TestAdminRoleThroughATeam(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	deputy := e.newUser(t, admin, "deputy")
	other := e.newUser(t, admin, "other")
	gone := e.newUser(t, admin, "gone")
	ops := e.newTeam(t, admin, "ops", deputy)

	res := e.grantTeam(t, admin, ops, "admin", "")
	if res.StatusCode != 201 {
		t.Fatalf("grant admin to the team: %d %v", res.StatusCode, res.body)
	}
	teamBinding := res.str("id")
	deputyTok := e.userToken(t, "deputy")

	if res := e.do(t, "GET", "/api/v1/users", req{token: deputyTok}); res.StatusCode != 200 {
		t.Fatalf("a member of an admin team has system permissions: %d", res.StatusCode)
	}

	// The original administrator steps down: the team's member is a fallback, so this is allowed.
	if res := e.do(t, "DELETE", "/api/v1/role-bindings/bootstrap-admin", req{token: admin}); res.StatusCode != 204 {
		t.Fatalf("with a team administrator in place the bootstrap grant can go: %d %v", res.StatusCode, res.body)
	}
	if res := e.do(t, "GET", "/api/v1/users", req{token: admin}); res.StatusCode != 403 {
		t.Fatalf("the former administrator lost access: %d", res.StatusCode)
	}

	// Deputy is now the only administrator, and only through the team. A disabled user is not a
	// replacement, so every way of removing deputy's admin rights is refused.
	if res := e.do(t, "PATCH", "/api/v1/users/"+gone, req{token: deputyTok, body: `{"disabled":true}`}); res.StatusCode != 200 {
		t.Fatalf("disable the spare user: %d %v", res.StatusCode, res.body)
	}
	refused := func(name string, res result) {
		t.Helper()
		if res.StatusCode != 409 || res.errorCode() != "last_admin" {
			t.Errorf("%s: got %d %v, want 409 last_admin", name, res.StatusCode, res.body)
		}
	}
	setMembers := func(tok string, ids ...string) result {
		return e.do(t, "PUT", "/api/v1/teams/"+ops+"/members", req{token: tok, body: fmt.Sprintf(`{"members":%s}`, jsonList(ids...))})
	}
	refused("emptying the team", setMembers(deputyTok))
	refused("swapping deputy for a disabled user", setMembers(deputyTok, gone))
	refused("deleting the team", e.do(t, "DELETE", "/api/v1/teams/"+ops, req{token: deputyTok}))
	refused("deleting the team's admin binding", e.do(t, "DELETE", "/api/v1/role-bindings/"+teamBinding, req{token: deputyTok}))
	if res := e.do(t, "GET", "/api/v1/teams/"+ops, req{token: deputyTok}); res.StatusCode != 200 || len(members(res)) != 1 {
		t.Fatalf("refused requests must leave the team untouched: %v", res.body)
	}

	// Adding an active administrator is fine; then swapping the first one out is fine too.
	if res := setMembers(deputyTok, deputy, other); res.StatusCode != 200 {
		t.Fatalf("adding a member: %d %v", res.StatusCode, res.body)
	}
	if res := setMembers(deputyTok, other); res.StatusCode != 200 {
		t.Fatalf("with another active administrator, deputy may leave the team: %d %v", res.StatusCode, res.body)
	}
	if res := e.do(t, "GET", "/api/v1/users", req{token: deputyTok}); res.StatusCode != 403 {
		t.Fatalf("deputy left the admin team and must lose system permissions: %d", res.StatusCode)
	}
	otherTok := e.userToken(t, "other")
	if res := e.do(t, "GET", "/api/v1/users", req{token: otherTok}); res.StatusCode != 200 {
		t.Fatalf("the remaining team member is the administrator now: %d", res.StatusCode)
	}
}

func TestTeamAndGroupEndpointsRequireTheirPermission(t *testing.T) {
	e := newEnv(t)
	admin := e.admin(t)
	uid := e.newUser(t, admin, "ayse")
	tok := e.userToken(t, "ayse")
	// The strongest non-admin roles carry no system permission.
	for _, role := range []string{"maintainer", "cluster-admin"} {
		if res := e.grant(t, admin, uid, role, ""); res.StatusCode != 201 {
			t.Fatalf("grant %s: %d", role, res.StatusCode)
		}
	}
	c := e.newCluster(t, admin, "alpha")
	team := e.newTeam(t, admin, "payments")
	group := e.newGroup(t, admin, "prod")

	tests := []struct{ method, path, body string }{
		{"GET", "/api/v1/teams", ""},
		{"POST", "/api/v1/teams", `{"name":"x"}`},
		{"GET", "/api/v1/teams/" + team, ""},
		{"PATCH", "/api/v1/teams/" + team, `{"name":"x"}`},
		{"DELETE", "/api/v1/teams/" + team, ""},
		{"PUT", "/api/v1/teams/" + team + "/members", fmt.Sprintf(`{"members":%s}`, jsonList(uid))},
		{"GET", "/api/v1/resource-groups", ""},
		{"POST", "/api/v1/resource-groups", `{"kind":"cluster","name":"x"}`},
		{"GET", "/api/v1/resource-groups/" + group, ""},
		{"PATCH", "/api/v1/resource-groups/" + group, `{"name":"x"}`},
		{"DELETE", "/api/v1/resource-groups/" + group, ""},
		{"PUT", "/api/v1/resource-groups/" + group + "/members", fmt.Sprintf(`{"members":%s}`, jsonList(c))},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			res := e.do(t, tt.method, tt.path, req{token: tok, body: tt.body})
			if res.StatusCode != 403 || res.errorCode() != "forbidden" {
				t.Fatalf("got %d %v, want 403 forbidden", res.StatusCode, res.body)
			}
		})
	}
	// None of it may have had an effect.
	if res := e.do(t, "GET", "/api/v1/teams/"+team, req{token: admin}); res.str("name") != "payments" || len(members(res)) != 0 {
		t.Fatalf("the team changed: %v", res.body)
	}
	if res := e.do(t, "GET", "/api/v1/resource-groups/"+group, req{token: admin}); res.str("name") != "prod" || len(members(res)) != 0 {
		t.Fatalf("the group changed: %v", res.body)
	}
}
