package authz

import (
	"testing"

	"github.com/magicorntech/Fluxor/internal/domain"
)

type fakeGroup struct {
	kind    domain.GroupKind
	members []string
}

type fakeGroups map[string]fakeGroup

func (g fakeGroups) Contains(kind domain.GroupKind, groupID, resourceID string) bool {
	grp, ok := g[groupID]
	if !ok || grp.kind != kind {
		return false
	}
	for _, id := range grp.members {
		if id == resourceID {
			return true
		}
	}
	return false
}

func bind(role string, cluster, app domain.Ref) domain.Binding {
	return domain.Binding{
		Subject: domain.Subject{Kind: domain.SubjectUser, ID: "u"},
		RoleID:  role,
		Scope:   domain.Scope{Cluster: cluster, Application: app},
	}
}

func id(v string) domain.Ref    { return domain.Ref{Kind: domain.RefID, ID: v} }
func group(v string) domain.Ref { return domain.Ref{Kind: domain.RefGroup, ID: v} }

var anyRef = domain.AnyRef()

func TestCan(t *testing.T) {
	groups := fakeGroups{
		"prod":     {domain.GroupKindCluster, []string{"c-prod-1", "c-prod-2"}},
		"payments": {domain.GroupKindApplication, []string{"a-pay-api", "a-pay-worker"}},
	}

	tests := []struct {
		name     string
		bindings []domain.Binding
		perm     Permission
		target   Target
		want     bool
	}{
		// --- no grants ---
		{"no bindings", nil, ClusterView, Target{ClusterID: "c1"}, false},
		{"unknown permission", []domain.Binding{bind(RoleAdmin, anyRef, anyRef)}, "cluster:fly", Target{ClusterID: "c1"}, false},
		{"unknown role grants nothing", []domain.Binding{bind("superuser", anyRef, anyRef)}, ClusterView, Target{ClusterID: "c1"}, false},

		// --- admin wildcard ---
		{"admin: cluster", []domain.Binding{bind(RoleAdmin, anyRef, anyRef)}, ClusterManage, Target{ClusterID: "c1"}, true},
		{"admin: app", []domain.Binding{bind(RoleAdmin, anyRef, anyRef)}, AppSync, Target{AppID: "a1", ClusterID: "c1"}, true},
		{"admin: system", []domain.Binding{bind(RoleAdmin, anyRef, anyRef)}, UserManage, Target{}, true},
		{"admin: create a new cluster", []domain.Binding{bind(RoleAdmin, anyRef, anyRef)}, ClusterManage, Target{}, true},
		{"admin narrowed to one cluster gets no system permission", []domain.Binding{bind(RoleAdmin, id("c1"), anyRef)}, UserManage, Target{}, false},
		{"admin narrowed still manages that cluster", []domain.Binding{bind(RoleAdmin, id("c1"), anyRef)}, ClusterManage, Target{ClusterID: "c1"}, true},
		{"admin narrowed does not manage others", []domain.Binding{bind(RoleAdmin, id("c1"), anyRef)}, ClusterManage, Target{ClusterID: "c2"}, false},

		// --- cluster permissions look at the cluster dimension only ---
		{"viewer on one cluster: that cluster", []domain.Binding{bind(RoleViewer, id("c1"), anyRef)}, ClusterView, Target{ClusterID: "c1"}, true},
		{"viewer on one cluster: another cluster", []domain.Binding{bind(RoleViewer, id("c1"), anyRef)}, ClusterView, Target{ClusterID: "c2"}, false},
		{"viewer cannot manage", []domain.Binding{bind(RoleViewer, anyRef, anyRef)}, ClusterManage, Target{ClusterID: "c1"}, false},
		{"cluster-admin on one cluster manages it", []domain.Binding{bind(RoleClusterAdmin, id("c1"), anyRef)}, ClusterManage, Target{ClusterID: "c1"}, true},
		{"cluster-admin on one cluster cannot register a new one", []domain.Binding{bind(RoleClusterAdmin, id("c1"), anyRef)}, ClusterManage, Target{}, false},
		{"cluster-admin on all clusters can register a new one", []domain.Binding{bind(RoleClusterAdmin, anyRef, anyRef)}, ClusterManage, Target{}, true},
		{"cluster permission ignores the application dimension", []domain.Binding{bind(RoleViewer, id("c1"), id("a1"))}, ClusterView, Target{ClusterID: "c1"}, true},
		{"cluster group: member", []domain.Binding{bind(RoleViewer, group("prod"), anyRef)}, ClusterView, Target{ClusterID: "c-prod-2"}, true},
		{"cluster group: non-member", []domain.Binding{bind(RoleViewer, group("prod"), anyRef)}, ClusterView, Target{ClusterID: "c-dev"}, false},
		{"cluster group: unknown group", []domain.Binding{bind(RoleViewer, group("nope"), anyRef)}, ClusterView, Target{ClusterID: "c-prod-1"}, false},
		{"an application group never grants access to clusters", []domain.Binding{bind(RoleViewer, group("payments"), anyRef)}, ClusterView, Target{ClusterID: "a-pay-api"}, false},
		{"a cluster group never grants access to applications", []domain.Binding{bind(RoleDeployer, anyRef, group("prod"))}, AppSync, Target{AppID: "c-prod-1", ClusterID: "c1"}, false},
		{"cluster group never covers a not-yet-existing cluster", []domain.Binding{bind(RoleClusterAdmin, group("prod"), anyRef)}, ClusterManage, Target{}, false},

		// --- application permissions need both dimensions ---
		{"deployer app+cluster both match", []domain.Binding{bind(RoleDeployer, id("c1"), id("a1"))}, AppSync, Target{AppID: "a1", ClusterID: "c1"}, true},
		{"deployer: wrong app", []domain.Binding{bind(RoleDeployer, id("c1"), id("a1"))}, AppSync, Target{AppID: "a2", ClusterID: "c1"}, false},
		{"deployer: wrong cluster", []domain.Binding{bind(RoleDeployer, id("c1"), id("a1"))}, AppSync, Target{AppID: "a1", ClusterID: "c2"}, false},
		{"deployer: any app on one cluster", []domain.Binding{bind(RoleDeployer, id("c1"), anyRef)}, AppRollback, Target{AppID: "a9", ClusterID: "c1"}, true},
		{"deployer: one app on any cluster", []domain.Binding{bind(RoleDeployer, anyRef, id("a1"))}, AppSync, Target{AppID: "a1", ClusterID: "c9"}, true},
		{"deployer cannot manage definitions", []domain.Binding{bind(RoleDeployer, anyRef, anyRef)}, AppManage, Target{AppID: "a1", ClusterID: "c1"}, false},
		{"maintainer can manage definitions", []domain.Binding{bind(RoleMaintainer, anyRef, anyRef)}, AppManage, Target{AppID: "a1", ClusterID: "c1"}, true},
		{"payments team deploys only to prod", []domain.Binding{bind(RoleDeployer, group("prod"), group("payments"))}, AppSync, Target{AppID: "a-pay-api", ClusterID: "c-prod-1"}, true},
		{"payments team: other app in prod", []domain.Binding{bind(RoleDeployer, group("prod"), group("payments"))}, AppSync, Target{AppID: "a-other", ClusterID: "c-prod-1"}, false},
		{"payments team: payments app in dev", []domain.Binding{bind(RoleDeployer, group("prod"), group("payments"))}, AppSync, Target{AppID: "a-pay-api", ClusterID: "c-dev"}, false},
		{"app permission without a cluster needs all clusters", []domain.Binding{bind(RoleMaintainer, id("c1"), anyRef)}, AppManage, Target{AppID: "a1"}, false},
		{"app permission without a cluster, all clusters", []domain.Binding{bind(RoleMaintainer, anyRef, anyRef)}, AppManage, Target{AppID: "a1"}, true},
		{"cluster-admin has no application permissions", []domain.Binding{bind(RoleClusterAdmin, anyRef, anyRef)}, AppSync, Target{AppID: "a1", ClusterID: "c1"}, false},

		// --- system permissions ---
		{"viewer has no system permission", []domain.Binding{bind(RoleViewer, anyRef, anyRef)}, UserManage, Target{}, false},
		{"deployer has no system permission", []domain.Binding{bind(RoleDeployer, anyRef, anyRef)}, RBACManage, Target{}, false},

		// --- grants add up ---
		{"union: second binding grants", []domain.Binding{bind(RoleViewer, id("c1"), anyRef), bind(RoleViewer, id("c2"), anyRef)}, ClusterView, Target{ClusterID: "c2"}, true},
		{"union: neither grants", []domain.Binding{bind(RoleViewer, id("c1"), anyRef), bind(RoleViewer, id("c2"), anyRef)}, ClusterView, Target{ClusterID: "c3"}, false},
		{"union: roles combine", []domain.Binding{bind(RoleViewer, anyRef, anyRef), bind(RoleClusterAdmin, id("c1"), anyRef)}, ClusterManage, Target{ClusterID: "c1"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewAccess(tt.bindings, groups).Can(tt.perm, tt.target)
			if got != tt.want {
				t.Fatalf("Can(%s, %+v) = %v, want %v", tt.perm, tt.target, got, tt.want)
			}
		})
	}
}

func TestGroupReferencesNeverMatchWithoutGroups(t *testing.T) {
	a := NewAccess([]domain.Binding{bind(RoleViewer, group("prod"), anyRef)}, nil)
	if a.Can(ClusterView, Target{ClusterID: "c1"}) {
		t.Fatal("a group reference must not match when no group source is configured")
	}
}

func TestPermissionClasses(t *testing.T) {
	for p, want := range map[Permission]Class{
		ClusterView: ClassCluster, ClusterManage: ClassCluster,
		AppView: ClassApp, AppSync: ClassApp, AppRollback: ClassApp, AppManage: ClassApp,
		UserManage: ClassSystem, TeamManage: ClassSystem, GroupManage: ClassSystem, RBACManage: ClassSystem, AuditView: ClassSystem,
	} {
		if p.Class() != want || !p.Valid() {
			t.Errorf("%s: class %d valid %v, want class %d valid", p, p.Class(), p.Valid(), want)
		}
	}
	if Wildcard.Class() != 0 || !Wildcard.Valid() {
		t.Error("the wildcard is valid but belongs to no class")
	}
	if Permission("nope").Valid() || Permission("nope").Class() != 0 {
		t.Error("unknown permissions must be invalid")
	}
}

func TestBuiltinRoles(t *testing.T) {
	want := map[string]struct {
		has     []Permission
		missing []Permission
		system  bool
	}{
		RoleViewer:       {has: []Permission{ClusterView, AppView}, missing: []Permission{AppSync, ClusterManage}},
		RoleDeployer:     {has: []Permission{ClusterView, AppView, AppSync, AppRollback}, missing: []Permission{AppManage, ClusterManage}},
		RoleMaintainer:   {has: []Permission{ClusterView, AppView, AppSync, AppRollback, AppManage}, missing: []Permission{ClusterManage, UserManage}},
		RoleClusterAdmin: {has: []Permission{ClusterView, ClusterManage}, missing: []Permission{AppView, AppSync}},
		RoleAdmin:        {has: []Permission{ClusterManage, AppManage, UserManage, RBACManage, AuditView}, system: true},
	}
	roles := Roles()
	if len(roles) != len(want) {
		t.Fatalf("got %d roles, want %d", len(roles), len(want))
	}
	for _, r := range roles {
		w, ok := want[r.ID]
		if !ok || !r.Builtin || r.Name == "" || r.Description == "" {
			t.Errorf("unexpected or incomplete role: %+v", r)
			continue
		}
		for _, p := range w.has {
			if !r.Has(p) {
				t.Errorf("%s must have %s", r.ID, p)
			}
		}
		for _, p := range w.missing {
			if r.Has(p) {
				t.Errorf("%s must not have %s", r.ID, p)
			}
		}
		if r.IsSystem() != w.system {
			t.Errorf("%s IsSystem = %v, want %v", r.ID, r.IsSystem(), w.system)
		}
	}
}

func TestRolesReturnsCopies(t *testing.T) {
	r := Roles()
	r[0].Permissions[0] = Wildcard
	r[0].Name = "hacked"
	again, _ := LookupRole(RoleViewer)
	if again.Name == "hacked" || again.Has(AppManage) {
		t.Fatal("callers must not be able to modify the built-in roles")
	}
	if _, ok := LookupRole("nope"); ok {
		t.Fatal("unknown role found")
	}
}

func TestSystemPermissionsAreExactlyTheSystemClass(t *testing.T) {
	listed := map[Permission]bool{}
	for _, p := range SystemPermissions() {
		if p.Class() != ClassSystem {
			t.Errorf("%s is listed but is not a system permission", p)
		}
		listed[p] = true
	}
	for p, class := range classes {
		if class == ClassSystem && !listed[p] {
			t.Errorf("%s is a system permission but missing from SystemPermissions()", p)
		}
	}
}
