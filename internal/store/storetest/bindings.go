package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

func binding(id string, sub domain.Subject, role string, scope domain.Scope, at time.Time) domain.Binding {
	return domain.Binding{ID: id, Subject: sub, RoleID: role, Scope: scope, CreatedAt: at, CreatedBy: "test"}
}

func bindings(t *testing.T, s store.Store) {
	ctx := context.Background()
	t0 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	alice := domain.Subject{Kind: domain.SubjectUser, ID: "alice"}
	bob := domain.Subject{Kind: domain.SubjectUser, ID: "bob"}
	team := domain.Subject{Kind: domain.SubjectTeam, ID: "payments"}
	clusterA := domain.Scope{Cluster: domain.Ref{Kind: domain.RefID, ID: "ca"}, Application: domain.AnyRef()}

	if got, err := s.ListBindings(ctx, store.BindingFilter{}); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty list must be a non-nil empty slice: %v %v", got, err)
	}

	for _, b := range []domain.Binding{
		binding("b1", alice, "viewer", domain.AnyScope(), t0),
		binding("b2", alice, "deployer", clusterA, t0.Add(time.Second)),
		binding("b3", bob, "viewer", domain.AnyScope(), t0.Add(2*time.Second)),
		binding("b4", team, "maintainer", clusterA, t0.Add(3*time.Second)),
	} {
		if err := s.CreateBinding(ctx, b); err != nil {
			t.Fatalf("create %s: %v", b.ID, err)
		}
	}

	if err := s.CreateBinding(ctx, binding("b1", bob, "admin", domain.AnyScope(), t0)); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("duplicate id: %v", err)
	}
	if err := s.CreateBinding(ctx, binding("b9", alice, "viewer", domain.AnyScope(), t0)); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("identical grant under a new id: %v", err)
	}
	// The same role in another scope is a different grant.
	if err := s.CreateBinding(ctx, binding("b10", alice, "viewer", clusterA, t0.Add(4*time.Second))); err != nil {
		t.Fatalf("same role, other scope: %v", err)
	}

	got, err := s.GetBinding(ctx, "b2")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Subject != alice || got.RoleID != "deployer" || got.Scope != clusterA || got.CreatedBy != "test" || !got.CreatedAt.Equal(t0.Add(time.Second)) {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if _, err := s.GetBinding(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}

	ids := func(f store.BindingFilter) []string {
		list, err := s.ListBindings(ctx, f)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		out := make([]string, 0, len(list))
		for _, b := range list {
			out = append(out, b.ID)
		}
		return out
	}
	eq := func(name string, got, want []string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: got %v, want %v", name, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s: got %v, want %v", name, got, want)
			}
		}
	}
	eq("all, in creation order", ids(store.BindingFilter{}), []string{"b1", "b2", "b3", "b4", "b10"})
	eq("one subject", ids(store.BindingFilter{Subjects: []domain.Subject{alice}}), []string{"b1", "b2", "b10"})
	eq("several subjects", ids(store.BindingFilter{Subjects: []domain.Subject{bob, team}}), []string{"b3", "b4"})
	eq("a team is not a user with the same id", ids(store.BindingFilter{Subjects: []domain.Subject{{Kind: domain.SubjectUser, ID: "payments"}}}), []string{})
	eq("by role", ids(store.BindingFilter{RoleID: "viewer"}), []string{"b1", "b3", "b10"})
	eq("subject and role", ids(store.BindingFilter{Subjects: []domain.Subject{alice}, RoleID: "viewer"}), []string{"b1", "b10"})

	if err := s.DeleteBinding(ctx, "b2"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteBinding(ctx, "b2"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
	// The grant can be created again once its binding is gone.
	if err := s.CreateBinding(ctx, binding("b11", alice, "deployer", clusterA, t0.Add(5*time.Second))); err != nil {
		t.Fatalf("re-create after delete: %v", err)
	}
}

func listUsers(t *testing.T, s store.Store) {
	ctx := context.Background()
	if got, err := s.ListUsers(ctx); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty list must be a non-nil empty slice: %v %v", got, err)
	}
	for _, u := range []domain.User{user("1", "zoe", ""), user("2", "adam", ""), user("3", "mia", "")} {
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ListUsers(ctx)
	if err != nil || len(got) != 3 || got[0].Username != "adam" || got[1].Username != "mia" || got[2].Username != "zoe" {
		t.Fatalf("must be sorted by username: %+v %v", got, err)
	}
	if got[0].PasswordHash == "" {
		t.Fatal("ListUsers must return full records")
	}
}
