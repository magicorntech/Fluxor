package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/magicorntech/Fluxor/internal/authz"
	"github.com/magicorntech/Fluxor/internal/domain"
	"github.com/magicorntech/Fluxor/internal/store"
)

// auditRec collects what the audit log needs to know about one request. The middleware creates it,
// the route wrapper names the action, authentication and the handlers fill in the rest, and the
// middleware writes the event once the response is known.
type auditRec struct {
	action      string
	actorID     string
	actorName   string
	subject     string
	subjectName string
	details     map[string]string
}

type auditKey struct{}

func recFrom(ctx context.Context) *auditRec {
	rec, _ := ctx.Value(auditKey{}).(*auditRec)
	return rec
}

// noteActor says who acted (for requests that are not authenticated by a token, such as a login).
func noteActor(r *http.Request, id, name string) {
	if rec := recFrom(r.Context()); rec != nil {
		rec.actorID, rec.actorName = id, name
	}
}

// noteSubject says what the action was about. Pass the name too: a deleted thing cannot be looked up later.
func noteSubject(r *http.Request, kind, id, name string) {
	if rec := recFrom(r.Context()); rec != nil {
		if id != "" {
			rec.subject = kind + ":" + id
		}
		rec.subjectName = name
	}
}

// noteDetail adds one fact about the action. Never pass secrets: passwords, tokens or hashes.
func noteDetail(r *http.Request, key, value string) {
	if rec := recFrom(r.Context()); rec != nil && value != "" {
		rec.details[key] = value
	}
}

// handle registers a route whose calls are audited under the given action name. If subjectKind is
// given and the route has an {id}, that is recorded as the subject. Reads are registered with
// mux.HandleFunc and are not audited.
func (s *server) handle(mux *http.ServeMux, pattern, action, subjectKind string, h http.HandlerFunc) {
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if rec := recFrom(r.Context()); rec != nil {
			rec.action = action
			if id := r.PathValue("id"); subjectKind != "" && id != "" {
				rec.subject = subjectKind + ":" + id
			}
		}
		h(w, r)
	})
}

// audit wraps the router: it prepares the record before the request runs and writes the event after.
func (s *server) audit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &auditRec{details: map[string]string{}}
		r = r.WithContext(context.WithValue(r.Context(), auditKey{}, rec))
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, r)
		if rec.action == "" {
			return
		}
		// Requests refused by the rate limiter are not recorded: an attacker could otherwise fill the
		// audit log by hammering an endpoint. They are logged to the server log instead.
		if sr.status == http.StatusTooManyRequests {
			return
		}
		s.record(r, rec, sr.status)
	})
}

// newAuditID makes an ID that sorts by creation time (nanoseconds, then a random tail). The log is ordered
// by time and then by ID, so events of the same millisecond still come out in the order they happened.
func newAuditID(now time.Time) string {
	return fmt.Sprintf("%016x%s", uint64(now.UnixNano()), newID()[:8])
}

func (s *server) record(r *http.Request, rec *auditRec, status int) {
	now := time.Now().UTC()
	ev := domain.AuditEvent{
		ID:          newAuditID(now),
		Time:        now.Truncate(time.Millisecond),
		ActorID:     rec.actorID,
		ActorName:   rec.actorName,
		Action:      rec.action,
		Subject:     rec.subject,
		SubjectName: rec.subjectName,
		Result:      domain.AuditResultFromStatus(status),
		Status:      status,
		IP:          s.clientIP(r),
		Details:     rec.details,
	}
	// The response has already gone out, so it must not be cut short if the client disconnects. A
	// failure here is logged and does not change the response: availability wins over completeness.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	if err := s.store.AddAudit(ctx, ev); err != nil {
		s.log.Error("writing an audit event", "action", ev.Action, "err", err)
	}
}

// recordNow writes an event for something that is not a plain request outcome, such as a refresh token
// being reused. The result follows from the status.
func (s *server) recordNow(r *http.Request, action string, status int, actorID, actorName string, details map[string]string) {
	s.record(r, &auditRec{action: action, actorID: actorID, actorName: actorName, details: details}, status)
}

func (s *server) listAudit(w http.ResponseWriter, r *http.Request) {
	if !s.requireSystem(w, r, authz.AuditView) {
		return
	}
	q := r.URL.Query()
	f := store.AuditFilter{ActorID: q.Get("actor"), Action: q.Get("action"), Result: domain.AuditResult(q.Get("result"))}
	if f.Result != "" && !f.Result.Valid() {
		writeError(w, http.StatusBadRequest, "invalid_argument", "result must be success, denied, failure or error")
		return
	}
	for name, dst := range map[string]*time.Time{"from": &f.From, "to": &f.To} {
		if v := q.Get(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_argument", name+" must be an RFC 3339 time, for example 2026-10-20T12:00:00Z")
				return
			}
			*dst = t.UTC()
		}
	}
	limit := 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "limit must be between 1 and 200")
			return
		}
		limit = n
	}
	if v := q.Get("cursor"); v != "" {
		c, err := domain.DecodeAuditCursor(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid cursor")
			return
		}
		f.Before = &c
	}

	f.Limit = limit + 1 // one more than asked, to know whether there is another page
	events, err := s.store.ListAudit(r.Context(), f)
	if err != nil {
		s.internalError(w, err)
		return
	}
	resp := map[string]any{}
	if len(events) > limit {
		events = events[:limit]
		last := events[len(events)-1]
		resp["nextCursor"] = domain.AuditCursor{Time: last.Time, ID: last.ID}.Encode()
	}
	resp["items"] = events
	writeJSON(w, http.StatusOK, resp)
}

// scopeText describes a scope in one short line, for audit details.
func scopeText(sc domain.Scope) string {
	ref := func(r domain.Ref) string {
		if r.Kind == domain.RefAny {
			return "any"
		}
		return string(r.Kind) + ":" + r.ID
	}
	return "cluster=" + ref(sc.Cluster) + " application=" + ref(sc.Application)
}

func joinFields(fields []string) string { return strings.Join(fields, ",") }
