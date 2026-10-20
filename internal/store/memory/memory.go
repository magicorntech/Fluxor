// Package memory is an in-memory store, used for tests and local development.
package memory

import (
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

type Store struct {
	mu       sync.RWMutex
	clusters map[string]domain.Cluster
	users    map[string]domain.User
	sessions map[string]domain.Session // keyed by token hash
	bindings map[string]domain.Binding
	teams    map[string]domain.Team
	groups   map[string]domain.ResourceGroup
	tokens   map[string]domain.APIToken // keyed by ID
	audit    []domain.AuditEvent
}

var _ store.Store = (*Store)(nil)

func New() *Store {
	return &Store{
		clusters: map[string]domain.Cluster{},
		users:    map[string]domain.User{},
		sessions: map[string]domain.Session{},
		bindings: map[string]domain.Binding{},
		teams:    map[string]domain.Team{},
		groups:   map[string]domain.ResourceGroup{},
		tokens:   map[string]domain.APIToken{},
	}
}

// --- clusters ---

// clone returns a copy that shares no map with the original and never has nil labels.
func clone(c domain.Cluster) domain.Cluster {
	labels := make(map[string]string, len(c.Labels))
	for k, v := range c.Labels {
		labels[k] = v
	}
	c.Labels = labels
	return c
}

func (s *Store) ListClusters(_ context.Context) ([]domain.Cluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Cluster, 0, len(s.clusters))
	for _, c := range s.clusters {
		out = append(out, clone(c))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Store) GetCluster(_ context.Context, id string) (domain.Cluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.clusters[id]
	if !ok {
		return domain.Cluster{}, store.ErrNotFound
	}
	return clone(c), nil
}

func (s *Store) CreateCluster(_ context.Context, c domain.Cluster) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.clusters {
		if existing.Name == c.Name {
			return store.ErrAlreadyExists
		}
	}
	s.clusters[c.ID] = clone(c)
	return nil
}

func (s *Store) DeleteCluster(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.clusters[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.clusters, id)
	return nil
}

// --- users ---

func (s *Store) CountUsers(_ context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int64(len(s.users)), nil
}

// conflict reports whether u collides with another user (a different ID).
func (s *Store) conflict(u domain.User) bool {
	for id, other := range s.users {
		if id == u.ID {
			continue
		}
		if other.Username == u.Username || (u.Email != "" && other.Email == u.Email) {
			return true
		}
	}
	return false
}

func (s *Store) CreateUser(_ context.Context, u domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[u.ID]; ok || s.conflict(u) {
		return store.ErrAlreadyExists
	}
	s.users[u.ID] = u
	return nil
}

func (s *Store) GetUser(_ context.Context, id string) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	if !ok {
		return domain.User{}, store.ErrNotFound
	}
	return u, nil
}

func (s *Store) GetUserByUsername(_ context.Context, username string) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.users {
		if u.Username == username {
			return u, nil
		}
	}
	return domain.User{}, store.ErrNotFound
}

func (s *Store) UpdateUser(_ context.Context, u domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[u.ID]; !ok {
		return store.ErrNotFound
	}
	if s.conflict(u) {
		return store.ErrAlreadyExists
	}
	s.users[u.ID] = u
	return nil
}

func (s *Store) ListUsers(_ context.Context) ([]domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out, nil
}

// --- sessions ---

func (s *Store) CreateSession(_ context.Context, sess domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[sess.TokenHash]; ok {
		return store.ErrAlreadyExists
	}
	s.sessions[sess.TokenHash] = sess
	return nil
}

func (s *Store) ConsumeSession(_ context.Context, tokenHash string, now time.Time) (domain.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[tokenHash]
	if !ok {
		return domain.Session{}, store.ErrNotFound
	}
	if sess.UsedAt != nil {
		return sess, store.ErrSessionUsed
	}
	used := sess
	used.UsedAt = &now
	s.sessions[tokenHash] = used
	return sess, nil
}

func (s *Store) DeleteSession(_ context.Context, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, tokenHash)
	return nil
}

func (s *Store) DeleteFamily(_ context.Context, familyID string) error {
	if familyID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for h, sess := range s.sessions {
		if sess.FamilyID == familyID {
			delete(s.sessions, h)
		}
	}
	return nil
}

func (s *Store) DeleteUserSessions(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for h, sess := range s.sessions {
		if sess.UserID == userID {
			delete(s.sessions, h)
		}
	}
	return nil
}

// --- role bindings ---

func (s *Store) CreateBinding(_ context.Context, b domain.Binding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.bindings[b.ID]; ok {
		return store.ErrAlreadyExists
	}
	for _, other := range s.bindings {
		if other.Key() == b.Key() {
			return store.ErrAlreadyExists
		}
	}
	s.bindings[b.ID] = b
	return nil
}

func (s *Store) GetBinding(_ context.Context, id string) (domain.Binding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.bindings[id]
	if !ok {
		return domain.Binding{}, store.ErrNotFound
	}
	return b, nil
}

func (s *Store) ListBindings(_ context.Context, f store.BindingFilter) ([]domain.Binding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.Binding{}
	for _, b := range s.bindings {
		if f.RoleID != "" && b.RoleID != f.RoleID {
			continue
		}
		if len(f.Subjects) > 0 && !slices.Contains(f.Subjects, b.Subject) {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) DeleteBinding(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.bindings[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.bindings, id)
	return nil
}

// --- teams ---

func cloneTeam(t domain.Team) domain.Team {
	t.Members = append([]string{}, t.Members...)
	return t
}

func (s *Store) teamNameTaken(name, exceptID string) bool {
	for id, other := range s.teams {
		if id != exceptID && domain.NameKey(other.Name) == domain.NameKey(name) {
			return true
		}
	}
	return false
}

func (s *Store) CreateTeam(_ context.Context, t domain.Team) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.teams[t.ID]; ok || s.teamNameTaken(t.Name, "") {
		return store.ErrAlreadyExists
	}
	s.teams[t.ID] = cloneTeam(t)
	return nil
}

func (s *Store) GetTeam(_ context.Context, id string) (domain.Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.teams[id]
	if !ok {
		return domain.Team{}, store.ErrNotFound
	}
	return cloneTeam(t), nil
}

func (s *Store) ListTeams(_ context.Context) ([]domain.Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Team, 0, len(s.teams))
	for _, t := range s.teams {
		out = append(out, cloneTeam(t))
	}
	sortTeams(out)
	return out, nil
}

func sortTeams(ts []domain.Team) {
	sort.Slice(ts, func(i, j int) bool {
		a, b := domain.NameKey(ts[i].Name), domain.NameKey(ts[j].Name)
		if a != b {
			return a < b
		}
		return ts[i].ID < ts[j].ID
	})
}

func (s *Store) ListTeamsOfUser(_ context.Context, userID string) ([]domain.Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.Team{}
	for _, t := range s.teams {
		if slices.Contains(t.Members, userID) {
			out = append(out, cloneTeam(t))
		}
	}
	sortTeams(out)
	return out, nil
}

func (s *Store) UpdateTeam(_ context.Context, t domain.Team) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.teams[t.ID]
	if !ok {
		return store.ErrNotFound
	}
	if s.teamNameTaken(t.Name, t.ID) {
		return store.ErrAlreadyExists
	}
	cur.Name, cur.Description = t.Name, t.Description
	s.teams[t.ID] = cur
	return nil
}

func (s *Store) SetTeamMembers(_ context.Context, id string, members []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.teams[id]
	if !ok {
		return store.ErrNotFound
	}
	t.Members = append([]string{}, members...)
	s.teams[id] = t
	return nil
}

func (s *Store) DeleteTeam(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.teams[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.teams, id)
	return nil
}

// --- resource groups ---

func cloneGroup(g domain.ResourceGroup) domain.ResourceGroup {
	g.Members = append([]string{}, g.Members...)
	return g
}

func (s *Store) groupNameTaken(kind domain.GroupKind, name, exceptID string) bool {
	for id, other := range s.groups {
		if id != exceptID && other.Kind == kind && domain.NameKey(other.Name) == domain.NameKey(name) {
			return true
		}
	}
	return false
}

func (s *Store) CreateGroup(_ context.Context, g domain.ResourceGroup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.groups[g.ID]; ok || s.groupNameTaken(g.Kind, g.Name, "") {
		return store.ErrAlreadyExists
	}
	s.groups[g.ID] = cloneGroup(g)
	return nil
}

func (s *Store) GetGroup(_ context.Context, id string) (domain.ResourceGroup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.groups[id]
	if !ok {
		return domain.ResourceGroup{}, store.ErrNotFound
	}
	return cloneGroup(g), nil
}

func (s *Store) ListGroups(_ context.Context, f store.GroupFilter) ([]domain.ResourceGroup, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.ResourceGroup{}
	for id, g := range s.groups {
		if f.Kind != "" && g.Kind != f.Kind {
			continue
		}
		if len(f.IDs) > 0 && !slices.Contains(f.IDs, id) {
			continue
		}
		out = append(out, cloneGroup(g))
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := domain.NameKey(out[i].Name), domain.NameKey(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) UpdateGroup(_ context.Context, g domain.ResourceGroup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.groups[g.ID]
	if !ok {
		return store.ErrNotFound
	}
	if s.groupNameTaken(cur.Kind, g.Name, g.ID) {
		return store.ErrAlreadyExists
	}
	cur.Name, cur.Description = g.Name, g.Description
	s.groups[g.ID] = cur
	return nil
}

func (s *Store) SetGroupMembers(_ context.Context, id string, members []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.groups[id]
	if !ok {
		return store.ErrNotFound
	}
	g.Members = append([]string{}, members...)
	s.groups[id] = g
	return nil
}

func (s *Store) DeleteGroup(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.groups[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.groups, id)
	return nil
}

func (s *Store) RemoveFromGroups(_ context.Context, kind domain.GroupKind, resourceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, g := range s.groups {
		if g.Kind != kind {
			continue
		}
		g.Members = slices.DeleteFunc(append([]string{}, g.Members...), func(m string) bool { return m == resourceID })
		s.groups[id] = g
	}
	return nil
}

// --- API tokens ---

func (s *Store) CreateToken(_ context.Context, t domain.APIToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tokens[t.ID]; ok {
		return store.ErrAlreadyExists
	}
	for _, other := range s.tokens {
		if other.TokenHash == t.TokenHash {
			return store.ErrAlreadyExists
		}
	}
	s.tokens[t.ID] = t
	return nil
}

func (s *Store) GetTokenByHash(_ context.Context, hash string) (domain.APIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.tokens {
		if t.TokenHash == hash {
			return t, nil
		}
	}
	return domain.APIToken{}, store.ErrNotFound
}

func (s *Store) ListTokens(_ context.Context, userID string) ([]domain.APIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.APIToken{}
	for _, t := range s.tokens {
		if t.UserID == userID {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func (s *Store) CountTokens(_ context.Context, userID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var n int64
	for _, t := range s.tokens {
		if t.UserID == userID {
			n++
		}
	}
	return n, nil
}

func (s *Store) DeleteToken(_ context.Context, userID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[id]
	if !ok || t.UserID != userID {
		return store.ErrNotFound
	}
	delete(s.tokens, id)
	return nil
}

func (s *Store) DeleteUserTokens(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, t := range s.tokens {
		if t.UserID == userID {
			delete(s.tokens, id)
		}
	}
	return nil
}

func (s *Store) TouchToken(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[id]
	if !ok {
		return store.ErrNotFound
	}
	t.LastUsedAt = &at
	s.tokens[id] = t
	return nil
}

// --- audit ---

func (s *Store) AddAudit(_ context.Context, e domain.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	details := make(map[string]string, len(e.Details))
	for k, v := range e.Details {
		details[k] = v
	}
	e.Details = details
	s.audit = append(s.audit, e)
	return nil
}

func (s *Store) ListAudit(_ context.Context, f store.AuditFilter) ([]domain.AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	out := []domain.AuditEvent{}
	for _, e := range s.audit {
		if f.ActorID != "" && e.ActorID != f.ActorID {
			continue
		}
		if f.Action != "" && e.Action != f.Action {
			continue
		}
		if f.Result != "" && e.Result != f.Result {
			continue
		}
		if !f.From.IsZero() && e.Time.Before(f.From) {
			continue
		}
		if !f.To.IsZero() && !e.Time.Before(f.To) {
			continue
		}
		if c := f.Before; c != nil && !(e.Time.Before(c.Time) || (e.Time.Equal(c.Time) && e.ID < c.ID)) {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Time.Equal(out[j].Time) {
			return out[i].Time.After(out[j].Time)
		}
		return out[i].ID > out[j].ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
