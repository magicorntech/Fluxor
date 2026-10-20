package authz

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store/memory"
)

var ctx = context.Background()

func newAuthz(t *testing.T) (*Authorizer, *memory.Store) {
	t.Helper()
	st := memory.New()
	return New(st), st
}

func addUser(t *testing.T, st *memory.Store, id, createdBy string, disabled bool, at time.Time) {
	t.Helper()
	err := st.CreateUser(ctx, domain.User{ID: id, Username: "user-" + id, CreatedBy: createdBy, Disabled: disabled, CreatedAt: at})
	if err != nil {
		t.Fatal(err)
	}
}

func addBinding(t *testing.T, st *memory.Store, id, userID, role string, scope domain.Scope) {
	t.Helper()
	err := st.CreateBinding(ctx, domain.Binding{
		ID: id, Subject: domain.Subject{Kind: domain.SubjectUser, ID: userID}, RoleID: role, Scope: scope,
		CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestForLoadsOnlyTheUsersBindings(t *testing.T) {
	az, st := newAuthz(t)
	addUser(t, st, "alice", "", false, time.Now())
	addUser(t, st, "bob", "", false, time.Now())
	addBinding(t, st, "b1", "alice", RoleClusterAdmin, domain.AnyScope())
	addBinding(t, st, "b2", "bob", RoleViewer, domain.AnyScope())

	alice, err := az.For(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if !alice.Can(ClusterManage, Target{ClusterID: "c1"}) {
		t.Fatal("alice must be able to manage clusters")
	}
	bob, _ := az.For(ctx, "bob")
	if bob.Can(ClusterManage, Target{ClusterID: "c1"}) || !bob.Can(ClusterView, Target{ClusterID: "c1"}) {
		t.Fatal("bob must only be able to view")
	}
	nobody, _ := az.For(ctx, "ghost")
	if nobody.Can(ClusterView, Target{ClusterID: "c1"}) {
		t.Fatal("a user without bindings has no access")
	}
}

func TestValidateBinding(t *testing.T) {
	az, st := newAuthz(t)
	addUser(t, st, "alice", "", false, time.Now())
	if err := st.CreateCluster(ctx, domain.Cluster{ID: "c1", Name: "c1", APIServer: "https://x", Labels: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	addTeam(t, st, "t1", "team-one")
	addGroup(t, st, "g-cluster", domain.GroupKindCluster, "clusters")
	addGroup(t, st, "g-app", domain.GroupKindApplication, "apps")

	user := domain.Subject{Kind: domain.SubjectUser, ID: "alice"}
	clusterC1 := domain.Scope{Cluster: domain.Ref{Kind: domain.RefID, ID: "c1"}, Application: domain.AnyRef()}
	mk := func(sub domain.Subject, role string, scope domain.Scope) domain.Binding {
		return domain.Binding{Subject: sub, RoleID: role, Scope: scope}
	}

	tests := []struct {
		name    string
		binding domain.Binding
		wantErr bool
	}{
		{"viewer everywhere", mk(user, RoleViewer, domain.AnyScope()), false},
		{"deployer on an existing cluster", mk(user, RoleDeployer, clusterC1), false},
		{"cluster-admin on an existing cluster", mk(user, RoleClusterAdmin, clusterC1), false},
		{"admin everywhere", mk(user, RoleAdmin, domain.AnyScope()), false},

		{"malformed", domain.Binding{}, true},
		{"unknown role", mk(user, "superuser", domain.AnyScope()), true},
		{"unknown user", mk(domain.Subject{Kind: domain.SubjectUser, ID: "ghost"}, RoleViewer, domain.AnyScope()), true},
		{"unknown cluster", mk(user, RoleViewer, domain.Scope{Cluster: domain.Ref{Kind: domain.RefID, ID: "nope"}, Application: domain.AnyRef()}), true},
		{"admin cannot be narrowed to a cluster", mk(user, RoleAdmin, clusterC1), true},
		{"cluster-admin cannot be limited by application", mk(user, RoleClusterAdmin,
			domain.Scope{Cluster: domain.AnyRef(), Application: domain.Ref{Kind: domain.RefID, ID: "a1"}}), true},
		{"a team as subject", mk(domain.Subject{Kind: domain.SubjectTeam, ID: "t1"}, RoleViewer, domain.AnyScope()), false},
		{"unknown team", mk(domain.Subject{Kind: domain.SubjectTeam, ID: "nope"}, RoleViewer, domain.AnyScope()), true},
		{"a cluster group as scope", mk(user, RoleViewer, domain.Scope{Cluster: domain.Ref{Kind: domain.RefGroup, ID: "g-cluster"}, Application: domain.AnyRef()}), false},
		{"unknown group", mk(user, RoleViewer, domain.Scope{Cluster: domain.Ref{Kind: domain.RefGroup, ID: "nope"}, Application: domain.AnyRef()}), true},
		{"an application group cannot scope clusters", mk(user, RoleViewer, domain.Scope{Cluster: domain.Ref{Kind: domain.RefGroup, ID: "g-app"}, Application: domain.AnyRef()}), true},
		{"a team with a cluster group", mk(domain.Subject{Kind: domain.SubjectTeam, ID: "t1"}, RoleDeployer, domain.Scope{Cluster: domain.Ref{Kind: domain.RefGroup, ID: "g-cluster"}, Application: domain.AnyRef()}), false},
		{"application scopes are not available yet", mk(user, RoleDeployer, domain.Scope{Cluster: domain.AnyRef(), Application: domain.Ref{Kind: domain.RefID, ID: "a1"}}), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := az.ValidateBinding(ctx, tt.binding)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateBinding() = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidBinding) {
				t.Fatalf("the error must wrap ErrInvalidBinding: %v", err)
			}
		})
	}
}

func TestAdminUserIDs(t *testing.T) {
	az, st := newAuthz(t)
	now := time.Now()
	addUser(t, st, "a1", "", false, now)
	addUser(t, st, "a2", "", false, now)
	addUser(t, st, "off", "", true, now)
	addUser(t, st, "narrow", "", false, now)
	addUser(t, st, "viewer", "", false, now)

	addBinding(t, st, "b-a1", "a1", RoleAdmin, domain.AnyScope())
	addBinding(t, st, "b-a2", "a2", RoleAdmin, domain.AnyScope())
	addBinding(t, st, "b-off", "off", RoleAdmin, domain.AnyScope())
	addBinding(t, st, "b-viewer", "viewer", RoleViewer, domain.AnyScope())
	// A binding for a user that no longer exists must be ignored.
	addBinding(t, st, "b-ghost", "ghost", RoleAdmin, domain.AnyScope())
	// Not narrowed means any/any only; this is stored directly, bypassing validation, to prove the
	// count does not trust the stored data blindly.
	addBinding(t, st, "b-narrow", "narrow", RoleAdmin, domain.Scope{Cluster: domain.Ref{Kind: domain.RefID, ID: "c1"}, Application: domain.AnyRef()})

	got, err := az.AdminUserIDs(ctx, AdminChange{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a1" || got[1] != "a2" {
		t.Fatalf("active admins = %v, want [a1 a2] (disabled, narrowed, non-admin and missing users do not count)", got)
	}

	got, _ = az.AdminUserIDs(ctx, AdminChange{ExcludeBinding: "b-a1"})
	if len(got) != 1 || got[0] != "a2" {
		t.Fatalf("without b-a1: %v, want [a2]", got)
	}
}

func TestEnsureBootstrapAdmin(t *testing.T) {
	t.Run("grants the bootstrap user once", func(t *testing.T) {
		az, st := newAuthz(t)
		addUser(t, st, "boot", BootstrapCreator, false, time.Now())

		g, err := az.EnsureBootstrapAdmin(ctx)
		if err != nil || !g.Created || g.UserID != "boot" || g.Username != "user-boot" {
			t.Fatalf("got %+v %v", g, err)
		}
		b, err := st.GetBinding(ctx, BootstrapBindingID)
		if err != nil || b.RoleID != RoleAdmin || !b.Scope.IsAny() || b.Subject.ID != "boot" || b.CreatedBy != BootstrapCreator {
			t.Fatalf("unexpected binding: %+v %v", b, err)
		}
		access, _ := az.For(ctx, "boot")
		if !access.Can(UserManage, Target{}) {
			t.Fatal("the bootstrap user must now be an administrator")
		}

		again, err := az.EnsureBootstrapAdmin(ctx)
		if err != nil || again.Created {
			t.Fatalf("second call must do nothing: %+v %v", again, err)
		}
	})

	t.Run("does nothing when an admin binding exists", func(t *testing.T) {
		az, st := newAuthz(t)
		addUser(t, st, "boot", BootstrapCreator, false, time.Now())
		addUser(t, st, "other", "", false, time.Now())
		addBinding(t, st, "b1", "other", RoleAdmin, domain.AnyScope())

		if g, err := az.EnsureBootstrapAdmin(ctx); err != nil || g.Created {
			t.Fatalf("must not grant while an administrator exists: %+v %v", g, err)
		}
		if _, err := st.GetBinding(ctx, BootstrapBindingID); err == nil {
			t.Fatal("no bootstrap binding may be created")
		}
	})

	t.Run("ignores users that were not created by the bootstrap", func(t *testing.T) {
		az, st := newAuthz(t)
		addUser(t, st, "manual", "someone", false, time.Now())
		if g, err := az.EnsureBootstrapAdmin(ctx); err != nil || g.Created {
			t.Fatalf("got %+v %v", g, err)
		}
	})

	t.Run("empty database", func(t *testing.T) {
		az, _ := newAuthz(t)
		if g, err := az.EnsureBootstrapAdmin(ctx); err != nil || g.Created {
			t.Fatalf("got %+v %v", g, err)
		}
	})

	t.Run("picks the oldest bootstrap user", func(t *testing.T) {
		az, st := newAuthz(t)
		now := time.Now()
		addUser(t, st, "newer", BootstrapCreator, false, now)
		addUser(t, st, "older", BootstrapCreator, false, now.Add(-time.Hour))
		g, err := az.EnsureBootstrapAdmin(ctx)
		if err != nil || g.UserID != "older" {
			t.Fatalf("got %+v %v", g, err)
		}
	})
}

func addTeam(t *testing.T, st *memory.Store, id, name string, members ...string) {
	t.Helper()
	if members == nil {
		members = []string{}
	}
	if err := st.CreateTeam(ctx, domain.Team{ID: id, Name: name, Members: members, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func addGroup(t *testing.T, st *memory.Store, id string, kind domain.GroupKind, name string, members ...string) {
	t.Helper()
	if members == nil {
		members = []string{}
	}
	if err := st.CreateGroup(ctx, domain.ResourceGroup{ID: id, Kind: kind, Name: name, Members: members, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func addTeamBinding(t *testing.T, st *memory.Store, id, teamID, role string, scope domain.Scope) {
	t.Helper()
	err := st.CreateBinding(ctx, domain.Binding{
		ID: id, Subject: domain.Subject{Kind: domain.SubjectTeam, ID: teamID}, RoleID: role, Scope: scope, CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func groupScope(groupID string) domain.Scope {
	return domain.Scope{Cluster: domain.Ref{Kind: domain.RefGroup, ID: groupID}, Application: domain.AnyRef()}
}

func TestTeamMembershipGrantsTheTeamsRoles(t *testing.T) {
	az, st := newAuthz(t)
	now := time.Now()
	for _, id := range []string{"ayse", "mehmet", "outsider"} {
		addUser(t, st, id, "", false, now)
	}
	addTeam(t, st, "payments", "payments", "ayse", "mehmet")
	addTeam(t, st, "platform", "platform", "mehmet")
	addTeamBinding(t, st, "b-pay", "payments", RoleViewer, domain.AnyScope())
	addTeamBinding(t, st, "b-plat", "platform", RoleClusterAdmin, domain.AnyScope())
	addBinding(t, st, "b-own", "mehmet", RoleDeployer, domain.AnyScope())

	can := func(user string, p Permission) bool {
		access, err := az.For(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		return access.Can(p, Target{ClusterID: "c1", AppID: "a1"})
	}
	if !can("ayse", ClusterView) || can("ayse", ClusterManage) {
		t.Fatal("ayse gets the payments team's viewer role only")
	}
	if !can("mehmet", ClusterView) || !can("mehmet", ClusterManage) || !can("mehmet", AppSync) {
		t.Fatal("mehmet unions both teams' roles and his own")
	}
	if can("outsider", ClusterView) {
		t.Fatal("a non-member gets nothing")
	}

	// Membership is read on every call, so changes apply at once.
	if err := st.SetTeamMembers(ctx, "payments", []string{"ayse"}); err != nil {
		t.Fatal(err)
	}
	if !can("ayse", ClusterView) || !can("mehmet", ClusterManage) {
		t.Fatal("removing mehmet from payments must not touch what the platform team grants him")
	}
	if err := st.SetTeamMembers(ctx, "payments", nil); err != nil {
		t.Fatal(err)
	}
	if can("ayse", ClusterView) {
		t.Fatal("ayse lost her only grant when the team was emptied")
	}
	if err := st.DeleteTeam(ctx, "platform"); err != nil {
		t.Fatal(err)
	}
	if can("mehmet", ClusterManage) {
		t.Fatal("a deleted team grants nothing")
	}
}

func TestGroupScopeFollowsGroupMembership(t *testing.T) {
	az, st := newAuthz(t)
	addUser(t, st, "ayse", "", false, time.Now())
	addGroup(t, st, "prod", domain.GroupKindCluster, "prod", "c-prod-1")
	addGroup(t, st, "apps", domain.GroupKindApplication, "apps", "c-prod-2") // wrong kind on purpose
	addBinding(t, st, "b1", "ayse", RoleViewer, groupScope("prod"))
	addBinding(t, st, "b2", "ayse", RoleClusterAdmin, groupScope("apps"))

	can := func(p Permission, cluster string) bool {
		access, err := az.For(ctx, "ayse")
		if err != nil {
			t.Fatal(err)
		}
		return access.Can(p, Target{ClusterID: cluster})
	}
	if !can(ClusterView, "c-prod-1") || can(ClusterView, "c-dev") {
		t.Fatal("the cluster group scopes the viewer role to its members")
	}
	if can(ClusterManage, "c-prod-2") {
		t.Fatal("an application group must not grant access to a cluster even if the ids match")
	}

	if err := st.SetGroupMembers(ctx, "prod", []string{"c-prod-1", "c-dev"}); err != nil {
		t.Fatal(err)
	}
	if !can(ClusterView, "c-dev") {
		t.Fatal("a new group member is covered at once")
	}
	if err := st.RemoveFromGroups(ctx, domain.GroupKindCluster, "c-prod-1"); err != nil {
		t.Fatal(err)
	}
	if can(ClusterView, "c-prod-1") {
		t.Fatal("a removed member is no longer covered")
	}
	if err := st.DeleteGroup(ctx, "prod"); err != nil {
		t.Fatal(err)
	}
	if can(ClusterView, "c-dev") {
		t.Fatal("a deleted group covers nothing")
	}
}

func TestTeamAndGroupTogether(t *testing.T) {
	// "The payments team may deploy, but only to the prod clusters."
	az, st := newAuthz(t)
	addUser(t, st, "ayse", "", false, time.Now())
	addGroup(t, st, "prod", domain.GroupKindCluster, "prod", "c-prod-1", "c-prod-2")
	addTeam(t, st, "payments", "payments", "ayse")
	addTeamBinding(t, st, "b1", "payments", RoleDeployer, groupScope("prod"))

	access, _ := az.For(ctx, "ayse")
	for cluster, want := range map[string]bool{"c-prod-1": true, "c-prod-2": true, "c-dev": false} {
		if got := access.Can(AppSync, Target{AppID: "any-app", ClusterID: cluster}); got != want {
			t.Errorf("AppSync on %s = %v, want %v", cluster, got, want)
		}
	}
	if access.Can(AppManage, Target{AppID: "any-app", ClusterID: "c-prod-1"}) {
		t.Error("a deployer cannot manage application definitions")
	}
}

func TestAdminUserIDsCountsTeamMembers(t *testing.T) {
	az, st := newAuthz(t)
	now := time.Now()
	for _, id := range []string{"direct", "viaTeam", "disabledViaTeam", "other"} {
		addUser(t, st, id, "", id == "disabledViaTeam", now)
	}
	addBinding(t, st, "b-direct", "direct", RoleAdmin, domain.AnyScope())
	addTeam(t, st, "ops", "ops", "viaTeam", "disabledViaTeam", "ghost-user")
	addTeamBinding(t, st, "b-team", "ops", RoleAdmin, domain.AnyScope())
	addTeam(t, st, "narrow-team", "narrow", "other")
	addTeamBinding(t, st, "b-narrow", "narrow-team", RoleAdmin, groupScope("anything"))

	ids := func(c AdminChange) string {
		got, err := az.AdminUserIDs(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(got, ",")
	}
	if got := ids(AdminChange{}); got != "direct,viaTeam" {
		t.Fatalf("admins = %q, want direct,viaTeam (disabled, missing and narrowed do not count)", got)
	}
	if got := ids(AdminChange{ExcludeBinding: "b-team"}); got != "direct" {
		t.Fatalf("without the team binding: %q", got)
	}
	if got := ids(AdminChange{TeamMembers: map[string][]string{"ops": {"other"}}}); got != "direct,other" {
		t.Fatalf("with the team's members replaced: %q", got)
	}
	if got := ids(AdminChange{TeamMembers: map[string][]string{"ops": nil}}); got != "direct" {
		t.Fatalf("with the team emptied (or deleted): %q", got)
	}
}

func TestWouldLeaveNoAdmin(t *testing.T) {
	az, st := newAuthz(t)
	addUser(t, st, "boss", "", false, time.Now())
	addUser(t, st, "deputy", "", false, time.Now())
	addBinding(t, st, "b-boss", "boss", RoleAdmin, domain.AnyScope())

	would := func(c AdminChange) bool {
		got, err := az.WouldLeaveNoAdmin(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if !would(AdminChange{ExcludeBinding: "b-boss"}) {
		t.Fatal("removing the only admin binding must be flagged")
	}
	if would(AdminChange{ExcludeBinding: "unrelated"}) {
		t.Fatal("removing an unrelated binding is fine")
	}

	addTeam(t, st, "ops", "ops", "deputy")
	addTeamBinding(t, st, "b-ops", "ops", RoleAdmin, domain.AnyScope())
	if would(AdminChange{ExcludeBinding: "b-boss"}) {
		t.Fatal("the team's member is a fallback administrator")
	}
	if !would(AdminChange{ExcludeBinding: "b-boss", TeamMembers: map[string][]string{"ops": nil}}) {
		t.Fatal("removing the boss and emptying the team leaves nobody")
	}

	// With no administrator at all there is nothing to protect, so repairs stay possible.
	az2, _ := newAuthz(t)
	if got, _ := az2.WouldLeaveNoAdmin(ctx, AdminChange{ExcludeBinding: "x"}); got {
		t.Fatal("must not flag when there is no administrator to begin with")
	}
}
