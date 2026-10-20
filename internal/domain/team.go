package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxNameRunes        = 64
	maxDescriptionRunes = 500
	// MaxMembers bounds a team or resource group, so one request cannot create an unbounded document.
	MaxMembers = 1000
)

// ValidateName checks a team or group name: 1-64 characters, no surrounding spaces, no control characters.
func ValidateName(name string) error {
	n := utf8.RuneCountInString(name)
	if n == 0 || n > maxNameRunes {
		return fmt.Errorf("name must be 1-%d characters", maxNameRunes)
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("name must not start or end with spaces")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return fmt.Errorf("name must not contain control characters")
		}
	}
	return nil
}

func validateDescription(d string) error {
	if utf8.RuneCountInString(d) > maxDescriptionRunes {
		return fmt.Errorf("description must be at most %d characters", maxDescriptionRunes)
	}
	return nil
}

// NameKey is what name uniqueness is decided on: names that differ only in case are the same name.
func NameKey(name string) string { return strings.ToLower(name) }

// NormalizeMembers returns the IDs sorted and without duplicates, never nil.
func NormalizeMembers(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// ValidateMembers checks the member IDs of a team or group (after normalizing they stay unique).
func ValidateMembers(ids []string) error {
	if len(ids) > MaxMembers {
		return fmt.Errorf("at most %d members are allowed", MaxMembers)
	}
	for _, id := range ids {
		if id == "" || len(id) > 128 {
			return fmt.Errorf("member ids must be 1-128 characters")
		}
	}
	return nil
}

// Team is a set of users. A role granted to a team applies to each of its members.
type Team struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Members     []string  `json:"members"` // user IDs, sorted
	CreatedAt   time.Time `json:"createdAt"`
	CreatedBy   string    `json:"createdBy,omitempty"`
}

func (t Team) Validate() error {
	if err := ValidateName(t.Name); err != nil {
		return err
	}
	if err := validateDescription(t.Description); err != nil {
		return err
	}
	return ValidateMembers(t.Members)
}

type GroupKind string

const (
	GroupKindCluster     GroupKind = "cluster"
	GroupKindApplication GroupKind = "application"
)

func (k GroupKind) Valid() bool { return k == GroupKindCluster || k == GroupKindApplication }

// ResourceGroup collects clusters or applications so that access can be granted to the whole set.
type ResourceGroup struct {
	ID          string    `json:"id"`
	Kind        GroupKind `json:"kind"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Members     []string  `json:"members"` // resource IDs, sorted
	CreatedAt   time.Time `json:"createdAt"`
	CreatedBy   string    `json:"createdBy,omitempty"`
}

func (g ResourceGroup) Validate() error {
	if !g.Kind.Valid() {
		return fmt.Errorf("kind must be %q or %q", GroupKindCluster, GroupKindApplication)
	}
	if err := ValidateName(g.Name); err != nil {
		return err
	}
	if err := validateDescription(g.Description); err != nil {
		return err
	}
	return ValidateMembers(g.Members)
}
