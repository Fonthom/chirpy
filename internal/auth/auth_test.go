package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHashPassword(t *testing.T) {
	password := "supersecret123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if hash == "" {
		t.Fatal("expected hash to not be empty")
	}
}

func TestCheckPasswordHash(t *testing.T) {
	password := "mystrongpass456"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}

	// Correct password
	match, err := CheckPasswordHash(password, hash)
	if err != nil {
		t.Fatal(err)
	}
	if !match {
		t.Error("expected passwords to match")
	}

	// Wrong password
	match, err = CheckPasswordHash("wrongpassword", hash)
	if err != nil {
		t.Fatal(err)
	}
	if match {
		t.Error("expected passwords to not match")
	}
}

func TestMakeJWT(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret-key"
	expiresIn := time.Hour

	token, err := MakeJWT(userID, secret, expiresIn)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if token == "" {
		t.Fatal("expected token string to not be empty")
	}
}

func TestValidateJWT(t *testing.T) {
	userID := uuid.New()
	secret := "super-secret-123"

	// Create a valid token
	token, err := MakeJWT(userID, secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	// Validate it
	returnedID, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if returnedID != userID {
		t.Errorf("expected userID %v, got %v", userID, returnedID)
	}
}

func TestValidateJWT_Expired(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret"

	// Create an already expired token
	token, err := MakeJWT(userID, secret, -time.Hour) // negative duration = expired
	if err != nil {
		t.Fatal(err)
	}

	_, err = ValidateJWT(token, secret)
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestValidateJWT_WrongSecret(t *testing.T) {
	userID := uuid.New()
	secret := "correct-secret"
	wrongSecret := "wrong-secret"

	token, err := MakeJWT(userID, secret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ValidateJWT(token, wrongSecret)
	if err == nil {
		t.Error("expected error when using wrong secret")
	}
}