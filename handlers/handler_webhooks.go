package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"

	"github.com/Fonthom/chirpy/internal/auth"
)

func (cfg *Config) HandlerPolkaWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		WriteError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}

	// Verify Polka API key
	apiKey, err := auth.GetAPIKey(r.Header)
	if err != nil || apiKey != cfg.polkaKey {
		WriteError(w, http.StatusUnauthorized, "Invalid or missing API key")
		return
	}

	var req struct {
		Event string `json:"event"`
		Data  struct {
			UserID string `json:"user_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Ignore all events except user.upgraded
	if req.Event != "user.upgraded" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	userID, err := uuid.Parse(req.Data.UserID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid user_id")
		return
	}

	_, err = cfg.db.UpgradeUserToChirpyRed(r.Context(), userID)
	if err != nil {
		if err == sql.ErrNoRows {
			WriteError(w, http.StatusNotFound, "User not found")
			return
		}
		log.Printf("UpgradeUserToChirpyRed error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Could not upgrade user")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}