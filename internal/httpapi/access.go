package httpapi

import (
	"net/http"

	"github.com/magicorntech/Fluxor/internal/authz"
)

// access loads what the signed-in user may do. On failure it has already written the response.
func (s *server) access(w http.ResponseWriter, r *http.Request) (*authz.Access, bool) {
	u, ok := UserFromContext(r.Context())
	if !ok {
		unauthenticated(w)
		return nil, false
	}
	access, err := s.authz.For(r.Context(), u.ID)
	if err != nil {
		s.internalError(w, err)
		return nil, false
	}
	return access, true
}

// requireSystem lets the request through only if the user holds the system permission
// (user management, access control, ...). Otherwise it has written a 403 and returns false.
func (s *server) requireSystem(w http.ResponseWriter, r *http.Request, p authz.Permission) bool {
	access, ok := s.access(w, r)
	if !ok {
		return false
	}
	if !access.Can(p, authz.Target{}) {
		forbidden(w)
		return false
	}
	return true
}

func forbidden(w http.ResponseWriter) {
	writeError(w, http.StatusForbidden, "forbidden", "you do not have permission to perform this action")
}
