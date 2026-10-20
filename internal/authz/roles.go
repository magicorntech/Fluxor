package authz

// Role is a named set of permissions. The built-in roles cannot be changed; their ID is their name.
type Role struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Builtin     bool         `json:"builtin"`
	Permissions []Permission `json:"permissions"`
}

const (
	RoleViewer       = "viewer"
	RoleDeployer     = "deployer"
	RoleMaintainer   = "maintainer"
	RoleClusterAdmin = "cluster-admin"
	RoleAdmin        = "admin"
)

var viewerPerms = []Permission{ClusterView, AppView}

func with(base []Permission, more ...Permission) []Permission {
	out := make([]Permission, 0, len(base)+len(more))
	out = append(out, base...)
	return append(out, more...)
}

var builtinRoles = []Role{
	{ID: RoleViewer, Name: "Viewer", Builtin: true,
		Description: "See clusters and applications",
		Permissions: viewerPerms},
	{ID: RoleDeployer, Name: "Deployer", Builtin: true,
		Description: "Viewer, plus sync and roll back applications",
		Permissions: with(viewerPerms, AppSync, AppRollback)},
	{ID: RoleMaintainer, Name: "Maintainer", Builtin: true,
		Description: "Deployer, plus create, edit and delete applications",
		Permissions: with(viewerPerms, AppSync, AppRollback, AppManage)},
	{ID: RoleClusterAdmin, Name: "Cluster admin", Builtin: true,
		Description: "See and manage clusters",
		Permissions: []Permission{ClusterView, ClusterManage}},
	{ID: RoleAdmin, Name: "Administrator", Builtin: true,
		Description: "Everything, including users, teams and access control",
		Permissions: []Permission{Wildcard}},
}

// Roles returns the built-in roles.
func Roles() []Role {
	out := make([]Role, len(builtinRoles))
	for i, r := range builtinRoles {
		r.Permissions = append([]Permission(nil), r.Permissions...)
		out[i] = r
	}
	return out
}

// LookupRole finds a built-in role by ID.
func LookupRole(id string) (Role, bool) {
	for _, r := range Roles() {
		if r.ID == id {
			return r, true
		}
	}
	return Role{}, false
}

// Has reports whether the role grants p.
func (r Role) Has(p Permission) bool {
	for _, have := range r.Permissions {
		if have == Wildcard || have == p {
			return true
		}
	}
	return false
}

// HasClass reports whether the role grants any permission of the class.
func (r Role) HasClass(c Class) bool {
	for _, have := range r.Permissions {
		if have == Wildcard || have.Class() == c {
			return true
		}
	}
	return false
}

// IsSystem reports whether the role grants any system permission (the wildcard included).
// Such roles are only valid when bound without narrowing the scope.
func (r Role) IsSystem() bool { return r.HasClass(ClassSystem) }
