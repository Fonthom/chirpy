package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/Fonthom/chirpy/internal/auth"
	"github.com/Fonthom/chirpy/internal/database"
)

func cleanChirp(body string) string {
	profane := map[string]bool{
		"kerfuffle": true,
		"sharbert":  true,
		"fornax":    true,
	}
	words := strings.Split(body, " ")
	for i, word := range words {
		if profane[strings.ToLower(word)] {
			words[i] = "****"
		}
	}
	return strings.Join(words, " ")
}

func (cfg *Config) HandlerChirpsCreate(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "Invalid or missing authorization header")
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		WriteError(w, http.StatusUnauthorized, "Invalid token")
		return
	}

	var req struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if len(req.Body) > 140 {
		WriteError(w, http.StatusBadRequest, "Chirp is too long")
		return
	}

	chirp, err := cfg.db.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   cleanChirp(req.Body),
		UserID: userID,
	})
	if err != nil {
		log.Printf("CreateChirp error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Could not create chirp")
		return
	}

	WriteJSON(w, http.StatusCreated, ChirpResponse{
		ID:        chirp.ID.String(),
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID.String(),
	})
}

func (cfg *Config) HandlerChirpsGet(w http.ResponseWriter, r *http.Request) {
	var chirps []database.Chirp
	var err error

	if authorIDStr := r.URL.Query().Get("author_id"); authorIDStr != "" {
		authorID, err := uuid.Parse(authorIDStr)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid author_id")
			return
		}
		chirps, err = cfg.db.GetChirpsByAuthor(r.Context(), authorID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "Could not fetch chirps")
			return
		}
	} else {
		chirps, err = cfg.db.GetAllChirps(r.Context())
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "Could not fetch chirps")
			return
		}
	}

	// Sort by created_at
	sortParam := r.URL.Query().Get("sort")
	if sortParam == "" || sortParam == "asc" {
		// Sort ascending (default)
		sort.Slice(chirps, func(i, j int) bool {
			return chirps[i].CreatedAt.Before(chirps[j].CreatedAt)
		})
	} else if sortParam == "desc" {
		// Sort descending
		sort.Slice(chirps, func(i, j int) bool {
			return chirps[i].CreatedAt.After(chirps[j].CreatedAt)
		})
	}

	resp := make([]ChirpResponse, len(chirps))
	for i, c := range chirps {
		resp[i] = ChirpResponse{
			ID:        c.ID.String(),
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
			Body:      c.Body,
			UserID:    c.UserID.String(),
		}
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (cfg *Config) HandlerChirpsGetByID(w http.ResponseWriter, r *http.Request) {
	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid chirp ID")
		return
	}

	chirp, err := cfg.db.GetChirpByID(r.Context(), chirpID)
	if err != nil {
		if err == sql.ErrNoRows {
			WriteError(w, http.StatusNotFound, "Chirp not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "Could not fetch chirp")
		return
	}

	WriteJSON(w, http.StatusOK, ChirpResponse{
		ID:        chirp.ID.String(),
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID.String(),
	})
}

func (cfg *Config) HandlerChirpsDelete(w http.ResponseWriter, r *http.Request) {
	// Authenticate
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

	chirpID, err := uuid.Parse(r.PathValue("chirpID"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid chirp ID")
		return
	}

	// Fetch chirp to verify ownership
	chirp, err := cfg.db.GetChirpByID(r.Context(), chirpID)
	if err != nil {
		if err == sql.ErrNoRows {
			WriteError(w, http.StatusNotFound, "Chirp not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "Could not fetch chirp")
		return
	}

	// Only the author may delete their chirp
	if chirp.UserID != userID {
		WriteError(w, http.StatusForbidden, "You are not the author of this chirp")
		return
	}

	if err := cfg.db.DeleteChirp(r.Context(), chirpID); err != nil {
		log.Printf("DeleteChirp error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Could not delete chirp")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}