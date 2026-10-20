package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

func apiToken(id, userID, hash string, created time.Time) domain.APIToken {
	return domain.APIToken{ID: id, UserID: userID, Name: "name-" + id, Prefix: "flx_" + id[:1], TokenHash: hash, CreatedAt: created, ExpiresAt: created.Add(24 * time.Hour)}
}

func tokens(t *testing.T, s store.Store) {
	ctx := context.Background()
	t0 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	if got, err := s.ListTokens(ctx, "u1"); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty list must be a non-nil empty slice: %v %v", got, err)
	}
	for _, tk := range []domain.APIToken{
		apiToken("a1", "u1", "hash-a1", t0),
		apiToken("b1", "u1", "hash-b1", t0.Add(time.Hour)),
		apiToken("c1", "u2", "hash-c1", t0.Add(2*time.Hour)),
	} {
		if err := s.CreateToken(ctx, tk); err != nil {
			t.Fatalf("create %s: %v", tk.ID, err)
		}
	}
	if err := s.CreateToken(ctx, apiToken("d1", "u1", "hash-a1", t0)); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("duplicate hash: %v", err)
	}
	if err := s.CreateToken(ctx, apiToken("a1", "u1", "hash-new", t0)); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("duplicate id: %v", err)
	}

	got, err := s.GetTokenByHash(ctx, "hash-b1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "b1" || got.UserID != "u1" || got.Name != "name-b1" || got.Prefix != "flx_b" || got.TokenHash != "hash-b1" ||
		!got.CreatedAt.Equal(t0.Add(time.Hour)) || !got.ExpiresAt.Equal(t0.Add(25*time.Hour)) || got.LastUsedAt != nil {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if _, err := s.GetTokenByHash(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown hash: %v", err)
	}

	list, _ := s.ListTokens(ctx, "u1")
	if len(list) != 2 || list[0].ID != "b1" || list[1].ID != "a1" {
		t.Fatalf("a user's tokens, newest first: %+v", list)
	}
	if n, _ := s.CountTokens(ctx, "u1"); n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
	if n, _ := s.CountTokens(ctx, "nobody"); n != 0 {
		t.Fatalf("count for nobody = %d", n)
	}

	used := t0.Add(48 * time.Hour)
	if err := s.TouchToken(ctx, "a1", used); err != nil {
		t.Fatal(err)
	}
	if after, _ := s.GetTokenByHash(ctx, "hash-a1"); after.LastUsedAt == nil || !after.LastUsedAt.Equal(used) {
		t.Fatalf("last use not recorded: %+v", after)
	}
	if err := s.TouchToken(ctx, "missing", used); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("touch missing: %v", err)
	}

	// A token can only be deleted by its owner.
	if err := s.DeleteToken(ctx, "u2", "a1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("another user must not delete it: %v", err)
	}
	if _, err := s.GetTokenByHash(ctx, "hash-a1"); err != nil {
		t.Fatalf("the token must have survived: %v", err)
	}
	if err := s.DeleteToken(ctx, "u1", "a1"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteToken(ctx, "u1", "a1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
	if _, err := s.GetTokenByHash(ctx, "hash-a1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted token still found: %v", err)
	}

	if err := s.DeleteUserTokens(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.CountTokens(ctx, "u1"); n != 0 {
		t.Fatalf("the user's tokens must be gone: %d", n)
	}
	if n, _ := s.CountTokens(ctx, "u2"); n != 1 {
		t.Fatalf("another user's tokens must stay: %d", n)
	}
}
