package controlapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const lanPasswordPrefix = "$argon2id$v=19$m=19456,t=2,p=1$"

func validLANPassword(password string) bool {
	return utf8.ValidString(password) && utf8.RuneCountInString(password) >= 12 && utf8.RuneCountInString(password) <= 128
}

func hashLANPassword(password string) (string, error) {
	if !validLANPassword(password) {
		return "", fmt.Errorf("password must contain 12 to 128 characters")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
	return lanPasswordPrefix + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(sum), nil
}

func parseLANPassword(encoded string) ([]byte, []byte, bool) {
	if !strings.HasPrefix(encoded, lanPasswordPrefix) {
		return nil, nil, false
	}
	parts := strings.Split(strings.TrimPrefix(encoded, lanPasswordPrefix), "$")
	if len(parts) != 2 {
		return nil, nil, false
	}
	salt, e1 := base64.RawStdEncoding.DecodeString(parts[0])
	sum, e2 := base64.RawStdEncoding.DecodeString(parts[1])
	return salt, sum, e1 == nil && e2 == nil && len(salt) == 16 && len(sum) == 32
}

func verifyLANPassword(encoded, password string) bool {
	salt, expected, ok := parseLANPassword(encoded)
	if !ok || !validLANPassword(password) {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}
