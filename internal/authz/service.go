package authz

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

// ApplicationsAvailable turns on application ids and application groups, as scope references and as
// groups that can be created. It stays off until applications exist, so nothing can be stored that
// could not be honoured.
const ApplicationsAvailable = false

// ErrInvalidBinding wraps the reason a role binding was rejected.
var ErrInvalidBinding = errors.New("invalid role binding")

// BootstrapBindingID is the fixed ID of the binding that makes the first user an administrator,
// so that creating it twice (several replicas starting at once) is harmless.
const BootstrapBindingID = "bootstrap-admin"

// BootstrapCreator marks users and bindings created by the automatic first-run setup.
const BootstrapCreator = "bootstrap"

// Store is what the authorizer needs from persistence.
type Store interface {
	store.BindingStore
	store.UserStore
	store.ClusterStore
	store.TeamStore
	store.GroupStore
}

type Authorizer struct {
	store Store
	now   func() time.Time
}

func New(st Store) *Authorizer { return &Authorizer{store: st, now: time.Now} }

// For loads what applies to the user: the bindings granted to them and to every team they belong
// to, plus the groups those bindings refer to. One call serves any number of checks.
func (a *Authorizer) For(ctx context.Context, userID string) (*Access, error) {
	subjects := []domain.Subject{{Kind: domain.SubjectUser, ID: userID}}
	teams, err := a.store.ListTeamsOfUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, t := range teams {
		subjects = append(subjects, domain.Subject{Kind: domain.SubjectTeam, ID: t.ID})
	}
	bindings, err := a.store.ListBindings(ctx, store.BindingFilter{Subjects: subjects})
	if err != nil {
		return nil, err
	}
	groups, err := a.loadGroups(ctx, bindings)
	if err != nil {
		return nil, err
	}
	return NewAccess(bindings, groups), nil
}

// loadGroups fetches the groups the bindings point at. It returns nil when there are none.
func (a *Authorizer) loadGroups(ctx context.Context, bindings []domain.Binding) (Groups, error) {
	wanted := map[string]bool{}
	for _, b := range bindings {
		for _, ref := range []domain.Ref{b.Scope.Cluster, b.Scope.Application} {
			if ref.Kind == domain.RefGroup {
				wanted[ref.ID] = true
			}
		}
	}
	if len(wanted) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(wanted))
	for id := range wanted {
		ids = append(ids, id)
	}
	list, err := a.store.ListGroups(ctx, store.GroupFilter{IDs: ids})
	if err != nil {
		return nil, err
	}
	idx := groupIndex{}
	for _, g := range list {
		members := make(map[string]bool, len(g.Members))
		for _, m := range g.Members {
			members[m] = true
		}
		idx[g.ID] = indexedGroup{kind: g.Kind, members: members}
	}
	return idx, nil
}

type indexedGroup struct {
	kind    domain.GroupKind
	members map[string]bool
}

// groupIndex is a Groups backed by the loaded groups. Groups that no longer exist are absent and
// therefore match nothing.
type groupIndex map[string]indexedGroup

func (g groupIndex) Contains(kind domain.GroupKind, groupID, resourceID string) bool {
	grp, ok := g[groupID]
	return ok && grp.kind == kind && grp.members[resourceID]
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidBinding, fmt.Sprintf(format, args...))
}

// ValidateBinding checks that a binding makes sense before it is stored: its shape, the role,
// the subject and the referenced resources must exist, and roles with system permissions must
// not be narrowed. The error wraps ErrInvalidBinding unless the store itself failed.
func (a *Authorizer) ValidateBinding(ctx context.Context, b domain.Binding) error {
	if err := b.Validate(); err != nil {
		return invalid("%v", err)
	}
	role, ok := LookupRole(b.RoleID)
	if !ok {
		return invalid("unknown role %q", b.RoleID)
	}
	if role.IsSystem() && !b.Scope.IsAny() {
		return invalid("role %q grants system permissions and cannot be limited to a scope", role.ID)
	}
	if !role.HasClass(ClassApp) && b.Scope.Application.Kind != domain.RefAny {
		return invalid("role %q has no application permissions, so the application scope must be %q", role.ID, domain.RefAny)
	}

	switch b.Subject.Kind {
	case domain.SubjectUser:
		if _, err := a.store.GetUser(ctx, b.Subject.ID); errors.Is(err, store.ErrNotFound) {
			return invalid("user %q does not exist", b.Subject.ID)
		} else if err != nil {
			return err
		}
	case domain.SubjectTeam:
		if _, err := a.store.GetTeam(ctx, b.Subject.ID); errors.Is(err, store.ErrNotFound) {
			return invalid("team %q does not exist", b.Subject.ID)
		} else if err != nil {
			return err
		}
	}

	switch b.Scope.Cluster.Kind {
	case domain.RefID:
		if _, err := a.store.GetCluster(ctx, b.Scope.Cluster.ID); errors.Is(err, store.ErrNotFound) {
			return invalid("cluster %q does not exist", b.Scope.Cluster.ID)
		} else if err != nil {
			return err
		}
	case domain.RefGroup:
		g, err := a.store.GetGroup(ctx, b.Scope.Cluster.ID)
		if errors.Is(err, store.ErrNotFound) {
			return invalid("group %q does not exist", b.Scope.Cluster.ID)
		}
		if err != nil {
			return err
		}
		if g.Kind != domain.GroupKindCluster {
			return invalid("group %q is a %s group, but the cluster scope needs a cluster group", g.ID, g.Kind)
		}
	}
	switch b.Scope.Application.Kind {
	case domain.RefID, domain.RefGroup:
		if !ApplicationsAvailable {
			return invalid("application scopes are not available yet")
		}
	}
	return nil
}

// AdminChange describes a change that is being considered, so that its effect on the set of
// administrators can be computed before it is made.
type AdminChange struct {
	// ExcludeBinding ignores this binding, as if it were deleted.
	ExcludeBinding string
	// TeamMembers pretends that these teams have these members. A nil or empty list means the team
	// has no members, which is also how a deleted team looks.
	TeamMembers map[string][]string
}

// AdminUserIDs returns the IDs of the active (not disabled) users who hold the admin role without
// any scope limit, directly or through a team, sorted. change shows what the answer would be after
// a pending change; use the zero value for the current state.
func (a *Authorizer) AdminUserIDs(ctx context.Context, change AdminChange) ([]string, error) {
	bindings, err := a.store.ListBindings(ctx, store.BindingFilter{RoleID: RoleAdmin})
	if err != nil {
		return nil, err
	}
	candidates := map[string]bool{}
	for _, b := range bindings {
		if b.ID == change.ExcludeBinding || !b.Scope.IsAny() {
			continue
		}
		switch b.Subject.Kind {
		case domain.SubjectUser:
			candidates[b.Subject.ID] = true
		case domain.SubjectTeam:
			members, overridden := change.TeamMembers[b.Subject.ID]
			if !overridden {
				t, err := a.store.GetTeam(ctx, b.Subject.ID)
				if errors.Is(err, store.ErrNotFound) {
					continue
				}
				if err != nil {
					return nil, err
				}
				members = t.Members
			}
			for _, m := range members {
				candidates[m] = true
			}
		}
	}
	ids := make([]string, 0, len(candidates))
	for id := range candidates {
		u, err := a.store.GetUser(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !u.Disabled {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// WouldLeaveNoAdmin reports whether the change would remove the last active administrator. It is
// false when there is no administrator to begin with, so a broken state can still be repaired.
func (a *Authorizer) WouldLeaveNoAdmin(ctx context.Context, change AdminChange) (bool, error) {
	before, err := a.AdminUserIDs(ctx, AdminChange{})
	if err != nil || len(before) == 0 {
		return false, err
	}
	after, err := a.AdminUserIDs(ctx, change)
	if err != nil {
		return false, err
	}
	return len(after) == 0, nil
}

// BootstrapGrant describes what EnsureBootstrapAdmin did.
type BootstrapGrant struct {
	Created  bool
	UserID   string
	Username string
}

// EnsureBootstrapAdmin makes the user created by the first-run setup an administrator, but only
// while no admin binding exists at all. That covers a fresh install as well as a database from
// before roles existed. It never grants anything once any administrator has been set up.
func (a *Authorizer) EnsureBootstrapAdmin(ctx context.Context) (BootstrapGrant, error) {
	existing, err := a.store.ListBindings(ctx, store.BindingFilter{RoleID: RoleAdmin})
	if err != nil || len(existing) > 0 {
		return BootstrapGrant{}, err
	}
	users, err := a.store.ListUsers(ctx)
	if err != nil {
		return BootstrapGrant{}, err
	}
	var first *domain.User
	for i := range users {
		u := users[i]
		if u.CreatedBy == BootstrapCreator && (first == nil || u.CreatedAt.Before(first.CreatedAt)) {
			first = &u
		}
	}
	if first == nil {
		return BootstrapGrant{}, nil
	}
	err = a.store.CreateBinding(ctx, domain.Binding{
		ID:        BootstrapBindingID,
		Subject:   domain.Subject{Kind: domain.SubjectUser, ID: first.ID},
		RoleID:    RoleAdmin,
		Scope:     domain.AnyScope(),
		CreatedAt: a.now().UTC(),
		CreatedBy: BootstrapCreator,
	})
	if errors.Is(err, store.ErrAlreadyExists) {
		return BootstrapGrant{}, nil // another replica did it first
	}
	if err != nil {
		return BootstrapGrant{}, err
	}
	return BootstrapGrant{Created: true, UserID: first.ID, Username: first.Username}, nil
}
