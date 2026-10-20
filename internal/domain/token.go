package domain

import (
	"fmt"
	"time"
	"unicode"
	"unicode/utf8"
)

// TokenPrefix starts every API token, so that one can be recognized (and told apart from a session
// token) without looking it up, and so that secret scanners can find leaked ones.
const TokenPrefix = "flx_"

// MaxTokensPerUser bounds how many API tokens one user can hold.
const MaxTokensPerUser = 20

// APIToken lets a script act as its owner without a password. It has exactly the owner's permissions.
// Only a hash of the secret is stored; the secret is shown once, when the token is created.
type APIToken struct {
	ID     string `json:"id"`
	UserID string `json:"userId"`
	Name   string `json:"name"`
	// Prefix is the first characters of the secret, to recognize the token in a list.
	Prefix     string     `json:"prefix"`
	TokenHash  string     `json:"-"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
}

func ValidateTokenName(name string) error {
	if n := utf8.RuneCountInString(name); n == 0 || n > 64 {
		return fmt.Errorf("name must be 1-64 characters")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return fmt.Errorf("name must not contain control characters")
		}
	}
	return nil
}
