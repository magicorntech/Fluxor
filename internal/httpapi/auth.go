package httpapi

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/magicorntech/Fluxor/internal/auth"
	"github.com/magicorntech/Fluxor/internal/authz"
	"github.com/magicorntech/Fluxor/internal/domain"
)

const (
	refreshCookie = "fluxor_refresh"
	refreshPath   = "/api/v1/auth"
	// csrfHeader must be present on cookie-authenticated requests. A cross-site form or
	// image request cannot set a custom header, which complements the SameSite=Strict cookie.
	csrfHeader = "X-Fluxor-Request"
)

// Routes that need no access token. Everything else is denied by default.
var publicRoutes = map[string]bool{
	"GET /healthz":              true,
	"POST /api/v1/auth/login":   true,
	"POST /api/v1/auth/refresh": true,
	"POST /api/v1/auth/logout":  true,
}

// Routes a user may call while still forced to change the initial password.
var allowedWhileMustChange = map[string]bool{
	"GET /api/v1/auth/me":       true,
	"PUT /api/v1/auth/password": true,
}

type ctxKey struct{}
type credKey struct{}

// UserFromContext returns the authenticated user of the request.
func UserFromContext(ctx context.Context) (domain.User, bool) {
	u, ok := ctx.Value(ctxKey{}).(domain.User)
	return u, ok
}

// credentialFrom says how the request was authenticated: with a session or an API token.
func credentialFrom(ctx context.Context) (auth.Credential, bool) {
	c, ok := ctx.Value(credKey{}).(auth.Credential)
	return c, ok
}

func itoa(n int) string { return strconv.Itoa(n) }

func (s *server) clientIP(r *http.Request) string { return clientIP(r, s.trustedProxies) }

// authn authenticates every request that is not public and puts the user into the context.
func (s *server) authn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := r.Method + " " + r.URL.Path
		if publicRoutes[route] {
			next.ServeHTTP(w, r)
			return
		}
		token, ok := bearerToken(r)
		if !ok {
			unauthenticated(w)
			return
		}
		u, cred, err := s.auth.AuthenticateBearer(r.Context(), token)
		if errors.Is(err, auth.ErrInvalidToken) {
			unauthenticated(w)
			return
		}
		if err != nil {
			s.internalError(w, err)
			return
		}
		if u.MustChangePassword && !allowedWhileMustChange[route] {
			writeError(w, http.StatusForbidden, "password_change_required", "the password must be changed before using the API")
			return
		}
		if rec := recFrom(r.Context()); rec != nil {
			rec.actorID, rec.actorName = u.ID, u.Username
			if cred.Kind == auth.CredentialToken {
				rec.details["via"] = "api_token"
			}
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, u)
		ctx = context.WithValue(ctx, credKey{}, cred)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(h[len(prefix):]), true
}

func unauthenticated(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeError(w, http.StatusUnauthorized, "unauthenticated", "authentication required")
}

// teamRef names a team the user belongs to.
type teamRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// userResponse is the signed-in user's own view: who they are and what the dashboard may offer them.
// It never contains the password hash.
type userResponse struct {
	ID                 string `json:"id"`
	Username           string `json:"username"`
	Email              string `json:"email,omitempty"`
	DisplayName        string `json:"displayName,omitempty"`
	MustChangePassword bool   `json:"mustChangePassword"`
	// Teams the user belongs to.
	Teams []teamRef `json:"teams"`
	// Permissions lists the system permissions (user and access management) the user holds. Which
	// clusters they may see or change is told per cluster in the cluster responses.
	Permissions []authz.Permission `json:"permissions"`
	// CanCreateClusters says whether the user may register new clusters.
	CanCreateClusters bool `json:"canCreateClusters"`
}

// userView builds the response for a user. The dashboard uses it only to show or hide things;
// every request is checked again by the server.
func (s *server) userView(ctx context.Context, u domain.User) (userResponse, error) {
	access, err := s.authz.For(ctx, u.ID)
	if err != nil {
		return userResponse{}, err
	}
	teams, err := s.store.ListTeamsOfUser(ctx, u.ID)
	if err != nil {
		return userResponse{}, err
	}
	resp := userResponse{
		ID: u.ID, Username: u.Username, Email: u.Email,
		DisplayName: u.DisplayName, MustChangePassword: u.MustChangePassword,
		Teams:             make([]teamRef, 0, len(teams)),
		Permissions:       []authz.Permission{},
		CanCreateClusters: access.Can(authz.ClusterManage, authz.Target{}),
	}
	for _, t := range teams {
		resp.Teams = append(resp.Teams, teamRef{ID: t.ID, Name: t.Name})
	}
	for _, p := range authz.SystemPermissions() {
		if access.Can(p, authz.Target{}) {
			resp.Permissions = append(resp.Permissions, p)
		}
	}
	return resp, nil
}

type sessionResponse struct {
	AccessToken string       `json:"accessToken"`
	ExpiresAt   time.Time    `json:"expiresAt"`
	User        userResponse `json:"user"`
}

// writeSession answers with the access token and sets the rotated refresh cookie.
func (s *server) writeSession(w http.ResponseWriter, r *http.Request, t auth.Tokens, u domain.User) {
	view, err := s.userView(r.Context(), u)
	if err != nil {
		s.internalError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookie,
		Value:    t.RefreshToken,
		Path:     refreshPath,
		Expires:  t.RefreshExpiresAt,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, sessionResponse{AccessToken: t.AccessToken, ExpiresAt: t.AccessExpiresAt, User: view})
}

func (s *server) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookie,
		Value:    "",
		Path:     refreshPath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *server) sessionMeta(r *http.Request) auth.SessionMeta {
	return auth.SessionMeta{UserAgent: r.UserAgent(), IP: s.clientIP(r)}
}

// allowAttempt applies a rate limit per client address. When it refuses it has answered 429 already.
func (s *server) allowAttempt(w http.ResponseWriter, r *http.Request, l *rateLimiter) bool {
	ok, retry := l.allow(s.clientIP(r))
	if ok {
		return true
	}
	secs := int(math.Ceil(retry.Seconds()))
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	s.log.Warn("rate limit hit", "path", r.URL.Path, "ip", s.clientIP(r), "retryAfter", retry.Round(time.Second))
	writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again in "+strconv.Itoa(secs)+" seconds")
	return false
}

func requireCSRFHeader(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get(csrfHeader) == "" {
		writeError(w, http.StatusForbidden, "forbidden", "missing "+csrfHeader+" header")
		return false
	}
	return true
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	if !s.allowAttempt(w, r, s.loginLimit) {
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Username == "" || in.Password == "" {
		writeError(w, http.StatusBadRequest, "invalid_argument", "username and password are required")
		return
	}
	tokens, u, err := s.auth.Login(r.Context(), in.Username, in.Password, s.sessionMeta(r))
	if errors.Is(err, auth.ErrInvalidCredentials) {
		// The client learns nothing but "invalid"; the audit log learns why.
		noteActor(r, "", domain.NormalizeUsername(in.Username))
		noteDetail(r, "reason", auth.FailureReason(err))
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	noteActor(r, u.ID, u.Username)
	s.writeSession(w, r, tokens, u)
}

func (s *server) refresh(w http.ResponseWriter, r *http.Request) {
	if !requireCSRFHeader(w, r) || !s.allowAttempt(w, r, s.refreshLimit) {
		return
	}
	var token string
	if c, err := r.Cookie(refreshCookie); err == nil {
		token = c.Value
	}
	tokens, u, err := s.auth.Refresh(r.Context(), token, s.sessionMeta(r))
	if errors.Is(err, auth.ErrInvalidToken) {
		if errors.Is(err, auth.ErrTokenReuse) {
			// A security event, not an ordinary failure: a used refresh token came back.
			owner := auth.ReusedBy(err)
			name := ""
			if u, gerr := s.store.GetUser(r.Context(), owner); gerr == nil {
				name = u.Username
			}
			s.recordNow(r, "auth.refresh_reuse", http.StatusUnauthorized, owner, name, map[string]string{"effect": "session family revoked"})
		}
		s.clearRefreshCookie(w)
		unauthenticated(w)
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.writeSession(w, r, tokens, u)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if !requireCSRFHeader(w, r) {
		return
	}
	if c, err := r.Cookie(refreshCookie); err == nil {
		if err := s.auth.Logout(r.Context(), c.Value); err != nil {
			s.internalError(w, err)
			return
		}
	}
	s.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	u, _ := UserFromContext(r.Context())
	view, err := s.userView(r.Context(), u)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *server) changePassword(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	tokens, updated, err := s.auth.ChangePassword(r.Context(), u.ID, in.CurrentPassword, in.NewPassword, s.sessionMeta(r))
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		// Not a 401: the session is fine, only the supplied current password is wrong.
		writeError(w, http.StatusBadRequest, "invalid_current_password", "the current password is incorrect")
	case errors.Is(err, auth.ErrWeakPassword):
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
	case err != nil:
		s.internalError(w, err)
	default:
		s.writeSession(w, r, tokens, updated)
	}
}
