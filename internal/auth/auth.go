package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	memory      uint32 = 32 * 1024
	iterations  uint32 = 2
	parallelism uint8  = 1
	keyLength   uint32 = 32
)

func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", fmt.Errorf("password must be at least 8 characters")
	}
	return hashArgon2id(password)
}

func HashSharePassword(password string) (string, error) {
	if len(strings.TrimSpace(password)) < 1 {
		return "", fmt.Errorf("password cannot be empty")
	}
	return hashArgon2id(password)
}

func hashArgon2id(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)
	return fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, iterations, parallelism, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func VerifyPassword(encoded, password string) bool {
	p := strings.Split(encoded, "$")
	if len(p) != 5 || p[0] != "argon2id" || p[1] != "v=19" {
		return false
	}
	var m uint32
	var t uint32
	var par uint8
	if _, err := fmt.Sscanf(p[2], "m=%d,t=%d,p=%d", &m, &t, &par); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(p[3])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(p[4])
	if err != nil {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, t, m, par, uint32(len(expected)))
	return subtle.ConstantTimeCompare(expected, actual) == 1
}

func Token() (raw string, hash []byte, err error) {
	b := make([]byte, 32)
	_, err = rand.Read(b)
	if err != nil {
		return "", nil, err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(raw))
	return raw, sum[:], nil
}
func TokenFromRaw(raw string) (string, []byte, error) {
	if _, err := base64.RawURLEncoding.DecodeString(raw); err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256([]byte(raw))
	return raw, sum[:], nil
}
func CSRF(secret []byte) string {
	sum := sha256.Sum256(secret)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
