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

	"github.com/Fonthom/chirpy/internal/auth"
	"github.com/Fonthom/chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
	jwtSecret      string
	polkaKey       string
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
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Email       string    `json:"email"`
	IsChirpyRed bool      `json:"is_chirpy_red"`
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

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("error opening database: %v", err)
	}

	polkaKey := os.Getenv("POLKA_KEY")
	if polkaKey == "" {
		log.Fatal("POLKA_KEY environment variable is not set")
	}

	apiCfg := &apiConfig{
		db:        database.New(db),
		platform:  os.Getenv("PLATFORM"),
		jwtSecret: jwtSecret,
		polkaKey:  polkaKey,
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

	// POST /api/users  — create user
	// PUT  /api/users  — update authenticated user's email and password
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var req struct {
				Email    string `json:"email"`
				Password string `json:"password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
				writeError(w, http.StatusBadRequest, "Invalid request body")
				return
			}

			hashedPassword, err := auth.HashPassword(req.Password)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Could not hash password")
				return
			}

			user, err := apiCfg.db.CreateUser(r.Context(), database.CreateUserParams{
				Email:          req.Email,
				HashedPassword: hashedPassword,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Could not create user")
				return
			}

			writeJSON(w, http.StatusCreated, userResponse{
				ID:          user.ID.String(),
				CreatedAt:   user.CreatedAt,
				UpdatedAt:   user.UpdatedAt,
				Email:       user.Email,
				IsChirpyRed: user.IsChirpyRed,
			})

		case http.MethodPut:
			// Require a valid access token
			token, err := auth.GetBearerToken(r.Header)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "Missing or malformed authorization header")
				return
			}
			userID, err := auth.ValidateJWT(token, apiCfg.jwtSecret)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "Invalid token")
				return
			}

			var req struct {
				Email    string `json:"email"`
				Password string `json:"password"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
				writeError(w, http.StatusBadRequest, "Invalid request body")
				return
			}

			hashedPassword, err := auth.HashPassword(req.Password)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Could not hash password")
				return
			}

			user, err := apiCfg.db.UpdateUser(r.Context(), database.UpdateUserParams{
				ID:             userID,
				Email:          req.Email,
				HashedPassword: hashedPassword,
			})
			if err != nil {
				log.Printf("UpdateUser error: %v", err)
				writeError(w, http.StatusInternalServerError, "Could not update user")
				return
			}

			writeJSON(w, http.StatusOK, userResponse{
				ID:          user.ID.String(),
				CreatedAt:   user.CreatedAt,
				UpdatedAt:   user.UpdatedAt,
				Email:       user.Email,
				IsChirpyRed: user.IsChirpyRed,
			})

		default:
			w.Header().Set("Allow", "POST, PUT")
			writeError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		}
	})

	// POST /api/login
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}

		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
			writeError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		user, err := apiCfg.db.GetUserByEmail(r.Context(), req.Email)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "Incorrect email or password")
			return
		}

		match, err := auth.CheckPasswordHash(req.Password, user.HashedPassword)
		if err != nil || !match {
			writeError(w, http.StatusUnauthorized, "Incorrect email or password")
			return
		}

		// Access token — always 1 hour
		accessToken, err := auth.MakeJWT(user.ID, apiCfg.jwtSecret, time.Hour)
		if err != nil {
			log.Printf("MakeJWT error: %v", err)
			writeError(w, http.StatusInternalServerError, "Could not create token")
			return
		}

		// Refresh token — 60 days, stored in DB
		rawRefresh, err := auth.MakeRefreshToken()
		if err != nil {
			log.Printf("MakeRefreshToken error: %v", err)
			writeError(w, http.StatusInternalServerError, "Could not create refresh token")
			return
		}
		_, err = apiCfg.db.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
			Token:     rawRefresh,
			UserID:    user.ID,
			ExpiresAt: time.Now().UTC().Add(60 * 24 * time.Hour),
		})
		if err != nil {
			log.Printf("CreateRefreshToken error: %v", err)
			writeError(w, http.StatusInternalServerError, "Could not save refresh token")
			return
		}

		type loginResponse struct {
			userResponse
			Token        string `json:"token"`
			RefreshToken string `json:"refresh_token"`
		}

		writeJSON(w, http.StatusOK, loginResponse{
			userResponse: userResponse{
				ID:          user.ID.String(),
				CreatedAt:   user.CreatedAt,
				UpdatedAt:   user.UpdatedAt,
				Email:       user.Email,
				IsChirpyRed: user.IsChirpyRed,
			},
			Token:        accessToken,
			RefreshToken: rawRefresh,
		})
	})

	// POST /api/refresh — exchange a valid refresh token for a new 1-hour access token
	mux.HandleFunc("/api/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}

		refreshToken, err := auth.GetBearerToken(r.Header)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "Missing or malformed token")
			return
		}

		// SQL query checks expiry and revocation — returns ErrNoRows if invalid
		user, err := apiCfg.db.GetUserFromRefreshToken(r.Context(), refreshToken)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "Invalid or expired refresh token")
			return
		}

		accessToken, err := auth.MakeJWT(user.ID, apiCfg.jwtSecret, time.Hour)
		if err != nil {
			log.Printf("MakeJWT error: %v", err)
			writeError(w, http.StatusInternalServerError, "Could not create token")
			return
		}

		writeJSON(w, http.StatusOK, struct {
			Token string `json:"token"`
		}{Token: accessToken})
	})

	// POST /api/revoke — revoke a refresh token (sets revoked_at, returns 204)
	mux.HandleFunc("/api/revoke", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}

		refreshToken, err := auth.GetBearerToken(r.Header)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "Missing or malformed token")
			return
		}

		if err := apiCfg.db.RevokeRefreshToken(r.Context(), refreshToken); err != nil {
			log.Printf("RevokeRefreshToken error: %v", err)
			writeError(w, http.StatusInternalServerError, "Could not revoke token")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	// POST /api/chirps and GET /api/chirps
	mux.HandleFunc("/api/chirps", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			token, err := auth.GetBearerToken(r.Header)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "Invalid or missing authorization header")
				return
			}

			userID, err := auth.ValidateJWT(token, apiCfg.jwtSecret)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "Invalid token")
				return
			}

			var req struct {
				Body string `json:"body"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "Invalid JSON")
				return
			}
			if len(req.Body) > 140 {
				writeError(w, http.StatusBadRequest, "Chirp is too long")
				return
			}

			chirp, err := apiCfg.db.CreateChirp(r.Context(), database.CreateChirpParams{
				Body:   cleanChirp(req.Body),
				UserID: userID,
			})
			if err != nil {
				log.Printf("CreateChirp error: %v", err)
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
			var chirps []database.Chirp

			if authorIDStr := r.URL.Query().Get("author_id"); authorIDStr != "" {
				authorID, err := uuid.Parse(authorIDStr)
				if err != nil {
					writeError(w, http.StatusBadRequest, "Invalid author_id")
					return
				}
				chirps, err = apiCfg.db.GetChirpsByAuthor(r.Context(), authorID)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "Could not fetch chirps")
					return
				}
			} else {
				chirps, err = apiCfg.db.GetAllChirps(r.Context())
				if err != nil {
					writeError(w, http.StatusInternalServerError, "Could not fetch chirps")
					return
				}
			}

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
	// DELETE /api/chirps/{chirpID}
	mux.HandleFunc("/api/chirps/{chirpID}", func(w http.ResponseWriter, r *http.Request) {
		chirpID, err := uuid.Parse(r.PathValue("chirpID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid chirp ID")
			return
		}

		switch r.Method {
		case http.MethodGet:
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

		case http.MethodDelete:
			// Authenticate
			token, err := auth.GetBearerToken(r.Header)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "Missing or malformed authorization header")
				return
			}
			userID, err := auth.ValidateJWT(token, apiCfg.jwtSecret)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "Invalid token")
				return
			}

			// Fetch chirp to verify ownership
			chirp, err := apiCfg.db.GetChirpByID(r.Context(), chirpID)
			if err != nil {
				if err == sql.ErrNoRows {
					writeError(w, http.StatusNotFound, "Chirp not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "Could not fetch chirp")
				return
			}

			// Only the author may delete their chirp
			if chirp.UserID != userID {
				writeError(w, http.StatusForbidden, "You are not the author of this chirp")
				return
			}

			if err := apiCfg.db.DeleteChirp(r.Context(), chirpID); err != nil {
				log.Printf("DeleteChirp error: %v", err)
				writeError(w, http.StatusInternalServerError, "Could not delete chirp")
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			w.Header().Set("Allow", "GET, DELETE")
			writeError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		}
	})

	// POST /api/polka/webhooks
	mux.HandleFunc("/api/polka/webhooks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}

		// Verify Polka API key
		apiKey, err := auth.GetAPIKey(r.Header)
		if err != nil || apiKey != apiCfg.polkaKey {
			writeError(w, http.StatusUnauthorized, "Invalid or missing API key")
			return
		}

		var req struct {
			Event string `json:"event"`
			Data  struct {
				UserID string `json:"user_id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// Ignore all events except user.upgraded
		if req.Event != "user.upgraded" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		userID, err := uuid.Parse(req.Data.UserID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid user_id")
			return
		}

		_, err = apiCfg.db.UpgradeUserToChirpyRed(r.Context(), userID)
		if err != nil {
			if err == sql.ErrNoRows {
				writeError(w, http.StatusNotFound, "User not found")
				return
			}
			log.Printf("UpgradeUserToChirpyRed error: %v", err)
			writeError(w, http.StatusInternalServerError, "Could not upgrade user")
			return
		}

		w.WriteHeader(http.StatusNoContent)
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

	// POST /admin/reset — dev-only: wipe all users (cascades to chirps + tokens) and reset counter
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