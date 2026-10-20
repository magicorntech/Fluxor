package auth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
	"github.com/magicorntech/Fluxor/internal/store/memory"
)

const (
	testSecret = "0123456789abcdef0123456789abcdef"
	testPass   = "initial-password-1"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

type env struct {
	svc   *Service
	store *memory.Store
	clock *fakeClock
}

// newEnv returns a service with one user ("admin") whose password is testPass.
func newEnv(t *testing.T) *env {
	t.Helper()
	st := memory.New()
	clock := &fakeClock{t: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}
	svc, err := NewService(Options{
		Users: st, Sessions: st, Tokens: st, Secret: []byte(testSecret),
		Params: testParams, Now: clock.now,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Bootstrap(context.Background(), "admin", testPass); err != nil {
		t.Fatal(err)
	}
	return &env{svc: svc, store: st, clock: clock}
}

func (e *env) login(t *testing.T, password string) (Tokens, domain.User, error) {
	t.Helper()
	return e.svc.Login(context.Background(), "admin", password, SessionMeta{})
}

func TestNewServiceRejectsShortSecret(t *testing.T) {
	st := memory.New()
	if _, err := NewService(Options{Users: st, Sessions: st, Secret: []byte("short")}); err == nil {
		t.Fatal("expected an error for a short secret")
	}
}

func TestLoginSuccess(t *testing.T) {
	e := newEnv(t)
	tokens, u, err := e.svc.Login(context.Background(), "  ADMIN ", testPass, SessionMeta{UserAgent: "ua", IP: "1.2.3.4"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if u.Username != "admin" || u.LastLoginAt == nil || !u.LastLoginAt.Equal(e.clock.t) {
		t.Fatalf("unexpected user after login: %+v", u)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatal("tokens must be set")
	}
	if want := e.clock.t.Add(DefaultAccessTTL); !tokens.AccessExpiresAt.Equal(want) {
		t.Fatalf("access expiry = %v, want %v", tokens.AccessExpiresAt, want)
	}
	if want := e.clock.t.Add(DefaultRefreshTTL); !tokens.RefreshExpiresAt.Equal(want) {
		t.Fatalf("refresh expiry = %v, want %v", tokens.RefreshExpiresAt, want)
	}

	got, err := e.svc.Authenticate(context.Background(), tokens.AccessToken)
	if err != nil || got.ID != u.ID {
		t.Fatalf("authenticate: %+v %v", got, err)
	}
	// Only a hash of the refresh token may be stored.
	if _, err := e.store.ConsumeSession(context.Background(), tokens.RefreshToken, e.clock.t); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("the raw refresh token must not be usable as a storage key")
	}
}

func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	_, _, errWrong := e.login(t, "not the password")
	_, _, errUnknown := e.svc.Login(ctx, "nobody", testPass, SessionMeta{})
	if !errors.Is(errWrong, ErrInvalidCredentials) || !errors.Is(errUnknown, ErrInvalidCredentials) {
		t.Fatalf("got %v and %v, want ErrInvalidCredentials for both", errWrong, errUnknown)
	}
	if errWrong.Error() != errUnknown.Error() {
		t.Fatal("error messages must not reveal whether the user exists")
	}
}

func TestLockoutAfterRepeatedFailures(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < maxFailed; i++ {
		if _, _, err := e.login(t, "wrong password!"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	// Locked: even the right password is refused, with the same generic error.
	if _, _, err := e.login(t, testPass); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("locked account accepted the correct password: %v", err)
	}

	e.clock.advance(lockoutPeriod + time.Second)
	if _, _, err := e.login(t, testPass); err != nil {
		t.Fatalf("login after the lockout expired: %v", err)
	}
}

func TestSuccessfulLoginResetsFailureCounter(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < maxFailed-1; i++ {
		_, _, _ = e.login(t, "wrong password!")
	}
	if _, _, err := e.login(t, testPass); err != nil {
		t.Fatal(err)
	}
	// Another few failures must not trip the lock, because the counter was reset.
	for i := 0; i < maxFailed-1; i++ {
		_, _, _ = e.login(t, "wrong password!")
	}
	if _, _, err := e.login(t, testPass); err != nil {
		t.Fatalf("counter was not reset by a successful login: %v", err)
	}
}

func TestDisabledUser(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	tokens, u, err := e.login(t, testPass)
	if err != nil {
		t.Fatal(err)
	}

	u.Disabled = true
	if err := e.store.UpdateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.login(t, testPass); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled user logged in: %v", err)
	}
	if _, err := e.svc.Authenticate(ctx, tokens.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("existing access token still valid for a disabled user: %v", err)
	}
	if _, _, err := e.svc.Refresh(ctx, tokens.RefreshToken, SessionMeta{}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("refresh worked for a disabled user: %v", err)
	}
}

func TestAccessTokenValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	tokens, u, _ := e.login(t, testPass)

	t.Run("expired", func(t *testing.T) {
		e.clock.advance(DefaultAccessTTL + time.Second)
		defer e.clock.advance(-(DefaultAccessTTL + time.Second))
		if _, err := e.svc.Authenticate(ctx, tokens.AccessToken); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("expired token accepted: %v", err)
		}
	})

	claims := jwt.RegisteredClaims{
		Issuer: issuer, Subject: u.ID,
		ExpiresAt: jwt.NewNumericDate(e.clock.t.Add(time.Hour)),
	}
	forge := func(m jwt.SigningMethod, key any, c jwt.RegisteredClaims) string {
		s, err := jwt.NewWithClaims(m, c).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	noExp := claims
	noExp.ExpiresAt = nil
	otherIssuer := claims
	otherIssuer.Issuer = "someone-else"

	cases := map[string]string{
		"empty":           "",
		"garbage":         "not.a.jwt",
		"wrong secret":    forge(jwt.SigningMethodHS256, []byte("another-secret-another-secret-123"), claims),
		"alg none":        forge(jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, claims),
		"hs512":           forge(jwt.SigningMethodHS512, []byte(testSecret), claims),
		"no expiry":       forge(jwt.SigningMethodHS256, []byte(testSecret), noExp),
		"wrong issuer":    forge(jwt.SigningMethodHS256, []byte(testSecret), otherIssuer),
		"tampered claims": tampered(tokens.AccessToken),
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := e.svc.Authenticate(ctx, tok); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("accepted a bad token: %v", err)
			}
		})
	}

	t.Run("unknown subject", func(t *testing.T) {
		c := claims
		c.Subject = "no-such-user"
		if _, err := e.svc.Authenticate(ctx, forge(jwt.SigningMethodHS256, []byte(testSecret), c)); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("accepted a token for an unknown user: %v", err)
		}
	})
}

// tampered flips a character in the payload so the signature no longer matches.
func tampered(tok string) string {
	parts := strings.Split(tok, ".")
	p := []byte(parts[1])
	if p[0] == 'a' {
		p[0] = 'b'
	} else {
		p[0] = 'a'
	}
	parts[1] = string(p)
	return strings.Join(parts, ".")
}

func TestRefreshRotation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first, _, _ := e.login(t, testPass)

	e.clock.advance(time.Minute)
	second, u, err := e.svc.Refresh(ctx, first.RefreshToken, SessionMeta{})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken || second.AccessToken == first.AccessToken {
		t.Fatal("refresh must issue new tokens")
	}
	if u.Username != "admin" {
		t.Fatalf("unexpected user: %+v", u)
	}

	if _, _, err := e.svc.Refresh(ctx, first.RefreshToken, SessionMeta{}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("a used refresh token must not work twice: %v", err)
	}
	if _, _, err := e.svc.Refresh(ctx, second.RefreshToken, SessionMeta{}); err != nil {
		t.Fatalf("the new refresh token must work: %v", err)
	}
}

func TestRefreshRejectsBadTokens(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	tokens, _, _ := e.login(t, testPass)

	if _, _, err := e.svc.Refresh(ctx, "", SessionMeta{}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("empty: %v", err)
	}
	if _, _, err := e.svc.Refresh(ctx, "made-up-token", SessionMeta{}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("unknown: %v", err)
	}

	e.clock.advance(DefaultRefreshTTL + time.Second)
	if _, _, err := e.svc.Refresh(ctx, tokens.RefreshToken, SessionMeta{}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired: %v", err)
	}
}

func TestLogout(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	tokens, _, _ := e.login(t, testPass)

	if err := e.svc.Logout(ctx, tokens.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Refresh(ctx, tokens.RefreshToken, SessionMeta{}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("refresh worked after logout: %v", err)
	}
	if err := e.svc.Logout(ctx, tokens.RefreshToken); err != nil {
		t.Fatalf("logging out twice must not fail: %v", err)
	}
	if err := e.svc.Logout(ctx, ""); err != nil {
		t.Fatalf("logging out without a token must not fail: %v", err)
	}
}

func TestChangePassword(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	old, u, _ := e.login(t, testPass)
	if !u.MustChangePassword {
		t.Fatal("bootstrap admin must have to change the password")
	}

	if _, _, err := e.svc.ChangePassword(ctx, u.ID, "wrong current!", "brand-new-password", SessionMeta{}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong current password: %v", err)
	}
	if _, _, err := e.svc.ChangePassword(ctx, u.ID, testPass, "short", SessionMeta{}); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("weak new password: %v", err)
	}
	if _, _, err := e.svc.ChangePassword(ctx, u.ID, testPass, testPass, SessionMeta{}); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("unchanged password: %v", err)
	}

	fresh, updated, err := e.svc.ChangePassword(ctx, u.ID, testPass, "brand-new-password", SessionMeta{})
	if err != nil {
		t.Fatalf("change password: %v", err)
	}
	if updated.MustChangePassword {
		t.Fatal("MustChangePassword must be cleared")
	}
	if _, _, err := e.login(t, testPass); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password still works: %v", err)
	}
	if _, _, err := e.login(t, "brand-new-password"); err != nil {
		t.Fatalf("new password does not work: %v", err)
	}
	if _, _, err := e.svc.Refresh(ctx, old.RefreshToken, SessionMeta{}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("sessions from before the change must be revoked: %v", err)
	}
	if _, _, err := e.svc.Refresh(ctx, fresh.RefreshToken, SessionMeta{}); err != nil {
		t.Fatalf("the caller's new session must work: %v", err)
	}
}

func TestBootstrap(t *testing.T) {
	ctx := context.Background()
	newSvc := func(t *testing.T) (*Service, *memory.Store) {
		st := memory.New()
		svc, err := NewService(Options{Users: st, Sessions: st, Tokens: st, Secret: []byte(testSecret), Params: testParams,
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		if err != nil {
			t.Fatal(err)
		}
		return svc, st
	}

	t.Run("generates a password once", func(t *testing.T) {
		svc, st := newSvc(t)
		res, err := svc.Bootstrap(ctx, "", "")
		if err != nil || !res.Created || res.Username != "admin" || len(res.Password) != 20 {
			t.Fatalf("bootstrap: %+v %v", res, err)
		}
		u, _ := st.GetUserByUsername(ctx, "admin")
		if !u.MustChangePassword || u.CreatedBy != "bootstrap" || strings.Contains(u.PasswordHash, res.Password) {
			t.Fatalf("unexpected bootstrap user: %+v", u)
		}
		if _, _, err := svc.Login(ctx, "admin", res.Password, SessionMeta{}); err != nil {
			t.Fatalf("generated password does not work: %v", err)
		}

		again, err := svc.Bootstrap(ctx, "", "")
		if err != nil || again.Created || again.Password != "" {
			t.Fatalf("second bootstrap must do nothing: %+v %v", again, err)
		}
		if n, _ := st.CountUsers(ctx); n != 1 {
			t.Fatalf("users = %d, want 1", n)
		}
	})

	t.Run("uses the given credentials", func(t *testing.T) {
		svc, _ := newSvc(t)
		res, err := svc.Bootstrap(ctx, "Root", "a-long-enough-password")
		if err != nil || !res.Created || res.Username != "root" || res.Password != "" {
			t.Fatalf("bootstrap: %+v %v", res, err)
		}
		if _, _, err := svc.Login(ctx, "root", "a-long-enough-password", SessionMeta{}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects weak or invalid input", func(t *testing.T) {
		svc, st := newSvc(t)
		if _, err := svc.Bootstrap(ctx, "admin", "short"); !errors.Is(err, ErrWeakPassword) {
			t.Fatalf("weak password: %v", err)
		}
		if _, err := svc.Bootstrap(ctx, "a b", "a-long-enough-password"); err == nil {
			t.Fatal("invalid username accepted")
		}
		if n, _ := st.CountUsers(ctx); n != 0 {
			t.Fatalf("a rejected bootstrap must not create a user, got %d", n)
		}
	})
}

func TestCreateUser(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	t.Run("generates a temporary password", func(t *testing.T) {
		u, pw, err := e.svc.CreateUser(ctx, NewUser{Username: "  Ayse.Yilmaz ", Email: " Ayse@Example.COM ", DisplayName: " Ayşe ", CreatedBy: "admin-id"})
		if err != nil {
			t.Fatal(err)
		}
		if u.Username != "ayse.yilmaz" || u.Email != "ayse@example.com" || u.DisplayName != "Ayşe" ||
			!u.MustChangePassword || u.CreatedBy != "admin-id" || len(pw) != 20 {
			t.Fatalf("unexpected user or password: %+v %q", u, pw)
		}
		if strings.Contains(u.PasswordHash, pw) {
			t.Fatal("the password must be stored hashed")
		}
		if _, got, err := e.svc.Login(ctx, "ayse.yilmaz", pw, SessionMeta{}); err != nil || got.ID != u.ID {
			t.Fatalf("the temporary password must work: %v", err)
		}
	})

	t.Run("uses the given password", func(t *testing.T) {
		u, pw, err := e.svc.CreateUser(ctx, NewUser{Username: "mehmet", Password: "given-password-123"})
		if err != nil || pw != "" {
			t.Fatalf("a given password must not be echoed back: %q %v", pw, err)
		}
		if _, _, err := e.svc.Login(ctx, u.Username, "given-password-123", SessionMeta{}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects bad input", func(t *testing.T) {
		if _, _, err := e.svc.CreateUser(ctx, NewUser{Username: "a b"}); !errors.Is(err, ErrInvalidUser) {
			t.Fatalf("bad username: %v", err)
		}
		if _, _, err := e.svc.CreateUser(ctx, NewUser{Username: "valid.name", Email: "nope"}); !errors.Is(err, ErrInvalidUser) {
			t.Fatalf("bad email: %v", err)
		}
		if _, _, err := e.svc.CreateUser(ctx, NewUser{Username: "valid.name", DisplayName: strings.Repeat("x", 101)}); !errors.Is(err, ErrInvalidUser) {
			t.Fatalf("long display name: %v", err)
		}
		if _, _, err := e.svc.CreateUser(ctx, NewUser{Username: "valid.name", Password: "short"}); !errors.Is(err, ErrWeakPassword) {
			t.Fatalf("weak password: %v", err)
		}
		if _, err := e.store.GetUserByUsername(ctx, "valid.name"); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("a rejected user must not be stored")
		}
	})

	t.Run("rejects duplicates", func(t *testing.T) {
		if _, _, err := e.svc.CreateUser(ctx, NewUser{Username: "ADMIN"}); !errors.Is(err, store.ErrAlreadyExists) {
			t.Fatalf("duplicate username: %v", err)
		}
		if _, _, err := e.svc.CreateUser(ctx, NewUser{Username: "first", Email: "dup@example.com"}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := e.svc.CreateUser(ctx, NewUser{Username: "second", Email: "DUP@example.com"}); !errors.Is(err, store.ErrAlreadyExists) {
			t.Fatalf("duplicate email: %v", err)
		}
	})
}

func TestResetPassword(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	tokens, u, err := e.login(t, testPass)
	if err != nil {
		t.Fatal(err)
	}
	// Lock the account first, to prove a reset clears the lockout.
	for i := 0; i < maxFailed; i++ {
		_, _, _ = e.login(t, "wrong password!")
	}

	pw, err := e.svc.ResetPassword(ctx, u.ID, "")
	if err != nil || len(pw) != 20 {
		t.Fatalf("reset: %q %v", pw, err)
	}
	if _, _, err := e.login(t, testPass); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("the old password must stop working: %v", err)
	}
	_, got, err := e.login(t, pw)
	if err != nil {
		t.Fatalf("the new password must work, lockout cleared: %v", err)
	}
	if !got.MustChangePassword {
		t.Fatal("a reset password must be changed at next login")
	}
	if _, _, err := e.svc.Refresh(ctx, tokens.RefreshToken, SessionMeta{}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("sessions from before the reset must end: %v", err)
	}

	if given, err := e.svc.ResetPassword(ctx, u.ID, "chosen-password-99"); err != nil || given != "" {
		t.Fatalf("a given password is not echoed back: %q %v", given, err)
	}
	if _, err := e.svc.ResetPassword(ctx, u.ID, "short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("weak password: %v", err)
	}
	if _, err := e.svc.ResetPassword(ctx, "missing", ""); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
}

func TestLoginFailureReasonsAreRecordedButIndistinguishable(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	messages := map[string]bool{}
	check := func(label, wantReason string, err error) {
		t.Helper()
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("%s: must still match ErrInvalidCredentials, got %v", label, err)
		}
		if got := FailureReason(err); got != wantReason {
			t.Fatalf("%s: reason %q, want %q", label, got, wantReason)
		}
		messages[err.Error()] = true
	}

	_, _, err := e.svc.Login(ctx, "nobody", "whatever-it-is-123", SessionMeta{})
	check("unknown user", ReasonUnknownUser, err)
	_, _, err = e.login(t, "wrong password!")
	check("wrong password", ReasonBadPassword, err)

	for i := 0; i < maxFailed; i++ {
		_, _, _ = e.login(t, "wrong password!")
	}
	_, _, err = e.login(t, testPass)
	check("locked, even with the right password", ReasonLocked, err)

	e.clock.advance(lockoutPeriod + time.Second)
	user, _ := e.store.GetUserByUsername(ctx, "admin")
	user.Disabled = true
	if err := e.store.UpdateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	_, _, err = e.login(t, testPass)
	check("disabled", ReasonDisabled, err)

	if len(messages) != 1 {
		t.Fatalf("the message must be the same for every reason, got %v", messages)
	}
	if FailureReason(ErrInvalidToken) != "" || FailureReason(nil) != "" {
		t.Fatal("FailureReason must be empty for other errors")
	}
}

func TestRefreshTokenReuseRevokesTheFamily(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first, _, _ := e.login(t, testPass)
	other, _, _ := e.login(t, testPass) // a second, unrelated login (another device)

	e.clock.advance(time.Minute)
	second, _, err := e.svc.Refresh(ctx, first.RefreshToken, SessionMeta{})
	if err != nil {
		t.Fatal(err)
	}
	e.clock.advance(time.Minute)
	third, _, err := e.svc.Refresh(ctx, second.RefreshToken, SessionMeta{})
	if err != nil {
		t.Fatalf("the chain must keep working: %v", err)
	}

	// Later, the very first token shows up again: it must have leaked.
	e.clock.advance(time.Minute)
	_, _, err = e.svc.Refresh(ctx, first.RefreshToken, SessionMeta{})
	if !errors.Is(err, ErrTokenReuse) || !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("reuse must be detected and still look like an invalid token: %v", err)
	}
	owner, _ := e.store.GetUserByUsername(ctx, "admin")
	if got := ReusedBy(err); got != owner.ID {
		t.Fatalf("the error must say whose session it was: %q, want %q", got, owner.ID)
	}
	if ReusedBy(ErrInvalidToken) != "" || ReusedBy(nil) != "" {
		t.Fatal("ReusedBy must be empty for other errors")
	}
	// The whole family is gone, including the newest, legitimate token.
	if _, _, err := e.svc.Refresh(ctx, third.RefreshToken, SessionMeta{}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("the successor must be revoked too: %v", err)
	}
	// Another login of the same user is not part of this family.
	if _, _, err := e.svc.Refresh(ctx, other.RefreshToken, SessionMeta{}); err != nil {
		t.Fatalf("an unrelated session must survive: %v", err)
	}
}

func TestRefreshTokenReuseWithinGraceIsOnlyRefused(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first, _, _ := e.login(t, testPass)

	second, _, err := e.svc.Refresh(ctx, first.RefreshToken, SessionMeta{})
	if err != nil {
		t.Fatal(err)
	}
	// Two browser tabs sending the same cookie a moment apart: the loser is refused, nothing is revoked.
	e.clock.advance(reuseGrace - time.Second)
	_, _, err = e.svc.Refresh(ctx, first.RefreshToken, SessionMeta{})
	if !errors.Is(err, ErrInvalidToken) || errors.Is(err, ErrTokenReuse) {
		t.Fatalf("a quick repeat must be refused without being treated as theft: %v", err)
	}
	if _, _, err := e.svc.Refresh(ctx, second.RefreshToken, SessionMeta{}); err != nil {
		t.Fatalf("the family must stay intact after a racing repeat: %v", err)
	}
}

func TestExpiredUsedTokenIsNotTheft(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first, _, _ := e.login(t, testPass)
	if _, _, err := e.svc.Refresh(ctx, first.RefreshToken, SessionMeta{}); err != nil {
		t.Fatal(err)
	}

	e.clock.advance(DefaultRefreshTTL + time.Hour)
	_, _, err := e.svc.Refresh(ctx, first.RefreshToken, SessionMeta{})
	if !errors.Is(err, ErrInvalidToken) || errors.Is(err, ErrTokenReuse) {
		t.Fatalf("an old, expired token is just invalid: %v", err)
	}
}

func TestSuccessorSessionsKeepTheFamily(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	first, _, _ := e.login(t, testPass)
	second, _, _ := e.svc.Refresh(ctx, first.RefreshToken, SessionMeta{})

	a, _ := e.store.ConsumeSession(ctx, hashToken(first.RefreshToken), e.clock.t)
	b, _ := e.store.ConsumeSession(ctx, hashToken(second.RefreshToken), e.clock.t)
	if a.FamilyID == "" || a.FamilyID != b.FamilyID {
		t.Fatalf("a refreshed session must stay in its family: %q vs %q", a.FamilyID, b.FamilyID)
	}
	other, _, _ := e.login(t, testPass)
	c, _ := e.store.ConsumeSession(ctx, hashToken(other.RefreshToken), e.clock.t)
	if c.FamilyID == a.FamilyID {
		t.Fatal("a new login must start a new family")
	}
}

func TestAPITokens(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, u, _ := e.login(t, testPass)

	secret, tok, err := e.svc.CreateToken(ctx, u.ID, "  ci pipeline ", 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, "flx_") || len(secret) != len("flx_")+43 {
		t.Fatalf("unexpected secret format: %q", secret)
	}
	if tok.Name != "ci pipeline" || tok.UserID != u.ID || tok.Prefix != secret[:tokenPrefixLen] ||
		!tok.ExpiresAt.Equal(e.clock.t.Add(30*24*time.Hour)) {
		t.Fatalf("unexpected token record: %+v", tok)
	}
	if strings.Contains(tok.TokenHash, secret) || tok.TokenHash == secret || strings.Contains(tok.Prefix, secret[tokenPrefixLen:]) {
		t.Fatal("only a hash and a short prefix may be stored")
	}

	t.Run("authenticates as its owner", func(t *testing.T) {
		got, cred, err := e.svc.AuthenticateBearer(ctx, secret)
		if err != nil || got.ID != u.ID || cred.Kind != CredentialToken || cred.TokenID != tok.ID {
			t.Fatalf("got %+v %+v %v", got, cred, err)
		}
	})
	t.Run("a session token still authenticates as a session", func(t *testing.T) {
		tokens, _, _ := e.login(t, testPass)
		got, cred, err := e.svc.AuthenticateBearer(ctx, tokens.AccessToken)
		if err != nil || got.ID != u.ID || cred.Kind != CredentialSession || cred.TokenID != "" {
			t.Fatalf("got %+v %+v %v", got, cred, err)
		}
	})
	t.Run("rejects wrong and malformed tokens", func(t *testing.T) {
		for _, bad := range []string{"flx_", "flx_wrong", secret + "x", secret[:len(secret)-1], "flx_" + strings.Repeat("A", 43)} {
			if _, _, err := e.svc.AuthenticateBearer(ctx, bad); !errors.Is(err, ErrInvalidToken) {
				t.Errorf("%q: %v", bad, err)
			}
		}
		// A token-shaped string must never be mistaken for a signed access token.
		forged, _ := e.svc.Authenticate(ctx, secret)
		if forged.ID != "" {
			t.Fatal("an API token is not an access token")
		}
	})
	t.Run("last use is recorded, at most once a minute", func(t *testing.T) {
		stored, _ := e.store.GetTokenByHash(ctx, hashToken(secret))
		if stored.LastUsedAt == nil {
			t.Fatal("the first use must be recorded")
		}
		first := *stored.LastUsedAt
		e.clock.advance(10 * time.Second)
		_, _, _ = e.svc.AuthenticateBearer(ctx, secret)
		stored, _ = e.store.GetTokenByHash(ctx, hashToken(secret))
		if !stored.LastUsedAt.Equal(first) {
			t.Fatal("a use within a minute must not write again")
		}
		e.clock.advance(time.Minute)
		_, _, _ = e.svc.AuthenticateBearer(ctx, secret)
		stored, _ = e.store.GetTokenByHash(ctx, hashToken(secret))
		if !stored.LastUsedAt.After(first) {
			t.Fatal("a later use must update the time")
		}
	})
	t.Run("expires", func(t *testing.T) {
		short, _, err := e.svc.CreateToken(ctx, u.ID, "short", MinTokenTTL)
		if err != nil {
			t.Fatal(err)
		}
		e.clock.advance(MinTokenTTL + time.Second)
		if _, _, err := e.svc.AuthenticateBearer(ctx, short); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("an expired token must be rejected: %v", err)
		}
	})
	t.Run("stops working when the user is disabled", func(t *testing.T) {
		secret2, _, _ := e.svc.CreateToken(ctx, u.ID, "temp", 48*time.Hour)
		user, _ := e.store.GetUser(ctx, u.ID)
		user.Disabled = true
		_ = e.store.UpdateUser(ctx, user)
		if _, _, err := e.svc.AuthenticateBearer(ctx, secret2); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("a disabled user's token must be rejected: %v", err)
		}
		user.Disabled = false
		_ = e.store.UpdateUser(ctx, user)
	})
}

func TestCreateTokenValidationAndLimit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, u, _ := e.login(t, testPass)

	for name, tt := range map[string]struct {
		name string
		ttl  time.Duration
	}{
		"empty name":       {"", 48 * time.Hour},
		"long name":        {strings.Repeat("x", 65), 48 * time.Hour},
		"control char":     {"a\tb", 48 * time.Hour},
		"too short a life": {"x", time.Hour},
		"zero life":        {"x", 0},
		"too long a life":  {"x", MaxTokenTTL + time.Hour},
	} {
		if _, _, err := e.svc.CreateToken(ctx, u.ID, tt.name, tt.ttl); !errors.Is(err, ErrInvalidTokenRequest) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if n, _ := e.store.CountTokens(ctx, u.ID); n != 0 {
		t.Fatalf("rejected requests must not create tokens, got %d", n)
	}

	for i := 0; i < domain.MaxTokensPerUser; i++ {
		if _, _, err := e.svc.CreateToken(ctx, u.ID, "t", 48*time.Hour); err != nil {
			t.Fatalf("token %d: %v", i, err)
		}
	}
	if _, _, err := e.svc.CreateToken(ctx, u.ID, "one too many", 48*time.Hour); !errors.Is(err, ErrTokenLimit) {
		t.Fatalf("got %v", err)
	}
	// The limit is per user.
	other, _, err := e.svc.CreateUser(ctx, NewUser{Username: "other.user"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.CreateToken(ctx, other.ID, "mine", 48*time.Hour); err != nil {
		t.Fatalf("another user is unaffected: %v", err)
	}
}

func TestRevokeToken(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, u, _ := e.login(t, testPass)
	other, _, _ := e.svc.CreateUser(ctx, NewUser{Username: "other.user"})
	secret, tok, _ := e.svc.CreateToken(ctx, u.ID, "mine", 48*time.Hour)

	if err := e.svc.RevokeToken(ctx, other.ID, tok.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("another user must not revoke it: %v", err)
	}
	if _, _, err := e.svc.AuthenticateBearer(ctx, secret); err != nil {
		t.Fatalf("it must still work: %v", err)
	}
	if err := e.svc.RevokeToken(ctx, u.ID, tok.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.AuthenticateBearer(ctx, secret); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("a revoked token must be rejected: %v", err)
	}
	list, _ := e.svc.ListTokens(ctx, u.ID)
	if len(list) != 0 {
		t.Fatalf("it must be gone from the list: %v", list)
	}
}

func TestResetPasswordRevokesAPITokens(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, u, _ := e.login(t, testPass)
	secret, _, _ := e.svc.CreateToken(ctx, u.ID, "mine", 48*time.Hour)

	if _, err := e.svc.ResetPassword(ctx, u.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.AuthenticateBearer(ctx, secret); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("a reset must revoke API tokens: %v", err)
	}
}

func TestChangeOwnPasswordKeepsAPITokens(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, u, _ := e.login(t, testPass)
	secret, _, _ := e.svc.CreateToken(ctx, u.ID, "ci", 48*time.Hour)

	if _, _, err := e.svc.ChangePassword(ctx, u.ID, testPass, "a-new-password-99", SessionMeta{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.AuthenticateBearer(ctx, secret); err != nil {
		t.Fatalf("changing your own password must not break your CI tokens: %v", err)
	}
}
