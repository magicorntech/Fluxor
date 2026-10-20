package auth

import (
	"strings"
	"testing"
)

var testParams = Params{Memory: 8, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

func TestHashVerify(t *testing.T) {
	h := NewHasher(testParams)
	enc, err := h.Hash("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, "$argon2id$v=19$m=8,t=1,p=1$") {
		t.Fatalf("unexpected format: %s", enc)
	}
	if ok, err := h.Verify(enc, "correct horse battery"); !ok || err != nil {
		t.Fatalf("correct password rejected: %v %v", ok, err)
	}
	if ok, err := h.Verify(enc, "wrong password!"); ok || err != nil {
		t.Fatalf("wrong password accepted: %v %v", ok, err)
	}

	again, _ := h.Hash("correct horse battery")
	if again == enc {
		t.Fatal("two hashes of the same password must differ (random salt)")
	}
}

func TestVerifyUsesParamsFromHash(t *testing.T) {
	enc, _ := NewHasher(testParams).Hash("some long password")
	// A hasher configured with different parameters must still verify old hashes.
	other := NewHasher(Params{Memory: 16, Time: 2, Threads: 2, SaltLen: 16, KeyLen: 32})
	if ok, err := other.Verify(enc, "some long password"); !ok || err != nil {
		t.Fatalf("hash created with other params rejected: %v %v", ok, err)
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	h := NewHasher(testParams)
	bad := []string{
		"",
		"plaintext",
		"$bcrypt$v=19$m=8,t=1,p=1$c2FsdA$a2V5",
		"$argon2id$v=18$m=8,t=1,p=1$c2FsdA$a2V5",
		"$argon2id$v=19$m=x,t=1,p=1$c2FsdA$a2V5",
		"$argon2id$v=19$m=8,t=1,p=1$!!!$a2V5",
		"$argon2id$v=19$m=8,t=1,p=1$c2FsdA$",
		"$argon2id$v=19$m=0,t=1,p=1$c2FsdA$a2V5",
		"$argon2id$v=19$m=8,t=0,p=1$c2FsdA$a2V5",
		"$argon2id$v=19$m=8,t=1,p=0$c2FsdA$a2V5",
		"$argon2id$v=19$m=8,t=1,p=300$c2FsdA$a2V5",
		"$argon2id$v=19$m=99999999,t=1,p=1$c2FsdA$a2V5", // would exhaust memory
	}
	for _, enc := range bad {
		if ok, err := h.Verify(enc, "whatever password"); ok || err == nil {
			t.Errorf("Verify(%q) = %v, %v; want an error", enc, ok, err)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name    string
		pw      string
		wantErr bool
	}{
		{"ok", "123456789012", false},
		{"too short", "12345678901", true},
		{"empty", "", true},
		{"counts characters, not bytes", "çççççççççççç", false}, // 12 runes, 24 bytes
		{"too long", strings.Repeat("a", 257), true},
		{"max length", strings.Repeat("a", 256), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.pw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePassword() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
