package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/magicorntech/Fluxor/internal/authz"
	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

func (s *server) listGroups(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.GroupManage) {
		return
	}
	kind := domain.GroupKind(r.URL.Query().Get("kind"))
	if kind != "" && !kind.Valid() {
		writeError(w, http.StatusBadRequest, "invalid_argument", "kind must be cluster or application")
		return
	}
	items, err := s.store.ListGroups(r.Context(), store.GroupFilter{Kind: kind})
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *server) getGroup(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.GroupManage) {
		return
	}
	g, err := s.store.GetGroup(r.Context(), r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *server) createGroup(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.GroupManage) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	var in struct {
		Kind        domain.GroupKind `json:"kind"`
		Name        string           `json:"name"`
		Description string           `json:"description"`
		Members     []string         `json:"members"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	g := domain.ResourceGroup{
		ID: newID(), Kind: in.Kind, Name: in.Name, Description: in.Description,
		Members: domain.NormalizeMembers(in.Members), CreatedAt: time.Now().UTC(), CreatedBy: actor.ID,
	}
	if err := g.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
		return
	}
	if !s.groupKindAvailable(w, g.Kind) || !s.groupMembersExist(w, r, g.Kind, g.Members) {
		return
	}
	if err := s.store.CreateGroup(r.Context(), g); err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, "already_exists", "a "+string(g.Kind)+" group named "+g.Name+" already exists")
			return
		}
		s.internalError(w, err)
		return
	}
	noteSubject(r, "group", g.ID, g.Name)
	noteDetail(r, "kind", string(g.Kind))
	noteDetail(r, "members", itoa(len(g.Members)))
	writeJSON(w, http.StatusCreated, g)
}

func (s *server) updateGroup(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.GroupManage) {
		return
	}
	var in struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	g, err := s.store.GetGroup(r.Context(), r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	noteSubject(r, "group", g.ID, g.Name)
	if in.Name != nil {
		noteDetail(r, "newName", *in.Name)
		g.Name = *in.Name
	}
	if in.Description != nil {
		g.Description = *in.Description
	}
	if err := g.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
		return
	}
	if err := s.store.UpdateGroup(r.Context(), g); err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, "already_exists", "a "+string(g.Kind)+" group named "+g.Name+" already exists")
			return
		}
		s.storeError(w, err)
		return
	}
	updated, err := s.store.GetGroup(r.Context(), g.ID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *server) setGroupMembers(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.GroupManage) {
		return
	}
	id := r.PathValue("id")
	members, ok := s.decodeMembers(w, r)
	if !ok {
		return
	}
	g, err := s.store.GetGroup(r.Context(), id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	noteSubject(r, "group", g.ID, g.Name)
	noteDetail(r, "members", itoa(len(members)))
	if !s.groupKindAvailable(w, g.Kind) || !s.groupMembersExist(w, r, g.Kind, members) {
		return
	}
	if err := s.store.SetGroupMembers(r.Context(), id, members); err != nil {
		s.storeError(w, err)
		return
	}
	updated, err := s.store.GetGroup(r.Context(), id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.GroupManage) {
		return
	}
	id := r.PathValue("id")
	g, err := s.store.GetGroup(r.Context(), id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	noteSubject(r, "group", g.ID, g.Name)
	if err := s.store.DeleteGroup(r.Context(), id); err != nil {
		s.storeError(w, err)
		return
	}
	s.dropBindingsForGroup(r, id)
	w.WriteHeader(http.StatusNoContent)
}

// dropBindingsForGroup removes grants that point at a group that no longer exists. Best effort:
// such grants match nothing, so failures are logged, not reported.
func (s *server) dropBindingsForGroup(r *http.Request, groupID string) {
	bindings, err := s.store.ListBindings(r.Context(), store.BindingFilter{})
	if err != nil {
		s.log.Error("listing bindings after group delete", "group", groupID, "err", err)
		return
	}
	for _, b := range bindings {
		inCluster := b.Scope.Cluster.Kind == domain.RefGroup && b.Scope.Cluster.ID == groupID
		inApp := b.Scope.Application.Kind == domain.RefGroup && b.Scope.Application.ID == groupID
		if !inCluster && !inApp {
			continue
		}
		if err := s.store.DeleteBinding(r.Context(), b.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			s.log.Error("deleting binding of a deleted group", "binding", b.ID, "err", err)
		}
	}
}

// groupKindAvailable refuses kinds whose resources do not exist yet.
func (s *server) groupKindAvailable(w http.ResponseWriter, kind domain.GroupKind) bool {
	if kind == domain.GroupKindApplication && !authz.ApplicationsAvailable {
		writeError(w, http.StatusBadRequest, "invalid_argument", "application groups are not available yet")
		return false
	}
	return true
}

func (s *server) groupMembersExist(w http.ResponseWriter, r *http.Request, kind domain.GroupKind, members []string) bool {
	if kind == domain.GroupKindCluster {
		return s.membersExist(w, r, members, s.clusterIDs)
	}
	return true
}
