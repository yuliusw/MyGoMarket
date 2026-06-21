package utils

import "testing"

func TestHashPasswordAndCheckPasswordHash(t *testing.T) {
	password := "secret123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if hash == "" {
		t.Fatalf("HashPassword returned empty hash")
	}

	if !CheckPasswordHash(password, hash) {
		t.Fatalf("CheckPasswordHash should return true for correct password")
	}

	if CheckPasswordHash("wrongpassword", hash) {
		t.Fatalf("CheckPasswordHash should return false for incorrect password")
	}
}
