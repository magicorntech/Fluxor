package store

import (
	"context"
	"errors"
	"time"

	"github.com/magicorntech/Fluxor/internal/domain"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	// ErrSessionUsed is returned by ConsumeSession for a refresh token that was already exchanged.
	ErrSessionUsed = errors.New("session already used")
)

type ClusterStore interface {
	ListClusters(ctx context.Context) ([]domain.Cluster, error)
	GetCluster(ctx context.Context, id string) (domain.Cluster, error)
	// CreateCluster returns ErrAlreadyExists when the name is taken.
	CreateCluster(ctx context.Context, c domain.Cluster) error
	DeleteCluster(ctx context.Context, id string) error
}

type UserStore interface {
	CountUsers(ctx context.Context) (int64, error)
	// CreateUser returns ErrAlreadyExists when the username or a non-empty email is taken.
	CreateUser(ctx context.Context, u domain.User) error
	GetUser(ctx context.Context, id string) (domain.User, error)
	// GetUserByUsername expects a normalized username.
	GetUserByUsername(ctx context.Context, username string) (domain.User, error)
	// UpdateUser replaces the stored user with the same ID; ErrNotFound if missing,
	// ErrAlreadyExists if the new email collides with another user.
	UpdateUser(ctx context.Context, u domain.User) error
	// ListUsers returns all users sorted by username.
	ListUsers(ctx context.Context) ([]domain.User, error)
}

// BindingFilter narrows ListBindings; zero fields match everything.
type BindingFilter struct {
	// Subjects matches bindings granted to any of these subjects.
	Subjects []domain.Subject
	RoleID   string
}

type BindingStore interface {
	// CreateBinding returns ErrAlreadyExists when the ID is taken or an identical grant
	// (same Key) exists.
	CreateBinding(ctx context.Context, b domain.Binding) error
	GetBinding(ctx context.Context, id string) (domain.Binding, error)
	// ListBindings returns matching bindings ordered by creation time, then ID.
	ListBindings(ctx context.Context, f BindingFilter) ([]domain.Binding, error)
	DeleteBinding(ctx context.Context, id string) error
}

type SessionStore interface {
	CreateSession(ctx context.Context, s domain.Session) error
	// ConsumeSession atomically marks the session with this token hash as used at `now` and returns it
	// (as it was before, so UsedAt is nil), so a refresh token can be exchanged at most once.
	// ErrNotFound if there is no such session. If it was already used, the stored session is returned
	// together with ErrSessionUsed and nothing changes.
	ConsumeSession(ctx context.Context, tokenHash string, now time.Time) (domain.Session, error)
	// DeleteSession removes a session; it is not an error if it does not exist.
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteUserSessions(ctx context.Context, userID string) error
	// DeleteFamily removes every session of a family. An empty ID removes nothing.
	DeleteFamily(ctx context.Context, familyID string) error
}

type TokenStore interface {
	// CreateToken returns ErrAlreadyExists when the ID or the hash is taken.
	CreateToken(ctx context.Context, t domain.APIToken) error
	// GetTokenByHash returns ErrNotFound for an unknown hash. Expired tokens may still be returned;
	// the caller checks ExpiresAt.
	GetTokenByHash(ctx context.Context, hash string) (domain.APIToken, error)
	// ListTokens returns the user's tokens, newest first.
	ListTokens(ctx context.Context, userID string) ([]domain.APIToken, error)
	CountTokens(ctx context.Context, userID string) (int64, error)
	// DeleteToken removes a token only if it belongs to the user; otherwise ErrNotFound.
	DeleteToken(ctx context.Context, userID, id string) error
	DeleteUserTokens(ctx context.Context, userID string) error
	// TouchToken records when the token was last used.
	TouchToken(ctx context.Context, id string, at time.Time) error
}

// AuditFilter narrows ListAudit; zero fields match everything.
type AuditFilter struct {
	ActorID string
	Action  string
	Result  domain.AuditResult
	// From is inclusive and To exclusive; a zero time means unbounded.
	From, To time.Time
	// Before continues after the last event of the previous page.
	Before *domain.AuditCursor
	// Limit is the page size; zero or less means 100.
	Limit int
}

type AuditStore interface {
	AddAudit(ctx context.Context, e domain.AuditEvent) error
	// ListAudit returns events newest first (by time, then ID, descending).
	ListAudit(ctx context.Context, f AuditFilter) ([]domain.AuditEvent, error)
}

type TeamStore interface {
	// CreateTeam returns ErrAlreadyExists when the name (compared ignoring case) is taken.
	CreateTeam(ctx context.Context, t domain.Team) error
	GetTeam(ctx context.Context, id string) (domain.Team, error)
	// ListTeams returns all teams sorted by name (ignoring case).
	ListTeams(ctx context.Context) ([]domain.Team, error)
	// ListTeamsOfUser returns the teams the user is a member of.
	ListTeamsOfUser(ctx context.Context, userID string) ([]domain.Team, error)
	// UpdateTeam changes the name and description only; members are left alone, so a concurrent
	// membership change is not lost. ErrNotFound / ErrAlreadyExists (name taken).
	UpdateTeam(ctx context.Context, t domain.Team) error
	// SetTeamMembers replaces the member list (user IDs). ErrNotFound if the team is missing.
	SetTeamMembers(ctx context.Context, id string, members []string) error
	DeleteTeam(ctx context.Context, id string) error
}

// GroupFilter narrows ListGroups; zero fields match everything.
type GroupFilter struct {
	Kind domain.GroupKind
	// IDs restricts the result to these groups. An empty slice means "no restriction", so callers
	// that have no IDs must not call ListGroups at all.
	IDs []string
}

type GroupStore interface {
	// CreateGroup returns ErrAlreadyExists when the name (ignoring case) is taken within the kind.
	CreateGroup(ctx context.Context, g domain.ResourceGroup) error
	GetGroup(ctx context.Context, id string) (domain.ResourceGroup, error)
	// ListGroups returns matching groups sorted by name (ignoring case).
	ListGroups(ctx context.Context, f GroupFilter) ([]domain.ResourceGroup, error)
	// UpdateGroup changes the name and description only (not the kind or the members).
	UpdateGroup(ctx context.Context, g domain.ResourceGroup) error
	// SetGroupMembers replaces the member list (resource IDs). ErrNotFound if the group is missing.
	SetGroupMembers(ctx context.Context, id string, members []string) error
	DeleteGroup(ctx context.Context, id string) error
	// RemoveFromGroups takes a resource out of every group of the kind, for when it is deleted.
	RemoveFromGroups(ctx context.Context, kind domain.GroupKind, resourceID string) error
}

// Store is the persistence boundary of the server. Implementations live in
// subpackages (memory for tests and local development, mongo for real use).
type Store interface {
	ClusterStore
	UserStore
	SessionStore
	BindingStore
	TeamStore
	GroupStore
	TokenStore
	AuditStore
}
