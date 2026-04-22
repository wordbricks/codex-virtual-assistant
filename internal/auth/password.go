package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	minPasswordLength = 12
	maxPasswordLength = 1024

	argon2Version = 19
)

var (
	ErrInvalidPassword = errors.New("password does not meet security policy")
	ErrInvalidHash     = errors.New("invalid password hash")
)

type PasswordParams struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

var DefaultPasswordParams = PasswordParams{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

func ValidateUserID(userID string) error {
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return errors.New("user id is required")
	}
	if trimmed != userID {
		return errors.New("user id must not have surrounding whitespace")
	}
	if utf8.RuneCountInString(userID) > 128 {
		return errors.New("user id must be at most 128 characters")
	}
	for _, r := range userID {
		if unicode.IsControl(r) || r == ':' {
			return errors.New("user id contains an invalid character")
		}
	}
	return nil
}

func ValidatePasswordQuality(password string) error {
	if password == "" {
		return fmt.Errorf("%w: password is required", ErrInvalidPassword)
	}
	if len(password) > maxPasswordLength {
		return fmt.Errorf("%w: password is too long", ErrInvalidPassword)
	}
	if utf8.RuneCountInString(password) < minPasswordLength {
		return fmt.Errorf("%w: password must be at least %d characters", ErrInvalidPassword, minPasswordLength)
	}
	for _, r := range password {
		if r == 0 || unicode.IsControl(r) {
			return fmt.Errorf("%w: password contains a control character", ErrInvalidPassword)
		}
	}
	return nil
}

func HashPassword(password string) (string, error) {
	return HashPasswordWithParams(password, DefaultPasswordParams)
}

func HashPasswordWithParams(password string, params PasswordParams) (string, error) {
	if err := ValidatePasswordQuality(password); err != nil {
		return "", err
	}
	if err := validatePasswordParams(params); err != nil {
		return "", err
	}

	salt := make([]byte, params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, params.Iterations, params.Memory, params.Parallelism, params.KeyLength)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2Version,
		params.Memory,
		params.Iterations,
		params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func VerifyPassword(password, encodedHash string) (bool, error) {
	params, salt, expectedKey, err := parsePasswordHash(encodedHash)
	if err != nil {
		return false, err
	}
	if !passwordUsableForVerify(password) {
		return false, nil
	}

	key := argon2.IDKey([]byte(password), salt, params.Iterations, params.Memory, params.Parallelism, uint32(len(expectedKey)))
	return subtle.ConstantTimeCompare(key, expectedKey) == 1, nil
}

func ValidatePasswordHash(encodedHash string) error {
	_, _, _, err := parsePasswordHash(encodedHash)
	return err
}

func parsePasswordHash(encodedHash string) (PasswordParams, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return PasswordParams{}, nil, nil, ErrInvalidHash
	}
	if parts[2] != fmt.Sprintf("v=%d", argon2Version) {
		return PasswordParams{}, nil, nil, ErrInvalidHash
	}

	paramParts := strings.Split(parts[3], ",")
	if len(paramParts) != 3 {
		return PasswordParams{}, nil, nil, ErrInvalidHash
	}
	values := make(map[string]uint64, 3)
	for _, part := range paramParts {
		key, value, ok := strings.Cut(part, "=")
		if !ok || key == "" || value == "" {
			return PasswordParams{}, nil, nil, ErrInvalidHash
		}
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return PasswordParams{}, nil, nil, ErrInvalidHash
		}
		if _, exists := values[key]; exists {
			return PasswordParams{}, nil, nil, ErrInvalidHash
		}
		values[key] = parsed
	}
	memory, ok := values["m"]
	if !ok {
		return PasswordParams{}, nil, nil, ErrInvalidHash
	}
	iterations, ok := values["t"]
	if !ok {
		return PasswordParams{}, nil, nil, ErrInvalidHash
	}
	parallelism, ok := values["p"]
	if !ok || parallelism > 255 {
		return PasswordParams{}, nil, nil, ErrInvalidHash
	}

	params := PasswordParams{
		Memory:      uint32(memory),
		Iterations:  uint32(iterations),
		Parallelism: uint8(parallelism),
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return PasswordParams{}, nil, nil, ErrInvalidHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return PasswordParams{}, nil, nil, ErrInvalidHash
	}
	params.SaltLength = uint32(len(salt))
	params.KeyLength = uint32(len(key))

	if err := validatePasswordParams(params); err != nil {
		return PasswordParams{}, nil, nil, ErrInvalidHash
	}
	return params, salt, key, nil
}

func validatePasswordParams(params PasswordParams) error {
	switch {
	case params.Memory < 8*1024 || params.Memory > 256*1024:
		return ErrInvalidHash
	case params.Iterations < 1 || params.Iterations > 10:
		return ErrInvalidHash
	case params.Parallelism < 1 || params.Parallelism > 16:
		return ErrInvalidHash
	case params.SaltLength < 16 || params.SaltLength > 64:
		return ErrInvalidHash
	case params.KeyLength < 16 || params.KeyLength > 64:
		return ErrInvalidHash
	default:
		return nil
	}
}

func passwordUsableForVerify(password string) bool {
	if password == "" || len(password) > maxPasswordLength {
		return false
	}
	for _, r := range password {
		if r == 0 || unicode.IsControl(r) {
			return false
		}
	}
	return true
}
