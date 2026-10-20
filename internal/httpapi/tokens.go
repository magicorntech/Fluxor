package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/magicorntech/Fluxor/internal/auth"
	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

const defaultTokenDays = 30

// requireSession lets only a signed-in session through, not an API token. A stolen token must not be
// able to create more tokens (a way to stay in after the first one is revoked) or change the password.
func (s *server) requireSession(w http.ResponseWriter, r *http.Request) (domain.User, bool) {
	u, ok := UserFromContext(r.Context())
	if !ok {
		unauthenticated(w)
		return domain.User{}, false
	}
	if cred, _ := credentialFrom(r.Context()); cred.Kind == auth.CredentialToken {
		writeError(w, http.StatusForbidden, "forbidden", "this action needs a signed-in session; an API token cannot do it")
		return domain.User{}, false
	}
	return u, true
}

func (s *server) listTokens(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	items, err := s.auth.ListTokens(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type createTokenResponse struct {
	domain.APIToken
	// Token is the secret. It is shown only now and cannot be read back.
	Token string `json:"token"`
}

func (s *server) createToken(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	var in struct {
		Name          string `json:"name"`
		ExpiresInDays int    `json:"expiresInDays"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	days := in.ExpiresInDays
	if days == 0 {
		days = defaultTokenDays
	}
	secret, tok, err := s.auth.CreateToken(r.Context(), u.ID, in.Name, time.Duration(days)*24*time.Hour)
	switch {
	case errors.Is(err, auth.ErrInvalidTokenRequest):
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
	case errors.Is(err, auth.ErrTokenLimit):
		writeError(w, http.StatusConflict, "limit_reached", "you already have the maximum number of API tokens; delete one first")
	case err != nil:
		s.internalError(w, err)
	default:
		noteSubject(r, "token", tok.ID, tok.Name)
		noteDetail(r, "expiresInDays", itoa(days))
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, createTokenResponse{APIToken: tok, Token: secret})
	}
}

func (s *server) deleteToken(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	// Record the token's name too: once it is deleted it cannot be looked up. Only the user's own tokens
	// are searched, so a foreign id leaves the name empty (and is refused below).
	if own, err := s.auth.ListTokens(r.Context(), u.ID); err == nil {
		for _, t := range own {
			if t.ID == id {
				noteSubject(r, "token", t.ID, t.Name)
			}
		}
	}
	// Someone else's token looks exactly like one that does not exist.
	if err := s.auth.RevokeToken(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "resource not found")
			return
		}
		s.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
