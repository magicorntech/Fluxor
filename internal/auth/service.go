package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

const (
	issuer        = "fluxor"
	minSecretLen  = 32
	maxFailed     = 5
	lockoutPeriod = 15 * time.Minute

	DefaultAccessTTL  = 15 * time.Minute
	DefaultRefreshTTL = 7 * 24 * time.Hour
)

var (
	// ErrInvalidCredentials is returned for every failed login, whatever the reason
	// (unknown user, wrong password, locked or disabled account), so callers cannot tell them apart.
	ErrInvalidCredentials = errors.New("invalid username or password")
	// ErrInvalidToken is returned for a bad, expired or revoked access or refresh token.
	ErrInvalidToken = errors.New("invalid or expired token")
	// ErrInvalidUser wraps the reason a new user's fields were rejected.
	ErrInvalidUser = errors.New("invalid user")
	// ErrTokenReuse is returned (it also matches ErrInvalidToken) when a refresh token that was already
	// exchanged is presented again some time later. That means the token leaked, so every session that
	// descends from the same login has been revoked.
	ErrTokenReuse = fmt.Errorf("%w: refresh token used twice", ErrInvalidToken)
	// ErrInvalidTokenRequest wraps the reason an API token request was rejected.
	ErrInvalidTokenRequest = errors.New("invalid token request")
	// ErrTokenLimit is returned when the user already holds the maximum number of API tokens.
	ErrTokenLimit = errors.New("token limit reached")
)

// TokenReuse is the error Refresh returns when it detects a refresh token being used a second time.
// It matches ErrTokenReuse and ErrInvalidToken, and says whose session it was, for the audit log.
type TokenReuse struct{ UserID string }

func (e *TokenReuse) Error() string { return ErrTokenReuse.Error() }

func (e *TokenReuse) Is(target error) bool {
	return target == ErrTokenReuse || target == ErrInvalidToken
}

// ReusedBy returns the owner of the session whose refresh token was reused, or "" if err is not a TokenReuse.
func ReusedBy(err error) string {
	var tr *TokenReuse
	if errors.As(err, &tr) {
		return tr.UserID
	}
	return ""
}

// Reasons a login can fail. They go into the audit log only; the client always gets the same answer.
const (
	ReasonUnknownUser = "unknown_user"
	ReasonBadPassword = "bad_password"
	ReasonLocked      = "locked"
	ReasonDisabled    = "disabled"
)

// LoginFailure is what a failed login returns. It matches ErrInvalidCredentials and says nothing more
// to the caller, but carries the real reason for the audit log. Never show the reason to the client.
type LoginFailure struct{ Reason string }

func (e *LoginFailure) Error() string { return ErrInvalidCredentials.Error() }

func (e *LoginFailure) Is(target error) bool { return target == ErrInvalidCredentials }

// FailureReason returns why a login failed, or "" if err is not a LoginFailure.
func FailureReason(err error) string {
	var lf *LoginFailure
	if errors.As(err, &lf) {
		return lf.Reason
	}
	return ""
}

// A used refresh token that shows up again within this time is most likely two requests racing with
// the same cookie (two browser tabs), not theft, so it is only refused.
const reuseGrace = 10 * time.Second

// Token lifetime bounds, and how often last use is written.
const (
	MinTokenTTL    = 24 * time.Hour
	MaxTokenTTL    = 365 * 24 * time.Hour
	touchInterval  = time.Minute
	tokenPrefixLen = 8 // characters of the secret kept to recognize a token in a list
)

type Options struct {
	Users      store.UserStore
	Sessions   store.SessionStore
	Tokens     store.TokenStore
	Secret     []byte // HMAC key for access tokens, at least 32 bytes
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	Params     Params           // argon2id parameters; zero value means DefaultParams
	Now        func() time.Time // clock, overridable in tests
	Logger     *slog.Logger
}

type Service struct {
	users      store.UserStore
	sessions   store.SessionStore
	tokens     store.TokenStore
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	hasher     Hasher
	dummyHash  string // verified against when the user is unknown, to keep timing uniform
	now        func() time.Time
	log        *slog.Logger
}

func NewService(o Options) (*Service, error) {
	if len(o.Secret) < minSecretLen {
		return nil, fmt.Errorf("access token secret must be at least %d bytes", minSecretLen)
	}
	s := &Service{
		users: o.Users, sessions: o.Sessions, tokens: o.Tokens, secret: o.Secret,
		accessTTL: o.AccessTTL, refreshTTL: o.RefreshTTL,
		now: o.Now, log: o.Logger,
	}
	if s.accessTTL == 0 {
		s.accessTTL = DefaultAccessTTL
	}
	if s.refreshTTL == 0 {
		s.refreshTTL = DefaultRefreshTTL
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	params := o.Params
	if params == (Params{}) {
		params = DefaultParams
	}
	s.hasher = NewHasher(params)
	dummy, err := s.hasher.Hash("fluxor-dummy-password")
	if err != nil {
		return nil, err
	}
	s.dummyHash = dummy
	return s, nil
}

// Tokens is the pair handed to a client after login or refresh.
type Tokens struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// SessionMeta is recorded with the session for later inspection.
type SessionMeta struct {
	UserAgent string
	IP        string
}

// Login verifies the credentials and opens a new session.
func (s *Service) Login(ctx context.Context, username, password string, meta SessionMeta) (Tokens, domain.User, error) {
	u, err := s.users.GetUserByUsername(ctx, domain.NormalizeUsername(username))
	if errors.Is(err, store.ErrNotFound) {
		_, _ = s.hasher.Verify(s.dummyHash, password)
		return Tokens{}, domain.User{}, &LoginFailure{Reason: ReasonUnknownUser}
	}
	if err != nil {
		return Tokens{}, domain.User{}, err
	}

	now := s.now()
	locked := u.LockedUntil != nil && now.Before(*u.LockedUntil)
	ok, verr := s.hasher.Verify(u.PasswordHash, password)
	if verr != nil {
		s.log.Error("stored password hash is unusable", "user", u.ID, "err", verr)
	}
	if locked || u.Disabled || !ok {
		reason := ReasonBadPassword
		switch {
		case locked:
			reason = ReasonLocked
		case u.Disabled:
			reason = ReasonDisabled
		default:
			s.recordFailure(ctx, u, now)
		}
		return Tokens{}, domain.User{}, &LoginFailure{Reason: reason}
	}

	u.FailedLogins = 0
	u.LockedUntil = nil
	u.LastLoginAt = &now
	if err := s.users.UpdateUser(ctx, u); err != nil {
		return Tokens{}, domain.User{}, err
	}
	t, err := s.issue(ctx, u, meta, "")
	return t, u, err
}

func (s *Service) recordFailure(ctx context.Context, u domain.User, now time.Time) {
	u.FailedLogins++
	if u.FailedLogins >= maxFailed {
		until := now.Add(lockoutPeriod)
		u.LockedUntil = &until
		u.FailedLogins = 0
		s.log.Warn("account locked after repeated failed logins", "user", u.ID, "until", until)
	}
	if err := s.users.UpdateUser(ctx, u); err != nil {
		s.log.Error("recording failed login", "user", u.ID, "err", err)
	}
}

// Refresh exchanges a refresh token for a new token pair. Each refresh token works once. Presenting
// a used one again is refused, and if it happens after the grace period it also revokes the whole
// family of sessions it belongs to (ErrTokenReuse), because then two parties hold the same token.
func (s *Service) Refresh(ctx context.Context, refreshToken string, meta SessionMeta) (Tokens, domain.User, error) {
	if refreshToken == "" {
		return Tokens{}, domain.User{}, ErrInvalidToken
	}
	now := s.now()
	sess, err := s.sessions.ConsumeSession(ctx, hashToken(refreshToken), now)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return Tokens{}, domain.User{}, ErrInvalidToken
	case errors.Is(err, store.ErrSessionUsed):
		if !now.Before(sess.ExpiresAt) || (sess.UsedAt != nil && now.Sub(*sess.UsedAt) <= reuseGrace) {
			return Tokens{}, domain.User{}, ErrInvalidToken
		}
		if err := s.sessions.DeleteFamily(ctx, sess.FamilyID); err != nil {
			s.log.Error("revoking a session family after token reuse", "user", sess.UserID, "err", err)
		}
		s.log.Warn("refresh token reuse detected; the session family was revoked", "user", sess.UserID, "ip", meta.IP)
		return Tokens{}, domain.User{}, &TokenReuse{UserID: sess.UserID}
	case err != nil:
		return Tokens{}, domain.User{}, err
	}
	if !now.Before(sess.ExpiresAt) {
		return Tokens{}, domain.User{}, ErrInvalidToken
	}
	u, err := s.users.GetUser(ctx, sess.UserID)
	if errors.Is(err, store.ErrNotFound) {
		return Tokens{}, domain.User{}, ErrInvalidToken
	}
	if err != nil {
		return Tokens{}, domain.User{}, err
	}
	if u.Disabled {
		_ = s.sessions.DeleteUserSessions(ctx, u.ID)
		return Tokens{}, domain.User{}, ErrInvalidToken
	}
	t, err := s.issue(ctx, u, meta, sess.FamilyID)
	return t, u, err
}

// Logout ends the session behind a refresh token. Unknown tokens are ignored.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	return s.sessions.DeleteSession(ctx, hashToken(refreshToken))
}

// Authenticate validates an access token and returns the user it belongs to.
// The user is loaded on every call, so disabling an account takes effect immediately.
func (s *Service) Authenticate(ctx context.Context, accessToken string) (domain.User, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(accessToken, &claims,
		func(*jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(s.now),
	)
	if err != nil || claims.Subject == "" {
		return domain.User{}, ErrInvalidToken
	}
	u, err := s.users.GetUser(ctx, claims.Subject)
	if errors.Is(err, store.ErrNotFound) {
		return domain.User{}, ErrInvalidToken
	}
	if err != nil {
		return domain.User{}, err
	}
	if u.Disabled {
		return domain.User{}, ErrInvalidToken
	}
	return u, nil
}

// ChangePassword sets a new password after verifying the current one, ends all of the user's
// sessions and opens a fresh one for the caller.
func (s *Service) ChangePassword(ctx context.Context, userID, current, next string, meta SessionMeta) (Tokens, domain.User, error) {
	u, err := s.users.GetUser(ctx, userID)
	if err != nil {
		return Tokens{}, domain.User{}, err
	}
	ok, _ := s.hasher.Verify(u.PasswordHash, current)
	if !ok {
		return Tokens{}, domain.User{}, ErrInvalidCredentials
	}
	if err := ValidatePassword(next); err != nil {
		return Tokens{}, domain.User{}, err
	}
	if next == current {
		return Tokens{}, domain.User{}, fmt.Errorf("%w: the new password must differ from the current one", ErrWeakPassword)
	}
	hash, err := s.hasher.Hash(next)
	if err != nil {
		return Tokens{}, domain.User{}, err
	}
	u.PasswordHash = hash
	u.MustChangePassword = false
	u.FailedLogins = 0
	u.LockedUntil = nil
	if err := s.users.UpdateUser(ctx, u); err != nil {
		return Tokens{}, domain.User{}, err
	}
	if err := s.sessions.DeleteUserSessions(ctx, u.ID); err != nil {
		return Tokens{}, domain.User{}, err
	}
	t, err := s.issue(ctx, u, meta, "")
	return t, u, err
}

// NewUser describes an account an administrator creates.
type NewUser struct {
	Username    string
	Email       string
	DisplayName string
	// Password is the temporary password. Empty means "generate one".
	Password  string
	CreatedBy string
}

// CreateUser adds an account that must change its password at first login. It returns the
// generated password when none was given; that is the only time it can be read.
// Errors: ErrInvalidUser, ErrWeakPassword or store.ErrAlreadyExists (username or email taken).
func (s *Service) CreateUser(ctx context.Context, in NewUser) (domain.User, string, error) {
	u := domain.User{
		ID:                 newID(),
		Username:           domain.NormalizeUsername(in.Username),
		Email:              strings.ToLower(strings.TrimSpace(in.Email)),
		DisplayName:        strings.TrimSpace(in.DisplayName),
		MustChangePassword: true,
		CreatedAt:          s.now().UTC(),
		CreatedBy:          in.CreatedBy,
	}
	if err := u.Validate(); err != nil {
		return domain.User{}, "", fmt.Errorf("%w: %v", ErrInvalidUser, err)
	}
	if len(u.DisplayName) > 100 {
		return domain.User{}, "", fmt.Errorf("%w: display name must be at most 100 characters", ErrInvalidUser)
	}

	password, generated, err := s.passwordOrGenerate(in.Password)
	if err != nil {
		return domain.User{}, "", err
	}
	if u.PasswordHash, err = s.hasher.Hash(password); err != nil {
		return domain.User{}, "", err
	}
	if err := s.users.CreateUser(ctx, u); err != nil {
		return domain.User{}, "", err
	}
	return u, generated, nil
}

// ResetPassword sets a temporary password, forces a change at next login, clears any lockout and
// ends all of the user's sessions. It returns the generated password when none was given.
func (s *Service) ResetPassword(ctx context.Context, userID, password string) (string, error) {
	u, err := s.users.GetUser(ctx, userID)
	if err != nil {
		return "", err
	}
	password, generated, err := s.passwordOrGenerate(password)
	if err != nil {
		return "", err
	}
	if u.PasswordHash, err = s.hasher.Hash(password); err != nil {
		return "", err
	}
	u.MustChangePassword = true
	u.FailedLogins = 0
	u.LockedUntil = nil
	if err := s.users.UpdateUser(ctx, u); err != nil {
		return "", err
	}
	// A reset usually means the account may be compromised: nothing issued before it may keep working.
	if err := s.RevokeUserAccess(ctx, u.ID); err != nil {
		return "", err
	}
	return generated, nil
}

// RevokeUserAccess ends all of the user's sessions and deletes all of their API tokens.
func (s *Service) RevokeUserAccess(ctx context.Context, userID string) error {
	if err := s.sessions.DeleteUserSessions(ctx, userID); err != nil {
		return err
	}
	return s.tokens.DeleteUserTokens(ctx, userID)
}

// passwordOrGenerate validates a given password or makes a new one. generated is the password
// when it was made here and empty otherwise.
func (s *Service) passwordOrGenerate(given string) (password, generated string, err error) {
	if given == "" {
		password, err = generatePassword(20)
		return password, password, err
	}
	if err := ValidatePassword(given); err != nil {
		return "", "", err
	}
	return given, "", nil
}

// BootstrapResult describes what Bootstrap did.
type BootstrapResult struct {
	Created  bool
	Username string
	// Password is set only when it was generated; it is not recoverable afterwards.
	Password string
}

// Bootstrap creates the first administrator account when no user exists yet.
// An empty username means "admin"; an empty password generates a random one.
// The account must change its password on first login.
func (s *Service) Bootstrap(ctx context.Context, username, password string) (BootstrapResult, error) {
	n, err := s.users.CountUsers(ctx)
	if err != nil {
		return BootstrapResult{}, err
	}
	if n > 0 {
		return BootstrapResult{}, nil
	}

	username = domain.NormalizeUsername(username)
	if username == "" {
		username = "admin"
	}
	res := BootstrapResult{Username: username}
	if password == "" {
		if password, err = generatePassword(20); err != nil {
			return BootstrapResult{}, err
		}
		res.Password = password
	} else if err := ValidatePassword(password); err != nil {
		return BootstrapResult{}, fmt.Errorf("bootstrap admin password: %w", err)
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return BootstrapResult{}, err
	}
	u := domain.User{
		ID:                 newID(),
		Username:           username,
		DisplayName:        "Administrator",
		PasswordHash:       hash,
		MustChangePassword: true,
		CreatedAt:          s.now().UTC(),
		CreatedBy:          "bootstrap",
	}
	if err := u.Validate(); err != nil {
		return BootstrapResult{}, fmt.Errorf("bootstrap admin: %w", err)
	}
	if err := s.users.CreateUser(ctx, u); err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			// Another replica won the race; nothing to do.
			return BootstrapResult{}, nil
		}
		return BootstrapResult{}, err
	}
	res.Created = true
	return res, nil
}

// issue opens a session for the user and signs a matching access token. familyID continues the family
// of the session being refreshed; empty starts a new one (a login).
func (s *Service) issue(ctx context.Context, u domain.User, meta SessionMeta, familyID string) (Tokens, error) {
	if familyID == "" {
		familyID = newID()
	}
	now := s.now().UTC()
	accessExp := now.Add(s.accessTTL)
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    issuer,
		Subject:   u.ID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(accessExp),
	}).SignedString(s.secret)
	if err != nil {
		return Tokens{}, err
	}

	refresh, err := randomToken(32)
	if err != nil {
		return Tokens{}, err
	}
	refreshExp := now.Add(s.refreshTTL)
	if err := s.sessions.CreateSession(ctx, domain.Session{
		ID:        newID(),
		UserID:    u.ID,
		TokenHash: hashToken(refresh),
		CreatedAt: now,
		ExpiresAt: refreshExp,
		UserAgent: truncate(meta.UserAgent, 256),
		IP:        meta.IP,
		FamilyID:  familyID,
	}); err != nil {
		return Tokens{}, err
	}
	return Tokens{AccessToken: access, AccessExpiresAt: accessExp, RefreshToken: refresh, RefreshExpiresAt: refreshExp}, nil
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// passwordAlphabet leaves out look-alike characters (0/O, 1/l/I).
const passwordAlphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"

func generatePassword(n int) (string, error) {
	out := make([]byte, n)
	max := big.NewInt(int64(len(passwordAlphabet)))
	for i := range out {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = passwordAlphabet[v.Int64()]
	}
	return string(out), nil
}

// Credential says how a request was authenticated.
type Credential struct {
	// Kind is "session" (an access token from signing in) or "token" (an API token).
	Kind string
	// TokenID is the API token's ID when Kind is "token".
	TokenID string
}

const (
	CredentialSession = "session"
	CredentialToken   = "token"
)

// AuthenticateBearer authenticates what was sent after "Bearer ": an API token if it starts with the
// token prefix, an access token otherwise. The user is loaded on every call, so disabling an account
// takes effect immediately for both.
func (s *Service) AuthenticateBearer(ctx context.Context, bearer string) (domain.User, Credential, error) {
	if strings.HasPrefix(bearer, domain.TokenPrefix) {
		return s.authenticateAPIToken(ctx, bearer)
	}
	u, err := s.Authenticate(ctx, bearer)
	return u, Credential{Kind: CredentialSession}, err
}

func (s *Service) authenticateAPIToken(ctx context.Context, secret string) (domain.User, Credential, error) {
	tok, err := s.tokens.GetTokenByHash(ctx, hashToken(secret))
	if errors.Is(err, store.ErrNotFound) {
		return domain.User{}, Credential{}, ErrInvalidToken
	}
	if err != nil {
		return domain.User{}, Credential{}, err
	}
	now := s.now()
	if !now.Before(tok.ExpiresAt) {
		return domain.User{}, Credential{}, ErrInvalidToken
	}
	u, err := s.users.GetUser(ctx, tok.UserID)
	if errors.Is(err, store.ErrNotFound) {
		return domain.User{}, Credential{}, ErrInvalidToken
	}
	if err != nil {
		return domain.User{}, Credential{}, err
	}
	if u.Disabled {
		return domain.User{}, Credential{}, ErrInvalidToken
	}
	// Last use is informational, so it is written at most once a minute and a failure is not an error.
	if tok.LastUsedAt == nil || now.Sub(*tok.LastUsedAt) >= touchInterval {
		if err := s.tokens.TouchToken(ctx, tok.ID, now.UTC()); err != nil {
			s.log.Error("recording API token use", "token", tok.ID, "err", err)
		}
	}
	return u, Credential{Kind: CredentialToken, TokenID: tok.ID}, nil
}

// CreateToken issues an API token for the user. The secret is returned here and never again.
// Errors: ErrInvalidTokenRequest, ErrTokenLimit.
func (s *Service) CreateToken(ctx context.Context, userID, name string, ttl time.Duration) (string, domain.APIToken, error) {
	name = strings.TrimSpace(name)
	if err := domain.ValidateTokenName(name); err != nil {
		return "", domain.APIToken{}, fmt.Errorf("%w: %v", ErrInvalidTokenRequest, err)
	}
	if ttl < MinTokenTTL || ttl > MaxTokenTTL {
		return "", domain.APIToken{}, fmt.Errorf("%w: the lifetime must be between 1 and 365 days", ErrInvalidTokenRequest)
	}
	n, err := s.tokens.CountTokens(ctx, userID)
	if err != nil {
		return "", domain.APIToken{}, err
	}
	if n >= domain.MaxTokensPerUser {
		return "", domain.APIToken{}, ErrTokenLimit
	}
	random, err := randomToken(32)
	if err != nil {
		return "", domain.APIToken{}, err
	}
	secret := domain.TokenPrefix + random
	now := s.now().UTC()
	tok := domain.APIToken{
		ID:        newID(),
		UserID:    userID,
		Name:      name,
		Prefix:    secret[:tokenPrefixLen],
		TokenHash: hashToken(secret),
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	if err := s.tokens.CreateToken(ctx, tok); err != nil {
		return "", domain.APIToken{}, err
	}
	return secret, tok, nil
}

// ListTokens returns the user's API tokens, newest first. Secrets are not included, they cannot be read back.
func (s *Service) ListTokens(ctx context.Context, userID string) ([]domain.APIToken, error) {
	return s.tokens.ListTokens(ctx, userID)
}

// RevokeToken deletes one of the user's own API tokens; store.ErrNotFound if it is not theirs.
func (s *Service) RevokeToken(ctx context.Context, userID, tokenID string) error {
	return s.tokens.DeleteToken(ctx, userID, tokenID)
}
