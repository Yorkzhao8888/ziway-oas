// Package password provides argon2id password hashing with bcrypt backward compatibility.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

var (
	// Argon2id parameters (recommended for memory-hard hashing)
	Time    = uint32(1)
	Memory  = uint32(64 * 1024)
	Threads = uint8(4)
	KeyLen  = uint32(32)
	SaltLen = uint32(16)
)

// Hash generates an argon2id hash of the password.
// Format: $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
func Hash(password string) (string, error) {
	salt := make([]byte, SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	hash := argon2.IDKey([]byte(password), salt, Time, Memory, Threads, KeyLen)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, Memory, Time, Threads, b64Salt, b64Hash), nil
}

// Verify checks a password against a hash.
// Supports both argon2id and bcrypt (legacy) hashes.
func Verify(password, hash string) error {
	if strings.HasPrefix(hash, "$argon2id$") {
		return verifyArgon2(password, hash)
	}
	// Legacy bcrypt support
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func verifyArgon2(password, encodedHash string) error {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return errors.New("invalid argon2 hash format")
	}

	var version int
	var memory uint32
	var time uint32
	var threads uint8
	_, err := fmt.Sscanf(parts[2], "v=%d", &version)
	if err != nil {
		return err
	}
	if version != argon2.Version {
		return errors.New("incompatible argon2 version")
	}

	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads)
	if err != nil {
		return err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return err
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return err
	}

	hash := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(expectedHash)))

	if subtle.ConstantTimeCompare(hash, expectedHash) != 1 {
		return errors.New("password mismatch")
	}
	return nil
}
