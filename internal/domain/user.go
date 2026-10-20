package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type User struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	// PasswordHash is never serialized.
	PasswordHash       string     `json:"-"`
	MustChangePassword bool       `json:"mustChangePassword"`
	Disabled           bool       `json:"disabled"`
	CreatedAt          time.Time  `json:"createdAt"`
	CreatedBy          string     `json:"createdBy,omitempty"`
	LastLoginAt        *time.Time `json:"lastLoginAt,omitempty"`
	FailedLogins       int        `json:"-"`
	LockedUntil        *time.Time `json:"-"`
}

var usernameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,62}[a-z0-9]$`)

// NormalizeUsername returns the canonical form used for storage and lookup.
func NormalizeUsername(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// Validate checks the identity fields; the username must already be normalized.
func (u User) Validate() error {
	if !usernameRE.MatchString(u.Username) {
		return fmt.Errorf("username must be 3-64 characters of lowercase letters, digits, '.', '_' and '-', starting and ending with a letter or digit")
	}
	if u.Email != "" && (len(u.Email) > 254 || !strings.Contains(u.Email, "@")) {
		return fmt.Errorf("email is not valid")
	}
	return nil
}

// Session is a refresh-token session. Only a hash of the token is ever stored.
//
// Refresh tokens rotate: using one marks it used and issues a successor in the same family. Using a
// token that was already used means two parties hold the same token, so the whole family is revoked.
type Session struct {
	ID        string
	UserID    string
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
	UserAgent string
	IP        string
	// FamilyID is shared by a session and all its successors, back to the login that started them.
	FamilyID string
	// UsedAt is set when the token was exchanged for a successor.
	UsedAt *time.Time
}
