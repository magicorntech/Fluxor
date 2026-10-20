// Package auth implements local accounts: password hashing, access/refresh tokens and sessions.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	MinPasswordLength = 12
	maxPasswordBytes  = 256
	maxArgonMemoryKiB = 512 * 1024 // refuse absurd parameters coming from a stored hash
)

// Params are the argon2id cost parameters. Memory is in KiB.
type Params struct {
	Memory  uint32
	Time    uint32
	Threads uint8
	SaltLen uint32
	KeyLen  uint32
}

// DefaultParams follow the OWASP recommendation for argon2id (64 MiB, 3 passes).
var DefaultParams = Params{Memory: 64 * 1024, Time: 3, Threads: 2, SaltLen: 16, KeyLen: 32}

type Hasher struct{ p Params }

func NewHasher(p Params) Hasher { return Hasher{p: p} }

// Hash returns a PHC-formatted argon2id hash: $argon2id$v=19$m=..,t=..,p=..$salt$key
func (h Hasher) Hash(password string) (string, error) {
	salt := make([]byte, h.p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, h.p.Time, h.p.Memory, h.p.Threads, h.p.KeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, h.p.Memory, h.p.Time, h.p.Threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify checks a password against a stored hash using the parameters recorded in the hash.
// A malformed hash is an error; a wrong password is (false, nil).
func (h Hasher) Verify(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false, errors.New("unsupported password hash format")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errors.New("unsupported argon2 version")
	}
	var p Params
	var threads uint32
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &threads); err != nil {
		return false, errors.New("invalid argon2 parameters")
	}
	if p.Memory == 0 || p.Memory > maxArgonMemoryKiB || p.Time == 0 || threads == 0 || threads > 255 {
		return false, errors.New("argon2 parameters out of range")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errors.New("invalid salt encoding")
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, errors.New("invalid key encoding")
	}
	got := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, uint8(threads), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// ErrWeakPassword wraps the reason a password was rejected.
var ErrWeakPassword = errors.New("password rejected")

// ValidatePassword enforces the password policy.
func ValidatePassword(pw string) error {
	if len(pw) > maxPasswordBytes {
		return fmt.Errorf("%w: password must be at most %d bytes", ErrWeakPassword, maxPasswordBytes)
	}
	if utf8.RuneCountInString(pw) < MinPasswordLength {
		return fmt.Errorf("%w: password must be at least %d characters", ErrWeakPassword, MinPasswordLength)
	}
	return nil
}
