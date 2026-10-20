package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/magicorntech/Fluxor/internal/auth"
	"github.com/magicorntech/Fluxor/internal/authz"
	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

// adminUserResponse is the view of a user that administrators get; it never has the password hash.
type adminUserResponse struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	Email              string     `json:"email,omitempty"`
	DisplayName        string     `json:"displayName,omitempty"`
	Disabled           bool       `json:"disabled"`
	MustChangePassword bool       `json:"mustChangePassword"`
	CreatedAt          time.Time  `json:"createdAt"`
	CreatedBy          string     `json:"createdBy,omitempty"`
	LastLoginAt        *time.Time `json:"lastLoginAt,omitempty"`
}

func toAdminUserResponse(u domain.User) adminUserResponse {
	return adminUserResponse{
		ID: u.ID, Username: u.Username, Email: u.Email, DisplayName: u.DisplayName,
		Disabled: u.Disabled, MustChangePassword: u.MustChangePassword,
		CreatedAt: u.CreatedAt, CreatedBy: u.CreatedBy, LastLoginAt: u.LastLoginAt,
	}
}

func (s *server) listUsers(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.UserManage) {
		return
	}
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	items := make([]adminUserResponse, 0, len(users))
	for _, u := range users {
		items = append(items, toAdminUserResponse(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *server) getUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.UserManage) {
		return
	}
	u, err := s.store.GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toAdminUserResponse(u))
}

// createUserResponse carries the generated temporary password, the only time it can be read.
type createUserResponse struct {
	adminUserResponse
	TemporaryPassword string `json:"temporaryPassword,omitempty"`
}

func (s *server) createUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.UserManage) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	var in struct {
		Username    string `json:"username"`
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Password    string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	u, generated, err := s.auth.CreateUser(r.Context(), auth.NewUser{
		Username: in.Username, Email: in.Email, DisplayName: in.DisplayName,
		Password: in.Password, CreatedBy: actor.ID,
	})
	switch {
	case errors.Is(err, auth.ErrInvalidUser), errors.Is(err, auth.ErrWeakPassword):
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
	case errors.Is(err, store.ErrAlreadyExists):
		writeError(w, http.StatusConflict, "already_exists", "a user with this username or email already exists")
	case err != nil:
		s.internalError(w, err)
	default:
		noteSubject(r, "user", u.ID, u.Username)
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, createUserResponse{adminUserResponse: toAdminUserResponse(u), TemporaryPassword: generated})
	}
}

func (s *server) updateUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.UserManage) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	var in struct {
		DisplayName *string `json:"displayName"`
		Email       *string `json:"email"`
		Disabled    *bool   `json:"disabled"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	u, err := s.store.GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	noteSubject(r, "user", u.ID, u.Username)
	var fields []string
	if in.DisplayName != nil {
		fields = append(fields, "displayName")
	}
	if in.Email != nil {
		fields = append(fields, "email")
	}
	if in.Disabled != nil {
		fields = append(fields, "disabled")
		noteDetail(r, "disabled", strconv.FormatBool(*in.Disabled))
	}
	noteDetail(r, "fields", joinFields(fields))

	if in.DisplayName != nil {
		name := strings.TrimSpace(*in.DisplayName)
		if len(name) > 100 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "display name must be at most 100 characters")
			return
		}
		u.DisplayName = name
	}
	if in.Email != nil {
		u.Email = strings.ToLower(strings.TrimSpace(*in.Email))
		if err := u.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
			return
		}
	}
	disabling := in.Disabled != nil && *in.Disabled && !u.Disabled
	if in.Disabled != nil {
		if disabling && u.ID == actor.ID {
			writeError(w, http.StatusBadRequest, "invalid_argument", "you cannot disable your own account")
			return
		}
		// No "last administrator" check is needed here: only active administrators hold user:manage
		// (directly or through a team), and they cannot disable themselves, so the account being
		// disabled is never the last admin.
		// Add the check (authz.WouldLeaveNoAdmin) as soon as another role can grant user:manage.
		u.Disabled = *in.Disabled
	}

	if err := s.store.UpdateUser(r.Context(), u); err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, "already_exists", "a user with this email already exists")
			return
		}
		s.storeError(w, err)
		return
	}
	if disabling {
		// The user is locked out at once anyway (their tokens stop working); this also removes their
		// sessions and API tokens, so that re-enabling the account does not bring old credentials back.
		if err := s.auth.RevokeUserAccess(r.Context(), u.ID); err != nil {
			s.log.Error("revoking access of a disabled user", "user", u.ID, "err", err)
		}
	}
	writeJSON(w, http.StatusOK, toAdminUserResponse(u))
}

func (s *server) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.UserManage) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	id := r.PathValue("id")
	if id == actor.ID {
		writeError(w, http.StatusBadRequest, "invalid_argument", "use the change-password endpoint for your own account")
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	target, err := s.store.GetUser(r.Context(), id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	noteSubject(r, "user", target.ID, target.Username)
	generated, err := s.auth.ResetPassword(r.Context(), id, in.Password)
	switch {
	case errors.Is(err, auth.ErrWeakPassword):
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
	case err != nil:
		s.storeError(w, err)
	default:
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]any{"temporaryPassword": generated})
	}
}
