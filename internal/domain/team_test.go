package domain

import (
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"simple", "payments", false},
		{"with space and dash", "Platform Eng-2", false},
		{"unicode", "Ödeme Ekibi", false},
		{"max length", strings.Repeat("a", 64), false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 65), true},
		{"leading space", " a", true},
		{"trailing space", "a ", true},
		{"control character", "a\tb", true},
		{"newline", "a\nb", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateName(tt.in); (err != nil) != tt.wantErr {
				t.Fatalf("ValidateName(%q) = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
		})
	}
}

func TestNameKeyIgnoresCase(t *testing.T) {
	if NameKey("Payments") != NameKey("pAYMENTS") || NameKey("Ödeme") != NameKey("ödeme") {
		t.Fatal("names differing only in case must share a key")
	}
	if NameKey("a") == NameKey("b") {
		t.Fatal("different names must have different keys")
	}
}

func TestNormalizeMembers(t *testing.T) {
	got := NormalizeMembers([]string{"b", "a", "b", "c", "a"})
	if strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("got %v", got)
	}
	if empty := NormalizeMembers(nil); empty == nil || len(empty) != 0 {
		t.Fatalf("must be a non-nil empty slice, got %#v", empty)
	}
}

func TestValidateMembers(t *testing.T) {
	if err := ValidateMembers([]string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if ValidateMembers([]string{""}) == nil {
		t.Fatal("empty id accepted")
	}
	if ValidateMembers([]string{strings.Repeat("x", 129)}) == nil {
		t.Fatal("overlong id accepted")
	}
	many := make([]string, MaxMembers+1)
	for i := range many {
		many[i] = "id"
	}
	if ValidateMembers(many) == nil {
		t.Fatal("too many members accepted")
	}
}

func TestTeamAndGroupValidate(t *testing.T) {
	if err := (Team{Name: "payments", Members: []string{"u1"}}).Validate(); err != nil {
		t.Fatal(err)
	}
	if (Team{Name: ""}).Validate() == nil || (Team{Name: "a", Description: strings.Repeat("x", 501)}).Validate() == nil {
		t.Fatal("invalid team accepted")
	}
	if err := (ResourceGroup{Kind: GroupKindCluster, Name: "prod"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if (ResourceGroup{Kind: "nodes", Name: "prod"}).Validate() == nil || (ResourceGroup{Name: "prod"}).Validate() == nil {
		t.Fatal("invalid group kind accepted")
	}
	if !GroupKindApplication.Valid() || GroupKind("x").Valid() {
		t.Fatal("GroupKind.Valid is wrong")
	}
}
