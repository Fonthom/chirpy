package database

import (
	"testing"

	"github.com/google/uuid"
)

func TestDatabaseQueries(t *testing.T) {
	t.Run("Structs exist", func(t *testing.T) {
		var _ User
		var _ Chirp
		var _ CreateUserParams
		var _ CreateChirpParams
	})

	t.Run("Query methods exist", func(t *testing.T) {
		var q *Queries

		_ = q.CreateUser
		_ = q.GetUserByEmail
		_ = q.CreateChirp
		_ = q.GetAllChirps
		_ = q.GetChirpByID
		_ = q.DeleteAllUsers
	})
}

func TestCreateUserParams(t *testing.T) {
	params := CreateUserParams{
		Email:          "test@example.com",
		HashedPassword: "$argon2id$...",
	}

	if params.Email == "" {
		t.Error("Email should be set")
	}
}

func TestCreateChirpParams(t *testing.T) {
	userID := uuid.New()

	params := CreateChirpParams{
		Body:   "Hello, this is a test chirp!",
		UserID: userID,
	}

	if params.Body == "" {
		t.Error("Body should not be empty")
	}
}