package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/magicorntech/Fluxor/internal/authz"
	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

func (s *server) listTeams(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.TeamManage) {
		return
	}
	items, err := s.store.ListTeams(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *server) getTeam(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.TeamManage) {
		return
	}
	t, err := s.store.GetTeam(r.Context(), r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *server) createTeam(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.TeamManage) {
		return
	}
	actor, _ := UserFromContext(r.Context())
	var in struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Members     []string `json:"members"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	t := domain.Team{
		ID: newID(), Name: in.Name, Description: in.Description,
		Members: domain.NormalizeMembers(in.Members), CreatedAt: time.Now().UTC(), CreatedBy: actor.ID,
	}
	if err := t.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
		return
	}
	if !s.membersExist(w, r, t.Members, s.userIDs) {
		return
	}
	if err := s.store.CreateTeam(r.Context(), t); err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, "already_exists", "a team named "+t.Name+" already exists")
			return
		}
		s.internalError(w, err)
		return
	}
	noteSubject(r, "team", t.ID, t.Name)
	noteDetail(r, "members", itoa(len(t.Members)))
	writeJSON(w, http.StatusCreated, t)
}

func (s *server) updateTeam(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.TeamManage) {
		return
	}
	var in struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	t, err := s.store.GetTeam(r.Context(), r.PathValue("id"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	noteSubject(r, "team", t.ID, t.Name)
	if in.Name != nil {
		noteDetail(r, "newName", *in.Name)
		t.Name = *in.Name
	}
	if in.Description != nil {
		t.Description = *in.Description
	}
	if err := t.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
		return
	}
	if err := s.store.UpdateTeam(r.Context(), t); err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, "already_exists", "a team named "+t.Name+" already exists")
			return
		}
		s.storeError(w, err)
		return
	}
	// Re-read so that a membership change made in the meantime is not reported as lost.
	updated, err := s.store.GetTeam(r.Context(), t.ID)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *server) setTeamMembers(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.TeamManage) {
		return
	}
	id := r.PathValue("id")
	members, ok := s.decodeMembers(w, r)
	if !ok {
		return
	}
	team, err := s.store.GetTeam(r.Context(), id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	noteSubject(r, "team", team.ID, team.Name)
	noteDetail(r, "members", itoa(len(members)))
	if !s.membersExist(w, r, members, s.userIDs) {
		return
	}
	if s.refuseLastAdminChange(w, r, authz.AdminChange{TeamMembers: map[string][]string{id: members}},
		"this change would leave no active administrator: the team carries the admin role") {
		return
	}
	if err := s.store.SetTeamMembers(r.Context(), id, members); err != nil {
		s.storeError(w, err)
		return
	}
	t, err := s.store.GetTeam(r.Context(), id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *server) deleteTeam(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.TeamManage) {
		return
	}
	id := r.PathValue("id")
	team, err := s.store.GetTeam(r.Context(), id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	noteSubject(r, "team", team.ID, team.Name)
	// A deleted team looks like a team without members.
	if s.refuseLastAdminChange(w, r, authz.AdminChange{TeamMembers: map[string][]string{id: nil}},
		"this team carries the admin role and its members are the last active administrators") {
		return
	}
	if err := s.store.DeleteTeam(r.Context(), id); err != nil {
		s.storeError(w, err)
		return
	}
	s.dropBindingsOfTeam(r, id)
	w.WriteHeader(http.StatusNoContent)
}

// dropBindingsOfTeam removes the grants of a team that no longer exists. Best effort: they would
// grant nothing anyway, so failures are logged, not reported.
func (s *server) dropBindingsOfTeam(r *http.Request, teamID string) {
	bindings, err := s.store.ListBindings(r.Context(), store.BindingFilter{
		Subjects: []domain.Subject{{Kind: domain.SubjectTeam, ID: teamID}},
	})
	if err != nil {
		s.log.Error("listing bindings of a deleted team", "team", teamID, "err", err)
		return
	}
	for _, b := range bindings {
		if err := s.store.DeleteBinding(r.Context(), b.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			s.log.Error("deleting binding of a deleted team", "binding", b.ID, "err", err)
		}
	}
}

// decodeMembers reads {"members": [...]}, validates and normalizes the list. The field is required so
// that a typo cannot silently empty a team.
func (s *server) decodeMembers(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	var in struct {
		Members *[]string `json:"members"`
	}
	if !decodeJSON(w, r, &in) {
		return nil, false
	}
	if in.Members == nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", "members is required (use [] to remove everyone)")
		return nil, false
	}
	if err := domain.ValidateMembers(*in.Members); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
		return nil, false
	}
	return domain.NormalizeMembers(*in.Members), true
}

// membersExist checks every member ID against the set of existing resources, loaded once. On a
// miss it has written a 400 (or a 500) and returns false.
func (s *server) membersExist(w http.ResponseWriter, r *http.Request, members []string, load func(*http.Request) (map[string]bool, error)) bool {
	if len(members) == 0 {
		return true
	}
	existing, err := load(r)
	if err != nil {
		s.internalError(w, err)
		return false
	}
	for _, m := range members {
		if !existing[m] {
			writeError(w, http.StatusBadRequest, "invalid_argument", "member "+m+" does not exist")
			return false
		}
	}
	return true
}

func (s *server) userIDs(r *http.Request) (map[string]bool, error) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(users))
	for _, u := range users {
		set[u.ID] = true
	}
	return set, nil
}

func (s *server) clusterIDs(r *http.Request) (map[string]bool, error) {
	clusters, err := s.store.ListClusters(r.Context())
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(clusters))
	for _, c := range clusters {
		set[c.ID] = true
	}
	return set, nil
}
