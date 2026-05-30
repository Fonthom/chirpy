package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/Fonthom/chirpy/internal/auth"
	"github.com/Fonthom/chirpy/internal/database"
)

func (cfg *Config) HandlerUsersCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Could not hash password")
		return
	}

	user, err := cfg.db.CreateUser(r.Context(), database.CreateUserParams{
		Email:          req.Email,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Could not create user")
		return
	}

	WriteJSON(w, http.StatusCreated, UserResponse{
		ID:          user.ID.String(),
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
		Email:       user.Email,
		IsChirpyRed: user.IsChirpyRed,
	})
}

func (cfg *Config) HandlerUsersUpdate(w http.ResponseWriter, r *http.Request) {
	// Require a valid access token
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "Missing or malformed authorization header")
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "Invalid token")
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Could not hash password")
		return
	}

	user, err := cfg.db.UpdateUser(r.Context(), database.UpdateUserParams{
		ID:             userID,
		Email:          req.Email,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		log.Printf("UpdateUser error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Could not update user")
		return
	}

	WriteJSON(w, http.StatusOK, UserResponse{
		ID:          user.ID.String(),
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
		Email:       user.Email,
		IsChirpyRed: user.IsChirpyRed,
	})
}