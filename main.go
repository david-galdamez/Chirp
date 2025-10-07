package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
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

func (cfg *apiConfig) serveMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`, cfg.fileserverHits.Load())))
}

func (cfg *apiConfig) resetMetric(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Store(0)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits reset to 0"))
}

type ValidateRequest struct {
	Body string `json:"body"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type OkResponse struct {
	CleanedBody string `json:"cleaned_body"`
}

var invalid_words = []string{"kerfuffle", "sharbert", "fornax"}

func (cfg *apiConfig) validateChirp(w http.ResponseWriter, r *http.Request) {
	request := ValidateRequest{}

	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&request)
	if err != nil {
		errRes := ErrorResponse{Error: "Something went wrong"}
		data, _ := json.Marshal(errRes)

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(data)
		return
	}

	if len(request.Body) > 140 {
		errRes := ErrorResponse{Error: "Chirp is too long"}
		data, _ := json.Marshal(errRes)
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(data)
		return
	}

	for _, word := range invalid_words {
		request.Body = strings.ReplaceAll(request.Body, strings.ToLower(word), "****")
		request.Body = strings.ReplaceAll(request.Body, strings.ToUpper(word), "****")
	}

	okRes := OkResponse{CleanedBody: request.Body}
	data, _ := json.Marshal(okRes)

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func main() {
	mux := http.NewServeMux()
	apiCfg := apiConfig{
		fileserverHits: atomic.Int32{},
	}

	mux.Handle("/api/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	mux.HandleFunc("GET /admin/metrics", apiCfg.serveMetrics)
	mux.HandleFunc("POST /admin/reset", apiCfg.resetMetric)
	mux.HandleFunc("POST /api/validate_chirp", apiCfg.validateChirp)
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/plain")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	err := server.ListenAndServe()
	if err != nil {
		log.Printf("Error listening to server: %v", err)
	}
}
