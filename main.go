package main

import (
	"net/http"

	"github.com/Fonthom/chirpy/handlers"
	
)

func main() {
	cfg := handlers.LoadConfig()
	mux := http.NewServeMux()

	// ── Middleware ─────────────────────────────────────────────────────────
	mux.Handle("/app/", cfg.MiddlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))

	// ── Health & Admin ─────────────────────────────────────────────────────
	mux.HandleFunc("/api/healthz", cfg.HandlerHealthz)
	mux.HandleFunc("/admin/metrics", cfg.HandlerAdminMetrics)
	mux.HandleFunc("/admin/reset", cfg.HandlerAdminReset)

	// ── Users ──────────────────────────────────────────────────────────────
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			cfg.HandlerUsersCreate(w, r)
		case http.MethodPut:
			cfg.HandlerUsersUpdate(w, r)
		default:
			w.Header().Set("Allow", "POST, PUT")
			handlers.WriteError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		}
	})

	// ── Auth ───────────────────────────────────────────────────────────────
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			handlers.WriteError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
			return
		}
		cfg.HandlerLogin(w, r)
	})
	mux.HandleFunc("/api/refresh", cfg.HandlerRefresh)
	mux.HandleFunc("/api/revoke", cfg.HandlerRevoke)

	// ── Chirps ─────────────────────────────────────────────────────────────
	mux.HandleFunc("/api/chirps", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			cfg.HandlerChirpsCreate(w, r)
		case http.MethodGet:
			cfg.HandlerChirpsGet(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			handlers.WriteError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		}
	})

	mux.HandleFunc("/api/chirps/{chirpID}", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			cfg.HandlerChirpsGetByID(w, r)
		case http.MethodDelete:
			cfg.HandlerChirpsDelete(w, r)
		default:
			w.Header().Set("Allow", "GET, DELETE")
			handlers.WriteError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		}
	})

	// ── Webhooks ───────────────────────────────────────────────────────────
	mux.HandleFunc("/api/polka/webhooks", cfg.HandlerPolkaWebhook)

	// ── Server ─────────────────────────────────────────────────────────────
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	server.ListenAndServe()
}