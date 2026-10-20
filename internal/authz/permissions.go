// Package authz decides who may do what: permissions, built-in roles and the evaluator that
// checks a user's role bindings against a target.
package authz

// Permission is a single capability, written "resource:action".
type Permission string

const (
	ClusterView   Permission = "cluster:view"
	ClusterManage Permission = "cluster:manage" // register, edit and remove clusters

	AppView     Permission = "app:view"
	AppSync     Permission = "app:sync"
	AppRollback Permission = "app:rollback"
	AppManage   Permission = "app:manage" // create, edit and delete application definitions

	UserManage  Permission = "user:manage"
	TeamManage  Permission = "team:manage"
	GroupManage Permission = "group:manage"
	RBACManage  Permission = "rbac:manage"
	AuditView   Permission = "audit:view"

	// Wildcard stands for every permission. Only the built-in admin role has it.
	Wildcard Permission = "*"
)

// Class says which part of a binding's scope a permission is checked against.
type Class int

const (
	// ClassCluster permissions look at the scope's cluster dimension only.
	ClassCluster Class = iota + 1
	// ClassApp permissions concern an application on a cluster, so both dimensions must match.
	ClassApp
	// ClassSystem permissions are not tied to a resource: they need a binding that is not
	// narrowed in any dimension.
	ClassSystem
)

var classes = map[Permission]Class{
	ClusterView:   ClassCluster,
	ClusterManage: ClassCluster,
	AppView:       ClassApp,
	AppSync:       ClassApp,
	AppRollback:   ClassApp,
	AppManage:     ClassApp,
	UserManage:    ClassSystem,
	TeamManage:    ClassSystem,
	GroupManage:   ClassSystem,
	RBACManage:    ClassSystem,
	AuditView:     ClassSystem,
}

// Class returns the permission's class, or 0 for unknown permissions and the wildcard.
func (p Permission) Class() Class { return classes[p] }

// Valid reports whether p is a known permission (the wildcard included).
func (p Permission) Valid() bool {
	_, ok := classes[p]
	return ok || p == Wildcard
}

// SystemPermissions lists the permissions that are not tied to a resource, in a stable order.
func SystemPermissions() []Permission {
	return []Permission{UserManage, TeamManage, GroupManage, RBACManage, AuditView}
}
