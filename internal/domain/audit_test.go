package domain

import (
	"strings"
	"testing"
	"time"
)

func TestAuditResultFromStatus(t *testing.T) {
	tests := map[int]AuditResult{
		200: AuditSuccess, 201: AuditSuccess, 204: AuditSuccess, 302: AuditSuccess,
		401: AuditDenied, 403: AuditDenied, 429: AuditDenied,
		400: AuditFailure, 404: AuditFailure, 409: AuditFailure,
		500: AuditError, 503: AuditError,
	}
	for status, want := range tests {
		if got := AuditResultFromStatus(status); got != want {
			t.Errorf("status %d: got %s, want %s", status, got, want)
		}
	}
	for _, r := range []AuditResult{AuditSuccess, AuditDenied, AuditFailure, AuditError} {
		if !r.Valid() {
			t.Errorf("%s must be valid", r)
		}
	}
	if AuditResult("maybe").Valid() {
		t.Error("an unknown result must be invalid")
	}
}

func TestAuditCursorRoundTrip(t *testing.T) {
	c := AuditCursor{Time: time.Date(2026, 10, 20, 12, 30, 45, 123000000, time.UTC), ID: "abc:def"}
	got, err := DecodeAuditCursor(c.Encode())
	if err != nil || !got.Time.Equal(c.Time) || got.ID != c.ID {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if strings.ContainsAny(c.Encode(), "+/= ") {
		t.Fatal("the cursor must be URL safe")
	}
}

func TestDecodeAuditCursorRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"", "!!!", "bm90LWEtY3Vyc29y", "MTIzOg"} { // empty, not base64, no colon, empty id
		if _, err := DecodeAuditCursor(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestValidateTokenName(t *testing.T) {
	for _, ok := range []string{"ci", "Deploy bot 2", strings.Repeat("a", 64)} {
		if err := ValidateTokenName(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", strings.Repeat("a", 65), "a\tb"} {
		if ValidateTokenName(bad) == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}
