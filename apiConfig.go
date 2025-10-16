package main

import (
	"database/sql"
	"david-galdamez/chirp/internal/auth"
	"david-galdamez/chirp/internal/database"
	"david-galdamez/chirp/models"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	queries        *database.Queries
	secretKey      string
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
	platform := os.Getenv("PLATFORM")
	if platform != "dev" {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	err := cfg.queries.DeleteUsers(r.Context())
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Internal server error"`))
		return
	}

	w.WriteHeader(http.StatusOK)
}

type ChirpRequest struct {
	Body   string    `json:"body"`
	UserId uuid.UUID `json:"user_id"`
}

func (cfg *apiConfig) createChirp(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "unauthorized"}`))
		return
	}

	_, err = auth.ValidateJWT(token, cfg.secretKey)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "unauthorized"}`))
		return
	}

	request := ChirpRequest{}

	decoder := json.NewDecoder(r.Body)
	err = decoder.Decode(&request)
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

	chirpDb, err := cfg.queries.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   request.Body,
		UserID: request.UserId,
	})
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Error creating chirp"`))
		return
	}

	chirp := models.Chirp{
		ID:        chirpDb.ID,
		CreatedAt: chirpDb.CreatedAt,
		UpdatedAt: chirpDb.UpdatedAt,
		Body:      chirpDb.Body,
		UserId:    chirpDb.UserID,
	}

	data, err := json.Marshal(chirp)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Internal server error"`))
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

type UserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (cfg *apiConfig) createUser(w http.ResponseWriter, r *http.Request) {
	request := UserRequest{}

	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&request)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`"error" : "Bad request"`))
		return
	}

	hashedPassword, err := auth.HashPassword(request.Password)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Error hashing password"`))
		return
	}

	userDatabase, err := cfg.queries.CreateUser(r.Context(), database.CreateUserParams{
		Email:          request.Email,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Error creating user"`))
		return
	}

	user := models.User{
		ID:        userDatabase.ID,
		CreatedAt: userDatabase.CreatedAt,
		UpdatedAt: userDatabase.UpdatedAt,
		Email:     userDatabase.Email,
	}

	jsonResponse, err := json.Marshal(user)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Internal server error"`))
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(jsonResponse)
}

type UserLogin struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (cfg *apiConfig) loginUser(w http.ResponseWriter, r *http.Request) {
	request := UserLogin{}

	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&request)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`"error" : "Bad request"`))
		return
	}

	userDB, err := cfg.queries.GetUser(r.Context(), request.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`"error": "Chirp not found"`))
			return
		}

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`"error" : "Incorrect email or password"`))
		return
	}

	isPassword, err := auth.CheckPasswordHash(request.Password, userDB.HashedPassword)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Internal server error"`))
		return
	}

	if !isPassword {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`"error" : "Incorrect email or password"`))
		return
	}

	token, err := auth.MakeJWT(userDB.ID, cfg.secretKey, time.Hour*1)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Error generating auth token"`))
		return
	}

	refreshToken, err := auth.MakeRefreshToken()
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Error generating refresh token"`))
		return
	}

	refreshTokenDb, err := cfg.queries.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:     refreshToken,
		UserID:    userDB.ID,
		ExpiresAt: time.Now().Add(time.Hour * 24 * 60),
	})
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Error creating refresh token"`))
		return
	}

	user := models.User{
		ID:           userDB.ID,
		CreatedAt:    userDB.CreatedAt,
		UpdatedAt:    userDB.UpdatedAt,
		Email:        userDB.Email,
		Token:        token,
		RefreshToken: refreshTokenDb.Token,
	}

	jsonRes, err := json.Marshal(user)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Internal server error"`))
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonRes)
}

func (cfg *apiConfig) getChirps(w http.ResponseWriter, r *http.Request) {
	chirpsDB, err := cfg.queries.GetChirps(r.Context())
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Internal server error"`))
		return
	}

	chirps := []models.Chirp{}
	for _, chirp := range chirpsDB {
		newChirp := models.Chirp{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserId:    chirp.UserID,
		}

		chirps = append(chirps, newChirp)
	}

	jsonRes, err := json.Marshal(chirps)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Internal server error"`))
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonRes)
}

func (cfg *apiConfig) getChirp(w http.ResponseWriter, r *http.Request) {
	chirpId := r.PathValue("chirpID")
	if chirpId == "" {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`"error": "Id is not valid"`))
		return
	}

	parsedId := uuid.MustParse(chirpId)

	chirpDb, err := cfg.queries.GetChirp(r.Context(), parsedId)
	if err != nil {
		if errors.Is(sql.ErrNoRows, err) {
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`"error": "Chirp not found"`))
			return
		}

		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Internal server error"`))
		return
	}

	chirp := models.Chirp{
		ID:        chirpDb.ID,
		CreatedAt: chirpDb.CreatedAt,
		UpdatedAt: chirpDb.UpdatedAt,
		Body:      chirpDb.Body,
		UserId:    chirpDb.UserID,
	}

	jsonRes, err := json.Marshal(chirp)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Internal server error"`))
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonRes)
}

func (cfg *apiConfig) refreshToken(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "unauthorized"}`))
		return
	}

	refreshTokenDB, err := cfg.queries.GetRefreshToken(r.Context(), refreshToken)
	if err != nil || time.Now().After(refreshTokenDB.ExpiresAt) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`"error" : "Not authorized"`))
		return
	}

	newToken, err := auth.MakeJWT(refreshTokenDB.UserID, cfg.secretKey, time.Hour*1)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Internal server error"`))
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`"token" : "%v"`, newToken)))
}

func (cfg *apiConfig) revokeToken(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`"error" : "unauthorized"`))
		return
	}

	_, err = cfg.queries.RevokeToken(r.Context(), refreshToken)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Internal server error"`))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type ChangeUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserResponse struct {
	Id        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (cfg *apiConfig) updateUser(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`"error" : "unauthorized"`))
		return
	}

	userId, err := auth.ValidateJWT(token, cfg.secretKey)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "unauthorized"}`))
		return
	}

	userRequest := ChangeUserRequest{}

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(userRequest); err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`"error" : "Bad request"`))
		return
	}

	hashedPassword, err := auth.HashPassword(userRequest.Password)
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Internal server error"`))
		return
	}

	userDB, err := cfg.queries.UpdateUser(r.Context(), database.UpdateUserParams{
		Email:          userRequest.Email,
		HashedPassword: hashedPassword,
		ID:             userId,
	})
	if err != nil {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`"error" : "Internal server error"`))
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write()
}
