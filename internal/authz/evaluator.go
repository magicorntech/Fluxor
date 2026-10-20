package authz

import "github.com/magicorntech/Fluxor/internal/domain"

// Target is the resource a permission is checked against.
type Target struct {
	// ClusterID is the cluster concerned. Empty means "not a specific existing cluster", for
	// example when registering a new one: only a scope covering all clusters grants that.
	ClusterID string
	// AppID is the application concerned (empty for cluster and system permissions).
	AppID string
}

// Groups answers whether a resource belongs to a resource group of the given kind. A group of
// another kind than asked for never matches, so an application group cannot grant access to clusters.
type Groups interface {
	Contains(kind domain.GroupKind, groupID, resourceID string) bool
}

// Access is the set of bindings that apply to one user (their own and those of their teams),
// loaded once so that many checks, such as filtering a list, cost a single query.
type Access struct {
	bindings []domain.Binding
	groups   Groups
	roles    map[string]Role
}

// NewAccess builds an Access from the effective bindings. groups may be nil, in which case
// group references never match.
func NewAccess(bindings []domain.Binding, groups Groups) *Access {
	roles := make(map[string]Role, len(builtinRoles))
	for _, r := range Roles() {
		roles[r.ID] = r
	}
	return &Access{bindings: bindings, groups: groups, roles: roles}
}

// Can reports whether any binding grants permission p on the target. Grants only add up;
// there is no way to deny.
//
//   - cluster permissions: the binding's cluster scope must cover the target cluster;
//   - application permissions: both the application scope and the cluster scope must cover
//     the target (an application is always checked per cluster it is deployed to);
//   - system permissions: the binding must not be narrowed in any dimension.
//
// Bindings of unknown roles grant nothing.
func (a *Access) Can(p Permission, t Target) bool {
	class := p.Class()
	if class == 0 {
		return false
	}
	for _, b := range a.bindings {
		role, ok := a.roles[b.RoleID]
		if !ok || !role.Has(p) {
			continue
		}
		switch class {
		case ClassSystem:
			if b.Scope.IsAny() {
				return true
			}
		case ClassCluster:
			if a.coversClusters(b.Scope.Cluster, t.ClusterID) {
				return true
			}
		case ClassApp:
			if a.covers(domain.GroupKindApplication, b.Scope.Application, t.AppID) && a.coversClusters(b.Scope.Cluster, t.ClusterID) {
				return true
			}
		}
	}
	return false
}

func (a *Access) coversClusters(r domain.Ref, id string) bool {
	return a.covers(domain.GroupKindCluster, r, id)
}

// covers reports whether the reference selects the resource of the given kind. An empty id (a
// resource that does not exist yet) is covered only by "any".
func (a *Access) covers(kind domain.GroupKind, r domain.Ref, id string) bool {
	switch r.Kind {
	case domain.RefAny:
		return true
	case domain.RefID:
		return id != "" && r.ID == id
	case domain.RefGroup:
		return id != "" && a.groups != nil && a.groups.Contains(kind, r.ID, id)
	}
	return false
}
