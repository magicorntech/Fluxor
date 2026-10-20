package domain

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type AuditResult string

const (
	AuditSuccess AuditResult = "success"
	AuditDenied  AuditResult = "denied"  // not signed in, not allowed, or rate limited
	AuditFailure AuditResult = "failure" // the request was refused for another reason (validation, conflict, not found)
	AuditError   AuditResult = "error"   // the server failed
)

func (r AuditResult) Valid() bool {
	return r == AuditSuccess || r == AuditDenied || r == AuditFailure || r == AuditError
}

// AuditResultFromStatus classifies an HTTP status.
func AuditResultFromStatus(status int) AuditResult {
	switch {
	case status >= 200 && status < 400:
		return AuditSuccess
	case status == 401 || status == 403 || status == 429:
		return AuditDenied
	case status >= 500:
		return AuditError
	default:
		return AuditFailure
	}
}

// AuditEvent records one action on the system. It never holds secrets: no passwords, tokens or hashes.
type AuditEvent struct {
	ID   string    `json:"id"`
	Time time.Time `json:"time"`
	// ActorID is the user who acted; empty when nobody was signed in (a failed login).
	ActorID string `json:"actorId,omitempty"`
	// ActorName is the username at the time; for a failed login it is the name that was tried.
	ActorName string `json:"actorName,omitempty"`
	// Action is a stable dotted name such as "user.create" or "auth.login".
	Action string `json:"action"`
	// Subject is what the action was about, as "kind:id" (for example "cluster:ab12"); empty if none.
	Subject string `json:"subject,omitempty"`
	// SubjectName is the subject's name at the time, since a deleted subject cannot be looked up later.
	SubjectName string      `json:"subjectName,omitempty"`
	Result      AuditResult `json:"result"`
	// Status is the HTTP status of the response.
	Status  int               `json:"status"`
	IP      string            `json:"ip,omitempty"`
	Details map[string]string `json:"details"`
}

// AuditCursor marks a position in the newest-first event list, for paging.
type AuditCursor struct {
	Time time.Time
	ID   string
}

// Encode returns an opaque string for the API.
func (c AuditCursor) Encode() string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.Time.UnixNano(), 10) + ":" + c.ID))
}

// DecodeAuditCursor parses what Encode produced.
func DecodeAuditCursor(s string) (AuditCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return AuditCursor{}, fmt.Errorf("invalid cursor")
	}
	ns, id, ok := strings.Cut(string(raw), ":")
	n, perr := strconv.ParseInt(ns, 10, 64)
	if !ok || perr != nil || id == "" {
		return AuditCursor{}, fmt.Errorf("invalid cursor")
	}
	return AuditCursor{Time: time.Unix(0, n).UTC(), ID: id}, nil
}
