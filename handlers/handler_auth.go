package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/Fonthom/chirpy/internal/auth"
	"github.com/Fonthom/chirpy/internal/database"
)

func (cfg *Config) HandlerLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	user, err := cfg.db.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "Incorrect email or password")
		return
	}

	match, err := auth.CheckPasswordHash(req.Password, user.HashedPassword)
	if err != nil || !match {
		WriteError(w, http.StatusUnauthorized, "Incorrect email or password")
		return
	}

	// Access token — always 1 hour
	accessToken, err := auth.MakeJWT(user.ID, cfg.jwtSecret, time.Hour)
	if err != nil {
		log.Printf("MakeJWT error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Could not create token")
		return
	}

	// Refresh token — 60 days, stored in DB
	rawRefresh, err := auth.MakeRefreshToken()
	if err != nil {
		log.Printf("MakeRefreshToken error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Could not create refresh token")
		return
	}
	_, err = cfg.db.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:     rawRefresh,
		UserID:    user.ID,
		ExpiresAt: time.Now().UTC().Add(60 * 24 * time.Hour),
	})
	if err != nil {
		log.Printf("CreateRefreshToken error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Could not save refresh token")
		return
	}

	type loginResponse struct {
		UserResponse
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}

	WriteJSON(w, http.StatusOK, loginResponse{
		UserResponse: UserResponse{
			ID:          user.ID.String(),
			CreatedAt:   user.CreatedAt,
			UpdatedAt:   user.UpdatedAt,
			Email:       user.Email,
			IsChirpyRed: user.IsChirpyRed,
		},
		Token:        accessToken,
		RefreshToken: rawRefresh,
	})
}

func (cfg *Config) HandlerRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		WriteError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}

	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "Missing or malformed token")
		return
	}

	// GetUserFromRefreshToken checks expiry and revocation in SQL
	user, err := cfg.db.GetUserFromRefreshToken(r.Context(), refreshToken)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}

	accessToken, err := auth.MakeJWT(user.ID, cfg.jwtSecret, time.Hour)
	if err != nil {
		log.Printf("MakeJWT error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Could not create token")
		return
	}

	WriteJSON(w, http.StatusOK, struct {
		Token string `json:"token"`
	}{Token: accessToken})
}

func (cfg *Config) HandlerRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		WriteError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}

	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "Missing or malformed token")
		return
	}

	if err := cfg.db.RevokeRefreshToken(r.Context(), refreshToken); err != nil {
		log.Printf("RevokeRefreshToken error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Could not revoke token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}