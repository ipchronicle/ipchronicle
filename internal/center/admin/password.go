package admin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	passwordSaltLength  = 16
	passwordKeyLength   = 32
	passwordMemoryKiB   = 64 * 1024
	passwordIterations  = 3
	passwordParallelism = 1
)

type PasswordHasher struct {
	work chan struct{}
}

func NewPasswordHasher() *PasswordHasher {
	return &PasswordHasher{work: make(chan struct{}, 1)}
}

func (h *PasswordHasher) Hash(ctx context.Context, password string) (string, error) {
	if err := h.acquire(ctx); err != nil {
		return "", err
	}
	defer h.release()
	salt := make([]byte, passwordSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, passwordIterations, passwordMemoryKiB, passwordParallelism, passwordKeyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		passwordMemoryKiB,
		passwordIterations,
		passwordParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func (h *PasswordHasher) Verify(ctx context.Context, password, encoded string) (bool, bool, error) {
	parameters, salt, expected, err := parsePasswordHash(encoded)
	if err != nil {
		return false, false, err
	}
	if err := h.acquire(ctx); err != nil {
		return false, false, err
	}
	defer h.release()
	actual := argon2.IDKey([]byte(password), salt, parameters.iterations, parameters.memoryKiB, parameters.parallelism, uint32(len(expected)))
	valid := subtle.ConstantTimeCompare(actual, expected) == 1
	needsRehash := parameters.memoryKiB != passwordMemoryKiB ||
		parameters.iterations != passwordIterations ||
		parameters.parallelism != passwordParallelism ||
		len(expected) != passwordKeyLength || len(salt) != passwordSaltLength
	return valid, needsRehash, nil
}

func (h *PasswordHasher) acquire(ctx context.Context) error {
	select {
	case h.work <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *PasswordHasher) release() {
	<-h.work
}

type passwordParameters struct {
	memoryKiB   uint32
	iterations  uint32
	parallelism uint8
}

func parsePasswordHash(encoded string) (passwordParameters, []byte, []byte, error) {
	var parameters passwordParameters
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return parameters, nil, nil, errors.New("invalid Argon2id password hash format")
	}
	version, err := strconv.Atoi(strings.TrimPrefix(parts[2], "v="))
	if err != nil || version != argon2.Version {
		return parameters, nil, nil, errors.New("unsupported Argon2id version")
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &parameters.memoryKiB, &parameters.iterations, &parameters.parallelism); err != nil {
		return parameters, nil, nil, errors.New("invalid Argon2id parameters")
	}
	if parameters.memoryKiB < 8*1024 || parameters.memoryKiB > 256*1024 ||
		parameters.iterations < 1 || parameters.iterations > 10 ||
		parameters.parallelism < 1 || parameters.parallelism > 4 {
		return parameters, nil, nil, errors.New("Argon2id parameters are outside supported bounds")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return parameters, nil, nil, errors.New("invalid Argon2id salt")
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) < 16 || len(key) > 64 {
		return parameters, nil, nil, errors.New("invalid Argon2id key")
	}
	return parameters, salt, key, nil
}

func ValidateUsername(username string) error {
	if username != strings.TrimSpace(username) {
		return errors.New("username must not start or end with whitespace")
	}
	count := utf8.RuneCountInString(username)
	if count < 1 || count > 64 {
		return errors.New("username must contain between 1 and 64 characters")
	}
	if strings.ContainsAny(username, "\x00\r\n\t") {
		return errors.New("username contains unsupported control characters")
	}
	return nil
}

func ValidateBootstrapPassword(password string) error {
	count := utf8.RuneCountInString(password)
	if count < 1 || count > 128 {
		return errors.New("bootstrap password must contain between 1 and 128 characters")
	}
	return nil
}

func ValidateNewPassword(password string) error {
	count := utf8.RuneCountInString(password)
	if count < 8 || count > 128 {
		return errors.New("password must contain between 8 and 128 characters")
	}
	return nil
}
