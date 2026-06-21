package utils

import (
	"testing"
	"time"
)

func TestGenerateAndParseToken(t *testing.T) {
	userID := "user-123"

	token, err := GenerateToken(userID)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	if token == "" {
		t.Fatalf("GenerateToken returned empty token")
	}

	claims, err := ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken returned error: %v", err)
	}

	if claims.UserID != userID {
		t.Fatalf("expected userID %s, got %s", userID, claims.UserID)
	}

	if claims.ExpiresAt == nil || claims.ExpiresAt.Time.Before(time.Now()) {
		t.Fatalf("token should not be expired")
	}
}

func TestParseToken_Invalid(t *testing.T) {
	_, err := ParseToken("invalid.token.value")
	if err == nil {
		t.Fatalf("expected error for invalid token, got nil")
	}
}
