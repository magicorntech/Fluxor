package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/magicorntech/Fluxor/internal/authz"
	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

func (s *server) listRoles(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.RBACManage) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": authz.Roles()})
}

func (s *server) listRoleBindings(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.RBACManage) {
		return
	}
	var filter store.BindingFilter
	kind, id := r.URL.Query().Get("subjectKind"), r.URL.Query().Get("subjectId")
	if (kind == "") != (id == "") {
		writeError(w, http.StatusBadRequest, "invalid_argument", "subjectKind and subjectId must be given together")
		return
	}
	if kind != "" {
		filter.Subjects = []domain.Subject{{Kind: domain.SubjectKind(kind), ID: id}}
	}
	items, err := s.store.ListBindings(r.Context(), filter)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *server) createRoleBinding(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.RBACManage) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	var in struct {
		Subject domain.Subject `json:"subject"`
		RoleID  string         `json:"roleId"`
		Scope   struct {
			Cluster     *domain.Ref `json:"cluster"`
			Application *domain.Ref `json:"application"`
		} `json:"scope"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	// A missing dimension means "all".
	scope := domain.AnyScope()
	if in.Scope.Cluster != nil {
		scope.Cluster = *in.Scope.Cluster
	}
	if in.Scope.Application != nil {
		scope.Application = *in.Scope.Application
	}
	b := domain.Binding{
		ID:        newID(),
		Subject:   in.Subject,
		RoleID:    in.RoleID,
		Scope:     scope,
		CreatedAt: time.Now().UTC(),
		CreatedBy: actor.ID,
	}
	if err := s.authz.ValidateBinding(r.Context(), b); err != nil {
		if errors.Is(err, authz.ErrInvalidBinding) {
			writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
			return
		}
		s.internalError(w, err)
		return
	}
	if err := s.store.CreateBinding(r.Context(), b); err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, "already_exists", "this role is already granted to the subject in this scope")
			return
		}
		s.internalError(w, err)
		return
	}
	noteSubject(r, "role_binding", b.ID, "")
	describeBinding(r, b)
	writeJSON(w, http.StatusCreated, b)
}

// describeBinding records who got which role where, for the audit log.
func describeBinding(r *http.Request, b domain.Binding) {
	noteDetail(r, "role", b.RoleID)
	noteDetail(r, "to", string(b.Subject.Kind)+":"+b.Subject.ID)
	noteDetail(r, "scope", scopeText(b.Scope))
}

func (s *server) deleteRoleBinding(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.RBACManage) {
		return
	}
	id := r.PathValue("id")
	b, err := s.store.GetBinding(r.Context(), id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	describeBinding(r, b)
	if b.RoleID == authz.RoleAdmin && b.Scope.IsAny() {
		if s.refuseLastAdminChange(w, r, authz.AdminChange{ExcludeBinding: id}, "this binding is what makes the last active administrator; grant the role to someone else first") {
			return
		}
	}
	if err := s.store.DeleteBinding(r.Context(), id); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// refuseLastAdminChange answers 409 last_admin and returns true if the change would leave the system
// without an active administrator. On a store failure it answers 500 and also returns true.
func (s *server) refuseLastAdminChange(w http.ResponseWriter, r *http.Request, change authz.AdminChange, message string) bool {
	last, err := s.authz.WouldLeaveNoAdmin(r.Context(), change)
	if err != nil {
		s.internalError(w, err)
		return true
	}
	if last {
		writeError(w, http.StatusConflict, "last_admin", message)
		return true
	}
	return false
}
