// Package httpapi implements the REST API described in api/openapi.yaml.
package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/magicorntech/Fluxor/internal/auth"
	"github.com/magicorntech/Fluxor/internal/authz"
	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
	"github.com/magicorntech/Fluxor/internal/version"
)

type Options struct {
	Store       store.Store
	Auth        *auth.Service
	Authz       *authz.Authorizer
	Logger      *slog.Logger
	CORSOrigins []string
	// CookieSecure marks the refresh cookie Secure (HTTPS only). Enable it whenever
	// the server is reached over HTTPS; leave it off for plain-HTTP local development.
	CookieSecure bool
	// TrustedProxies are the reverse proxies whose X-Forwarded-For header is believed. Leave empty when
	// clients connect directly; otherwise every request would appear to come from the proxy.
	TrustedProxies []netip.Prefix
	// LoginPerMinute and RefreshPerMinute limit attempts per client address: that many at once, then that
	// many per minute. Zero means the default (10 and 60); a negative value turns the limit off.
	LoginPerMinute, RefreshPerMinute int
	// Now is the clock of the rate limiter; tests replace it. Nil means time.Now.
	Now func() time.Time
}

const (
	defaultLoginPerMinute   = 10
	defaultRefreshPerMinute = 60
)

func orDefault(n, def int) int {
	if n == 0 {
		return def
	}
	return n
}

type server struct {
	store          store.Store
	auth           *auth.Service
	authz          *authz.Authorizer
	log            *slog.Logger
	cookieSecure   bool
	trustedProxies []netip.Prefix
	loginLimit     *rateLimiter
	refreshLimit   *rateLimiter
}

// New builds the HTTP handler for the whole API, middleware included.
// Every route except the ones listed in publicRoutes requires a valid access token.
func New(opts Options) http.Handler {
	if opts.Auth == nil || opts.Authz == nil {
		panic("httpapi: Options.Auth and Options.Authz are required")
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	s := &server{
		store: opts.Store, auth: opts.Auth, authz: opts.Authz, log: log, cookieSecure: opts.CookieSecure,
		trustedProxies: opts.TrustedProxies,
		loginLimit:     newRateLimiter(orDefault(opts.LoginPerMinute, defaultLoginPerMinute), opts.Now),
		refreshLimit:   newRateLimiter(orDefault(opts.RefreshPerMinute, defaultRefreshPerMinute), opts.Now),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.getHealth)

	// Routes registered with s.handle are audited under the action name given; the others only read.
	s.handle(mux, "POST /api/v1/auth/login", "auth.login", "", s.login)
	mux.HandleFunc("POST /api/v1/auth/refresh", s.refresh)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("GET /api/v1/auth/me", s.me)
	s.handle(mux, "PUT /api/v1/auth/password", "auth.password_change", "", s.changePassword)

	mux.HandleFunc("GET /api/v1/tokens", s.listTokens)
	s.handle(mux, "POST /api/v1/tokens", "token.create", "", s.createToken)
	s.handle(mux, "DELETE /api/v1/tokens/{id}", "token.revoke", "token", s.deleteToken)

	mux.HandleFunc("GET /api/v1/audit", s.listAudit)

	mux.HandleFunc("GET /api/v1/users", s.listUsers)
	s.handle(mux, "POST /api/v1/users", "user.create", "", s.createUser)
	mux.HandleFunc("GET /api/v1/users/{id}", s.getUser)
	s.handle(mux, "PATCH /api/v1/users/{id}", "user.update", "user", s.updateUser)
	s.handle(mux, "POST /api/v1/users/{id}/reset-password", "user.reset_password", "user", s.resetUserPassword)

	mux.HandleFunc("GET /api/v1/teams", s.listTeams)
	s.handle(mux, "POST /api/v1/teams", "team.create", "", s.createTeam)
	mux.HandleFunc("GET /api/v1/teams/{id}", s.getTeam)
	s.handle(mux, "PATCH /api/v1/teams/{id}", "team.update", "team", s.updateTeam)
	s.handle(mux, "DELETE /api/v1/teams/{id}", "team.delete", "team", s.deleteTeam)
	s.handle(mux, "PUT /api/v1/teams/{id}/members", "team.set_members", "team", s.setTeamMembers)

	mux.HandleFunc("GET /api/v1/resource-groups", s.listGroups)
	s.handle(mux, "POST /api/v1/resource-groups", "group.create", "", s.createGroup)
	mux.HandleFunc("GET /api/v1/resource-groups/{id}", s.getGroup)
	s.handle(mux, "PATCH /api/v1/resource-groups/{id}", "group.update", "group", s.updateGroup)
	s.handle(mux, "DELETE /api/v1/resource-groups/{id}", "group.delete", "group", s.deleteGroup)
	s.handle(mux, "PUT /api/v1/resource-groups/{id}/members", "group.set_members", "group", s.setGroupMembers)

	mux.HandleFunc("GET /api/v1/roles", s.listRoles)
	mux.HandleFunc("GET /api/v1/role-bindings", s.listRoleBindings)
	s.handle(mux, "POST /api/v1/role-bindings", "role_binding.create", "", s.createRoleBinding)
	s.handle(mux, "DELETE /api/v1/role-bindings/{id}", "role_binding.delete", "role_binding", s.deleteRoleBinding)

	mux.HandleFunc("GET /api/v1/clusters", s.listClusters)
	s.handle(mux, "POST /api/v1/clusters", "cluster.create", "", s.createCluster)
	mux.HandleFunc("GET /api/v1/clusters/{id}", s.getCluster)
	s.handle(mux, "DELETE /api/v1/clusters/{id}", "cluster.delete", "cluster", s.deleteCluster)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	})

	return chain(s.audit(s.authn(mux)), recoverer(log), cors(opts.CORSOrigins), requestLog(log))
}

func (s *server) getHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version.Version})
}

// clusterResponse is a cluster together with what the signed-in user may do with it, so the
// dashboard can hide buttons that would only produce a 403.
type clusterResponse struct {
	domain.Cluster
	Permissions []authz.Permission `json:"permissions"`
}

func clusterView(access *authz.Access, c domain.Cluster) clusterResponse {
	perms := []authz.Permission{}
	for _, p := range []authz.Permission{authz.ClusterView, authz.ClusterManage} {
		if access.Can(p, authz.Target{ClusterID: c.ID}) {
			perms = append(perms, p)
		}
	}
	return clusterResponse{Cluster: c, Permissions: perms}
}

func (s *server) listClusters(w http.ResponseWriter, r *http.Request) {
	access, ok := s.access(w, r)
	if !ok {
		return
	}
	all, err := s.store.ListClusters(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	// A user sees only the clusters they may view; having none is an empty list, not an error.
	items := make([]clusterResponse, 0, len(all))
	for _, c := range all {
		if access.Can(authz.ClusterView, authz.Target{ClusterID: c.ID}) {
			items = append(items, clusterView(access, c))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type clusterInput struct {
	Name      string            `json:"name"`
	APIServer string            `json:"apiServer"`
	Labels    map[string]string `json:"labels"`
}

func (s *server) createCluster(w http.ResponseWriter, r *http.Request) {
	access, ok := s.access(w, r)
	if !ok {
		return
	}
	// A cluster that does not exist yet is covered only by a grant over all clusters.
	if !access.Can(authz.ClusterManage, authz.Target{}) {
		forbidden(w)
		return
	}
	var in clusterInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Labels == nil {
		in.Labels = map[string]string{}
	}
	c := domain.Cluster{
		ID:        newID(),
		Name:      in.Name,
		APIServer: in.APIServer,
		Labels:    in.Labels,
		Status:    domain.ClusterStatusUnknown,
		CreatedAt: time.Now().UTC(),
	}
	if err := c.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
		return
	}
	if err := s.store.CreateCluster(r.Context(), c); err != nil {
		if errors.Is(err, store.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, "already_exists", "a cluster named "+c.Name+" already exists")
			return
		}
		s.internalError(w, err)
		return
	}
	noteSubject(r, "cluster", c.ID, c.Name)
	writeJSON(w, http.StatusCreated, clusterView(access, c))
}

func (s *server) getCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	access, ok := s.access(w, r)
	if !ok {
		return
	}
	// Clusters the user may not view look exactly like clusters that do not exist.
	if !access.Can(authz.ClusterView, authz.Target{ClusterID: id}) {
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
		return
	}
	c, err := s.store.GetCluster(r.Context(), id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, clusterView(access, c))
}

func (s *server) deleteCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	access, ok := s.access(w, r)
	if !ok {
		return
	}
	target := authz.Target{ClusterID: id}
	if !access.Can(authz.ClusterView, target) {
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
		return
	}
	if !access.Can(authz.ClusterManage, target) {
		forbidden(w)
		return
	}
	cluster, err := s.store.GetCluster(r.Context(), id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	noteSubject(r, "cluster", cluster.ID, cluster.Name)
	if err := s.store.DeleteCluster(r.Context(), id); err != nil {
		s.storeError(w, err)
		return
	}
	s.dropBindingsForCluster(r, id)
	if err := s.store.RemoveFromGroups(r.Context(), domain.GroupKindCluster, id); err != nil {
		s.log.Error("removing a deleted cluster from its groups", "cluster", id, "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// dropBindingsForCluster removes grants that point at a cluster that no longer exists, so they do
// not linger as unresolvable entries. It is best effort: failures are logged, not reported.
func (s *server) dropBindingsForCluster(r *http.Request, clusterID string) {
	bindings, err := s.store.ListBindings(r.Context(), store.BindingFilter{})
	if err != nil {
		s.log.Error("listing bindings after cluster delete", "cluster", clusterID, "err", err)
		return
	}
	for _, b := range bindings {
		if b.Scope.Cluster.Kind == domain.RefID && b.Scope.Cluster.ID == clusterID {
			if err := s.store.DeleteBinding(r.Context(), b.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
				s.log.Error("deleting binding of a removed cluster", "binding", b.ID, "err", err)
			}
		}
	}
}

func (s *server) storeError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
		return
	}
	s.internalError(w, err)
}

func (s *server) internalError(w http.ResponseWriter, err error) {
	s.log.Error("request failed", "err", err)
	writeError(w, http.StatusInternalServerError, "internal", "internal error")
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
