package storetest

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

func user(id, username, email string) domain.User {
	return domain.User{
		ID:                 id,
		Username:           username,
		Email:              email,
		DisplayName:        "User " + username,
		PasswordHash:       "hash-" + id,
		MustChangePassword: true,
		CreatedAt:          time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		CreatedBy:          "test",
	}
}

func users(t *testing.T, s store.Store) {
	ctx := context.Background()

	if n, err := s.CountUsers(ctx); err != nil || n != 0 {
		t.Fatalf("count on empty store: %d %v", n, err)
	}

	if err := s.CreateUser(ctx, user("1", "ayse", "ayse@example.com")); err != nil {
		t.Fatalf("create: %v", err)
	}
	// Users without an email must not collide with each other.
	if err := s.CreateUser(ctx, user("2", "mehmet", "")); err != nil {
		t.Fatalf("create without email: %v", err)
	}
	if err := s.CreateUser(ctx, user("3", "veli", "")); err != nil {
		t.Fatalf("second user without email must be allowed: %v", err)
	}
	if err := s.CreateUser(ctx, user("4", "ayse", "")); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("duplicate username: got %v", err)
	}
	if err := s.CreateUser(ctx, user("5", "other", "ayse@example.com")); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("duplicate email: got %v", err)
	}
	if n, _ := s.CountUsers(ctx); n != 3 {
		t.Fatalf("count = %d, want 3", n)
	}

	got, err := s.GetUser(ctx, "1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Username != "ayse" || got.Email != "ayse@example.com" || got.PasswordHash != "hash-1" ||
		!got.MustChangePassword || got.CreatedBy != "test" || !got.CreatedAt.Equal(user("1", "", "").CreatedAt) {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if got.LastLoginAt != nil || got.LockedUntil != nil {
		t.Fatalf("optional times must be nil: %+v", got)
	}

	byName, err := s.GetUserByUsername(ctx, "mehmet")
	if err != nil || byName.ID != "2" {
		t.Fatalf("get by username: %+v %v", byName, err)
	}
	if _, err := s.GetUserByUsername(ctx, "nobody"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get missing by username: %v", err)
	}
	if _, err := s.GetUser(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}

	login := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	lock := login.Add(15 * time.Minute)
	got.MustChangePassword = false
	got.Disabled = true
	got.FailedLogins = 3
	got.LastLoginAt = &login
	got.LockedUntil = &lock
	got.PasswordHash = "new-hash"
	if err := s.UpdateUser(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	upd, _ := s.GetUser(ctx, "1")
	if upd.MustChangePassword || !upd.Disabled || upd.FailedLogins != 3 || upd.PasswordHash != "new-hash" ||
		upd.LastLoginAt == nil || !upd.LastLoginAt.Equal(login) || upd.LockedUntil == nil || !upd.LockedUntil.Equal(lock) {
		t.Fatalf("update not persisted: %+v", upd)
	}

	// Clearing the optional times must persist too.
	upd.LastLoginAt, upd.LockedUntil = nil, nil
	if err := s.UpdateUser(ctx, upd); err != nil {
		t.Fatalf("update clearing times: %v", err)
	}
	if again, _ := s.GetUser(ctx, "1"); again.LastLoginAt != nil || again.LockedUntil != nil {
		t.Fatalf("times not cleared: %+v", again)
	}

	if err := s.UpdateUser(ctx, user("missing", "ghost", "")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}
	clash := user("2", "mehmet", "ayse@example.com")
	if err := s.UpdateUser(ctx, clash); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("update with taken email: %v", err)
	}
}

func sessions(t *testing.T, s store.Store) {
	ctx := context.Background()
	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Millisecond)
	now := time.Now().UTC().Truncate(time.Millisecond)
	mk := func(id, userID, hash, family string) domain.Session {
		return domain.Session{ID: id, UserID: userID, TokenHash: hash, CreatedAt: exp.Add(-time.Minute), ExpiresAt: exp, UserAgent: "ua", IP: "10.0.0.1", FamilyID: family}
	}

	for _, sess := range []domain.Session{mk("s1", "u1", "h1", "fam-a"), mk("s2", "u1", "h2", "fam-a"), mk("s3", "u2", "h3", "fam-b")} {
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatalf("create %s: %v", sess.ID, err)
		}
	}
	if err := s.CreateSession(ctx, mk("s4", "u2", "h1", "fam-c")); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("duplicate token hash: %v", err)
	}

	// The first exchange wins and returns the session as it was.
	got, err := s.ConsumeSession(ctx, "h1", now)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if got.ID != "s1" || got.UserID != "u1" || got.FamilyID != "fam-a" || got.UserAgent != "ua" || got.IP != "10.0.0.1" ||
		!got.ExpiresAt.Equal(exp) || got.UsedAt != nil {
		t.Fatalf("consume returned %+v", got)
	}

	// A second exchange of the same token is refused and says when the first one happened.
	again, err := s.ConsumeSession(ctx, "h1", now.Add(time.Minute))
	if !errors.Is(err, store.ErrSessionUsed) {
		t.Fatalf("a token must be exchangeable only once: %v", err)
	}
	if again.ID != "s1" || again.FamilyID != "fam-a" || again.UsedAt == nil || !again.UsedAt.Equal(now) {
		t.Fatalf("a refused exchange must report the first use, unchanged: %+v", again)
	}

	if _, err := s.ConsumeSession(ctx, "nope", now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown token: %v", err)
	}

	// Exactly one of several concurrent exchanges succeeds.
	if err := s.CreateSession(ctx, mk("s5", "u3", "h5", "fam-d")); err != nil {
		t.Fatal(err)
	}
	var wins, used atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch _, err := s.ConsumeSession(ctx, "h5", now); {
			case err == nil:
				wins.Add(1)
			case errors.Is(err, store.ErrSessionUsed):
				used.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 || used.Load() != 7 {
		t.Fatalf("concurrent exchanges: %d won, %d refused; want 1 and 7", wins.Load(), used.Load())
	}

	// Deleting is idempotent and works on used sessions as well.
	if err := s.DeleteSession(ctx, "h1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteSession(ctx, "h1"); err != nil {
		t.Fatalf("deleting a missing session must not fail: %v", err)
	}
	if _, err := s.ConsumeSession(ctx, "h1", now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted session still known: %v", err)
	}

	// A family goes away as a whole, other families stay, and an empty ID removes nothing.
	if err := s.DeleteFamily(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeSession(ctx, "h3", now); err != nil {
		t.Fatalf("an empty family ID must delete nothing: %v", err)
	}
	if err := s.DeleteFamily(ctx, "fam-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeSession(ctx, "h2", now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("the family's other session must be gone: %v", err)
	}

	if err := s.CreateSession(ctx, mk("s6", "u1", "h6", "fam-e")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUserSessions(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeSession(ctx, "h6", now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("user session survived: %v", err)
	}
	if _, err := s.ConsumeSession(ctx, "h5", now); !errors.Is(err, store.ErrSessionUsed) {
		t.Fatalf("other user's session must survive: %v", err)
	}
}
