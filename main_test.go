package main

import (
	"testing"

	"github.com/Fonthom/chirpy/internal/auth"
	"github.com/google/uuid"
	"time"
)

func TestAuthFunctions(t *testing.T) {
	t.Run("JWT Create and Validate", func(t *testing.T) {
		userID := uuid.New()
		secret := "test-secret-key"

		token, err := auth.MakeJWT(userID, secret, time.Hour)
		if err != nil {
			t.Fatalf("MakeJWT failed: %v", err)
		}

		returnedID, err := auth.ValidateJWT(token, secret)
		if err != nil {
			t.Fatalf("ValidateJWT failed: %v", err)
		}

		if returnedID != userID {
			t.Errorf("Expected userID %v, got %v", userID, returnedID)
		}
	})

	t.Run("Expired JWT rejected", func(t *testing.T) {
		userID := uuid.New()
		secret := "test-secret"

		token, err := auth.MakeJWT(userID, secret, -time.Hour) // expired
		if err != nil {
			t.Fatal(err)
		}

		_, err = auth.ValidateJWT(token, secret)
		if err == nil {
			t.Error("Expected error for expired token")
		}
	})
}

func TestMainPackage(t *testing.T) {
	t.Log("Main package structure is ready for handler tests")
}