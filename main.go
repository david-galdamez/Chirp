package main

import (
	"database/sql"
	"david-galdamez/chirp/internal/database"
	"log"
	"net/http"
	"os"
	"sync/atomic"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

var invalid_words = []string{"kerfuffle", "sharbert", "fornax"}

func main() {
	mux := http.NewServeMux()

	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env: %v", err)
	}

	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}

	dbQueries := database.New(db)

	secretKey := os.Getenv("SECRET_KEY")
	if secretKey == "" {
		log.Fatalf("Secret key not found")
	}

	apiCfg := apiConfig{
		fileserverHits: atomic.Int32{},
		queries:        dbQueries,
		secretKey:      secretKey,
	}

	mux.Handle("/api/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))

	mux.HandleFunc("GET /admin/metrics", apiCfg.serveMetrics)
	mux.HandleFunc("POST /admin/reset", apiCfg.resetMetric)

	mux.HandleFunc("POST /api/users", apiCfg.createUser)
	mux.HandleFunc("POST /api/login", apiCfg.loginUser)
	mux.HandleFunc("POST /api/refresh", apiCfg.refreshToken)
	mux.HandleFunc("POST /api/revoke", apiCfg.revokeToken)
	mux.HandleFunc("POST /api/chirps", apiCfg.createChirp)

	mux.HandleFunc("PUT /api/users", apiCfg.updateUser)

	mux.HandleFunc("GET /api/chirps", apiCfg.getChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.getChirp)

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/plain")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	err = server.ListenAndServe()
	if err != nil {
		log.Printf("Error listening to server: %v", err)
	}
}
