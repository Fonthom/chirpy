package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"strings"
)

type apiConfig struct {
	fileserverHits atomic.Int32
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

func main() {
	apiCfg := &apiConfig{}
	mux := http.NewServeMux()
 
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
 
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
 
	mux.HandleFunc("/api/validate_chirp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte(`{"error":"Method Not Allowed"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
 
		var req struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"Invalid JSON"}`))
			return
		}
		if len(req.Body) > 140 {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"Chirp is too long"}`))
			return
		}
 
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(struct {
			CleanedBody string `json:"cleaned_body"`
		}{CleanedBody: cleanChirp(req.Body)})
	})
 
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
 
	mux.HandleFunc("/admin/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Method Not Allowed"))
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