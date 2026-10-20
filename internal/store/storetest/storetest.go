// Package storetest holds conformance tests that every store.Store implementation must pass.
package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

// Run executes the conformance suite. newStore must return an empty store.
func Run(t *testing.T, newStore func(t *testing.T) store.Store) {
	t.Run("Clusters", func(t *testing.T) { clusters(t, newStore(t)) })
	t.Run("Users", func(t *testing.T) { users(t, newStore(t)) })
	t.Run("Sessions", func(t *testing.T) { sessions(t, newStore(t)) })
	t.Run("ListUsers", func(t *testing.T) { listUsers(t, newStore(t)) })
	t.Run("Bindings", func(t *testing.T) { bindings(t, newStore(t)) })
	t.Run("Teams", func(t *testing.T) { teams(t, newStore(t)) })
	t.Run("Groups", func(t *testing.T) { groups(t, newStore(t)) })
	t.Run("Tokens", func(t *testing.T) { tokens(t, newStore(t)) })
	t.Run("Audit", func(t *testing.T) { audit(t, newStore(t)) })
}

func cluster(id, name string) domain.Cluster {
	return domain.Cluster{
		ID:        id,
		Name:      name,
		APIServer: "https://" + name + ":6443",
		Labels:    map[string]string{"env": "test"},
		Status:    domain.ClusterStatusUnknown,
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func clusters(t *testing.T, s store.Store) {
	ctx := context.Background()

	if items, err := s.ListClusters(ctx); err != nil || len(items) != 0 {
		t.Fatalf("empty list: %v %v", items, err)
	}

	for _, c := range []domain.Cluster{cluster("1", "zeta"), cluster("2", "alpha")} {
		if err := s.CreateCluster(ctx, c); err != nil {
			t.Fatalf("create %s: %v", c.Name, err)
		}
	}

	items, err := s.ListClusters(ctx)
	if err != nil || len(items) != 2 || items[0].Name != "alpha" || items[1].Name != "zeta" {
		t.Fatalf("list must be sorted by name: %v %v", items, err)
	}

	got, err := s.GetCluster(ctx, "2")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	want := cluster("2", "alpha")
	if got.Name != want.Name || got.APIServer != want.APIServer || got.Labels["env"] != "test" ||
		got.Status != want.Status || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("round trip mismatch: got %+v want %+v", got, want)
	}

	if err := s.CreateCluster(ctx, cluster("3", "alpha")); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("duplicate name: got %v, want ErrAlreadyExists", err)
	}

	nolabels := cluster("4", "nolabels")
	nolabels.Labels = nil
	if err := s.CreateCluster(ctx, nolabels); err != nil {
		t.Fatalf("create without labels: %v", err)
	}
	if got, err := s.GetCluster(ctx, "4"); err != nil || got.Labels == nil {
		t.Fatalf("labels must never be nil: %+v %v", got, err)
	}

	if _, err := s.GetCluster(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get missing: got %v, want ErrNotFound", err)
	}
	if err := s.DeleteCluster(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("delete missing: got %v, want ErrNotFound", err)
	}
	if err := s.DeleteCluster(ctx, "1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetCluster(ctx, "1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get after delete: got %v", err)
	}
}
