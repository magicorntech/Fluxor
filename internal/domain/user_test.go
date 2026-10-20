package domain

import "testing"

func TestUserValidate(t *testing.T) {
	tests := []struct {
		name    string
		user    User
		wantErr bool
	}{
		{"ok", User{Username: "ayse.yilmaz"}, false},
		{"ok with email", User{Username: "admin", Email: "a@b.co"}, false},
		{"too short", User{Username: "ab"}, true},
		{"uppercase", User{Username: "Admin"}, true},
		{"space", User{Username: "a b c"}, true},
		{"leading dot", User{Username: ".abc"}, true},
		{"trailing dash", User{Username: "abc-"}, true},
		{"bad email", User{Username: "admin", Email: "nope"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.user.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeUsername(t *testing.T) {
	if got := NormalizeUsername("  Ayse.Yilmaz "); got != "ayse.yilmaz" {
		t.Fatalf("got %q", got)
	}
}
