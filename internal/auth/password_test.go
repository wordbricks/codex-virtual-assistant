package auth

import "testing"

var testPasswordParams = PasswordParams{
	Memory:      8 * 1024,
	Iterations:  1,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   16,
}

func TestHashPasswordRoundTrip(t *testing.T) {
	t.Parallel()

	hash, err := HashPasswordWithParams("correct horse battery staple", testPasswordParams)
	if err != nil {
		t.Fatalf("HashPasswordWithParams() error = %v", err)
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if !ok {
		t.Fatal("VerifyPassword() = false, want true")
	}

	ok, err = VerifyPassword("wrong horse battery staple", hash)
	if err != nil {
		t.Fatalf("VerifyPassword(wrong) error = %v", err)
	}
	if ok {
		t.Fatal("VerifyPassword(wrong) = true, want false")
	}
}

func TestHashPasswordRejectsWeakPassword(t *testing.T) {
	t.Parallel()

	if _, err := HashPasswordWithParams("short", testPasswordParams); err == nil {
		t.Fatal("HashPasswordWithParams(short) error = nil, want error")
	}
}

func TestValidatePasswordHashRejectsInvalidHash(t *testing.T) {
	t.Parallel()

	if err := ValidatePasswordHash("$argon2id$v=19$m=1,t=1,p=1$bad$bad"); err == nil {
		t.Fatal("ValidatePasswordHash() error = nil, want error")
	}
}
