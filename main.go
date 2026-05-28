package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"github.com/Fonthom/chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Hits: %d", cfg.fileserverHits.Load())
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Store(0)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits reset to 0"))
}

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

type userResponse struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type chirpResponse struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    string    `json:"user_id"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: msg})
}

func main() {
	godotenv.Load()

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		log.Fatal("DB_URL environment variable is not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("error opening database: %v", err)
	}

	apiCfg := &apiConfig{
		db:       database.New(db),
		platform: os.Getenv("PLATFORM"),
	}

	mux := http.NewServeMux()

	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))

	// GET /api/healthz
	mux.HandleFunc("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method Not Allowed"))
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// POST /api/users
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}

		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
			writeError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		user, err := apiCfg.db.CreateUser(r.Context(), req.Email)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Could not create user")
			return
		}

		writeJSON(w, http.StatusCreated, userResponse{
			ID:        user.ID.String(),
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
			Email:     user.Email,
		})
	})

		// POST /api/chirps and GET /api/chirps
	mux.HandleFunc("/api/chirps", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var req struct {
				Body   string `json:"body"`
				UserID string `json:"user_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "Invalid JSON")
				return
			}

			// Validate length
			if len(req.Body) > 140 {
				writeError(w, http.StatusBadRequest, "Chirp is too long")
				return
			}

			// Validate user_id is a valid UUID
			userID, err := uuid.Parse(req.UserID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "Invalid user_id")
				return
			}

			// Clean profane words then persist
			chirp, err := apiCfg.db.CreateChirp(r.Context(), database.CreateChirpParams{
				Body:   cleanChirp(req.Body),
				UserID: userID,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Could not create chirp")
				return
			}

			writeJSON(w, http.StatusCreated, chirpResponse{
				ID:        chirp.ID.String(),
				CreatedAt: chirp.CreatedAt,
				UpdatedAt: chirp.UpdatedAt,
				Body:      chirp.Body,
				UserID:    chirp.UserID.String(),
			})

		case http.MethodGet:
			chirps, err := apiCfg.db.GetAllChirps(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Could not fetch chirps")
				return
			}

			// Convert to response slice
			resp := make([]chirpResponse, len(chirps))
			for i, c := range chirps {
				resp[i] = chirpResponse{
					ID:        c.ID.String(),
					CreatedAt: c.CreatedAt,
					UpdatedAt: c.UpdatedAt,
					Body:      c.Body,
					UserID:    c.UserID.String(),
				}
			}

			writeJSON(w, http.StatusOK, resp)

		default:
			w.Header().Set("Allow", "GET, POST")
			writeError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		}
	})

	// GET /api/chirps/{chirpID}
	mux.HandleFunc("/api/chirps/{chirpID}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}

		chirpIDStr := r.PathValue("chirpID")
		if chirpIDStr == "" {
			writeError(w, http.StatusBadRequest, "Invalid chirp ID")
			return
		}

		chirpID, err := uuid.Parse(chirpIDStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid chirp ID")
			return
		}

		chirp, err := apiCfg.db.GetChirpByID(r.Context(), chirpID)
		if err != nil {
			if err == sql.ErrNoRows {
				writeError(w, http.StatusNotFound, "Chirp not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "Could not fetch chirp")
			return
		}

		writeJSON(w, http.StatusOK, chirpResponse{
			ID:        chirp.ID.String(),
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID.String(),
		})
	})

	// GET /admin/metrics
	mux.HandleFunc("/admin/metrics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method Not Allowed"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<html>\n  <body>\n    <h1>Welcome, Chirpy Admin</h1>\n    <p>Chirpy has been visited %d times!</p>\n  </body>\n</html>`, apiCfg.fileserverHits.Load())
	})

	// POST /admin/reset — dev-only: wipe all users (cascades to chirps) and reset hit counter
	mux.HandleFunc("/admin/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method Not Allowed"))
			return
		}
		if apiCfg.platform != "dev" {
			writeError(w, http.StatusForbidden, "Forbidden")
			return
		}
		if err := apiCfg.db.DeleteAllUsers(context.Background()); err != nil {
			writeError(w, http.StatusInternalServerError, "Could not delete users")
			return
		}
		apiCfg.handlerReset(w, r)
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	server.ListenAndServe()
}