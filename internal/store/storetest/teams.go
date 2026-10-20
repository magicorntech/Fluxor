package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

func team(id, name string, members ...string) domain.Team {
	return domain.Team{ID: id, Name: name, Description: "d-" + id, Members: members, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), CreatedBy: "test"}
}

func teams(t *testing.T, s store.Store) {
	ctx := context.Background()

	if got, err := s.ListTeams(ctx); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty list must be a non-nil empty slice: %v %v", got, err)
	}
	for _, tm := range []domain.Team{team("t1", "Zeta", "u1", "u2"), team("t2", "alpha", "u2"), team("t3", "Empty")} {
		if err := s.CreateTeam(ctx, tm); err != nil {
			t.Fatalf("create %s: %v", tm.Name, err)
		}
	}
	if err := s.CreateTeam(ctx, team("t4", "ALPHA")); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("names must be unique ignoring case: %v", err)
	}
	if err := s.CreateTeam(ctx, team("t1", "Another")); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("duplicate id: %v", err)
	}

	list, err := s.ListTeams(ctx)
	if err != nil || len(list) != 3 || list[0].Name != "alpha" || list[1].Name != "Empty" || list[2].Name != "Zeta" {
		t.Fatalf("must be sorted by name ignoring case: %+v %v", list, err)
	}

	got, err := s.GetTeam(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Zeta" || got.Description != "d-t1" || len(got.Members) != 2 || got.CreatedBy != "test" || !got.CreatedAt.Equal(team("", "").CreatedAt) {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if empty, _ := s.GetTeam(ctx, "t3"); empty.Members == nil || len(empty.Members) != 0 {
		t.Fatalf("an empty team must have [] members, not nil: %#v", empty.Members)
	}
	if _, err := s.GetTeam(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}

	names := func(ts []domain.Team) []string {
		out := []string{}
		for _, x := range ts {
			out = append(out, x.Name)
		}
		return out
	}
	of, _ := s.ListTeamsOfUser(ctx, "u2")
	if n := names(of); len(n) != 2 || n[0] != "alpha" || n[1] != "Zeta" {
		t.Fatalf("teams of u2: %v", n)
	}
	if of, _ := s.ListTeamsOfUser(ctx, "nobody"); of == nil || len(of) != 0 {
		t.Fatalf("a user without teams gets an empty non-nil slice: %#v", of)
	}

	// Renaming keeps the members; membership changes keep the name.
	renamed := got
	renamed.Name, renamed.Description = "Omega", "new"
	if err := s.UpdateTeam(ctx, renamed); err != nil {
		t.Fatal(err)
	}
	if after, _ := s.GetTeam(ctx, "t1"); after.Name != "Omega" || after.Description != "new" || len(after.Members) != 2 {
		t.Fatalf("update must change only name and description: %+v", after)
	}
	if err := s.SetTeamMembers(ctx, "t1", []string{"u9"}); err != nil {
		t.Fatal(err)
	}
	if after, _ := s.GetTeam(ctx, "t1"); after.Name != "Omega" || len(after.Members) != 1 || after.Members[0] != "u9" {
		t.Fatalf("members must be replaced, name kept: %+v", after)
	}
	if err := s.SetTeamMembers(ctx, "t1", nil); err != nil {
		t.Fatal(err)
	}
	if after, _ := s.GetTeam(ctx, "t1"); after.Members == nil || len(after.Members) != 0 {
		t.Fatalf("clearing members gives []: %#v", after.Members)
	}
	clash := got
	clash.Name = "ALPHA"
	if err := s.UpdateTeam(ctx, clash); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("rename to a taken name: %v", err)
	}
	same := renamed
	same.Description = "only the description changes"
	if err := s.UpdateTeam(ctx, same); err != nil {
		t.Fatalf("keeping its own name is not a clash: %v", err)
	}
	if err := s.UpdateTeam(ctx, team("missing", "x")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}
	if err := s.SetTeamMembers(ctx, "missing", nil); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("set members of missing: %v", err)
	}

	if err := s.DeleteTeam(ctx, "t2"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteTeam(ctx, "t2"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
	// The name is free again.
	if err := s.CreateTeam(ctx, team("t5", "alpha")); err != nil {
		t.Fatalf("re-create after delete: %v", err)
	}
}

func group(id string, kind domain.GroupKind, name string, members ...string) domain.ResourceGroup {
	return domain.ResourceGroup{ID: id, Kind: kind, Name: name, Description: "d-" + id, Members: members, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), CreatedBy: "test"}
}

func groups(t *testing.T, s store.Store) {
	ctx := context.Background()
	cl, ap := domain.GroupKindCluster, domain.GroupKindApplication

	if got, err := s.ListGroups(ctx, store.GroupFilter{}); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty list must be a non-nil empty slice: %v %v", got, err)
	}
	for _, g := range []domain.ResourceGroup{
		group("g1", cl, "prod", "c1", "c2"), group("g2", cl, "Dev", "c3"), group("g3", ap, "prod", "a1"), group("g4", ap, "empty"),
	} {
		if err := s.CreateGroup(ctx, g); err != nil {
			t.Fatalf("create %s: %v", g.ID, err)
		}
	}
	if err := s.CreateGroup(ctx, group("g5", cl, "PROD")); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("names are unique per kind, ignoring case: %v", err)
	}
	if err := s.CreateGroup(ctx, group("g1", cl, "other")); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("duplicate id: %v", err)
	}

	ids := func(f store.GroupFilter) []string {
		list, err := s.ListGroups(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, g := range list {
			out = append(out, g.ID)
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
	eq("all, sorted by name", ids(store.GroupFilter{}), []string{"g2", "g4", "g1", "g3"})
	eq("clusters only", ids(store.GroupFilter{Kind: cl}), []string{"g2", "g1"})
	eq("applications only", ids(store.GroupFilter{Kind: ap}), []string{"g4", "g3"})
	eq("by ids", ids(store.GroupFilter{IDs: []string{"g3", "g2", "missing"}}), []string{"g2", "g3"})
	eq("by ids and kind", ids(store.GroupFilter{Kind: cl, IDs: []string{"g3", "g2"}}), []string{"g2"})

	got, err := s.GetGroup(ctx, "g1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != cl || got.Name != "prod" || got.Description != "d-g1" || len(got.Members) != 2 || got.CreatedBy != "test" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if empty, _ := s.GetGroup(ctx, "g4"); empty.Members == nil || len(empty.Members) != 0 {
		t.Fatalf("an empty group must have [] members: %#v", empty.Members)
	}
	if _, err := s.GetGroup(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}

	renamed := got
	renamed.Name, renamed.Description = "production", "new"
	renamed.Kind = ap // the kind cannot be changed by an update
	if err := s.UpdateGroup(ctx, renamed); err != nil {
		t.Fatal(err)
	}
	if after, _ := s.GetGroup(ctx, "g1"); after.Name != "production" || after.Description != "new" || after.Kind != cl || len(after.Members) != 2 {
		t.Fatalf("update must change only name and description: %+v", after)
	}
	clash := got
	clash.Name = "DEV"
	if err := s.UpdateGroup(ctx, clash); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("rename to a taken name: %v", err)
	}
	// The same name in the other kind is fine.
	other := group("g3", ap, "Production")
	if err := s.UpdateGroup(ctx, other); err != nil {
		t.Fatalf("same name in another kind is allowed: %v", err)
	}
	if err := s.UpdateGroup(ctx, group("missing", cl, "x")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}

	if err := s.SetGroupMembers(ctx, "g2", []string{"c7", "c8"}); err != nil {
		t.Fatal(err)
	}
	if after, _ := s.GetGroup(ctx, "g2"); len(after.Members) != 2 || after.Name != "Dev" {
		t.Fatalf("members replaced, name kept: %+v", after)
	}
	if err := s.SetGroupMembers(ctx, "missing", nil); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("set members of missing: %v", err)
	}

	// RemoveFromGroups pulls the resource out of the groups of that kind only.
	if err := s.SetGroupMembers(ctx, "g1", []string{"x", "c2"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetGroupMembers(ctx, "g3", []string{"x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveFromGroups(ctx, cl, "x"); err != nil {
		t.Fatal(err)
	}
	if g1, _ := s.GetGroup(ctx, "g1"); len(g1.Members) != 1 || g1.Members[0] != "c2" {
		t.Fatalf("cluster group must lose the member: %+v", g1)
	}
	if g3, _ := s.GetGroup(ctx, "g3"); len(g3.Members) != 1 {
		t.Fatalf("application group must keep it: %+v", g3)
	}
	if err := s.RemoveFromGroups(ctx, cl, "never-a-member"); err != nil {
		t.Fatalf("removing a non-member is a no-op: %v", err)
	}

	if err := s.DeleteGroup(ctx, "g4"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteGroup(ctx, "g4"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
	if err := s.CreateGroup(ctx, group("g6", ap, "empty")); err != nil {
		t.Fatalf("re-create after delete: %v", err)
	}
}
