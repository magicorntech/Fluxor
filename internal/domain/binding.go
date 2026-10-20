package domain

import (
	"fmt"
	"time"
)

type SubjectKind string

const (
	SubjectUser SubjectKind = "user"
	SubjectTeam SubjectKind = "team"
)

// Subject is who a role is granted to: a single user or every member of a team.
type Subject struct {
	Kind SubjectKind `json:"kind"`
	ID   string      `json:"id"`
}

type RefKind string

const (
	RefAny   RefKind = "any"   // every resource of the dimension
	RefID    RefKind = "id"    // one resource
	RefGroup RefKind = "group" // every member of a resource group
)

// Ref selects resources along one dimension (clusters or applications) of a scope.
type Ref struct {
	Kind RefKind `json:"kind"`
	ID   string  `json:"id,omitempty"`
}

func AnyRef() Ref { return Ref{Kind: RefAny} }

func (r Ref) Validate() error {
	switch r.Kind {
	case RefAny:
		if r.ID != "" {
			return fmt.Errorf("a reference of kind %q must not have an id", RefAny)
		}
	case RefID, RefGroup:
		if r.ID == "" || len(r.ID) > 128 {
			return fmt.Errorf("a reference of kind %q needs an id of at most 128 characters", r.Kind)
		}
	default:
		return fmt.Errorf("reference kind must be %q, %q or %q", RefAny, RefID, RefGroup)
	}
	return nil
}

// Scope says where a role applies. A permission on an application deployed to a cluster needs
// both dimensions to match; a cluster permission looks at the cluster dimension only.
type Scope struct {
	Cluster     Ref `json:"cluster"`
	Application Ref `json:"application"`
}

func AnyScope() Scope { return Scope{Cluster: AnyRef(), Application: AnyRef()} }

func (s Scope) IsAny() bool { return s.Cluster.Kind == RefAny && s.Application.Kind == RefAny }

// Binding grants a role to a subject within a scope. Permissions only add up; nothing denies.
type Binding struct {
	ID        string    `json:"id"`
	Subject   Subject   `json:"subject"`
	RoleID    string    `json:"roleId"`
	Scope     Scope     `json:"scope"`
	CreatedAt time.Time `json:"createdAt"`
	CreatedBy string    `json:"createdBy,omitempty"`
}

// Validate checks the shape of the binding. Whether the role, subject and referenced
// resources exist is checked by the authorizer.
func (b Binding) Validate() error {
	if b.Subject.Kind != SubjectUser && b.Subject.Kind != SubjectTeam {
		return fmt.Errorf("subject kind must be %q or %q", SubjectUser, SubjectTeam)
	}
	if b.Subject.ID == "" || len(b.Subject.ID) > 128 {
		return fmt.Errorf("subject id is required")
	}
	if b.RoleID == "" {
		return fmt.Errorf("roleId is required")
	}
	if err := b.Scope.Cluster.Validate(); err != nil {
		return fmt.Errorf("scope.cluster: %w", err)
	}
	if err := b.Scope.Application.Validate(); err != nil {
		return fmt.Errorf("scope.application: %w", err)
	}
	return nil
}

// Key identifies the grant itself (who, which role, where). Two bindings with the same key are
// duplicates, so stores keep it unique.
func (b Binding) Key() string {
	return fmt.Sprintf("%s:%s|%s|c=%s:%s|a=%s:%s",
		b.Subject.Kind, b.Subject.ID, b.RoleID,
		b.Scope.Cluster.Kind, b.Scope.Cluster.ID,
		b.Scope.Application.Kind, b.Scope.Application.ID)
}
