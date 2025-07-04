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

	"github.com/joho/godotenv"

	"github.com/google/uuid"
	"github.com/johnstontu/Chirpy/internal/auth"
	"github.com/johnstontu/Chirpy/internal/database"
	_ "github.com/lib/pq"
)

func handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(http.StatusText(http.StatusOK)))
}

func (cfg *apiConfig) numRequests(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "text/html")

	hits := cfg.fileserverHits.Load()

	htmlContent := fmt.Sprintf("<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", hits)

	w.Write([]byte(htmlContent))
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {

	if cfg.platform != "dev" {
		log.Printf("403 forbidden")
		w.WriteHeader(403)
		return
	}

	err := cfg.dbQueries.DeleteUsers(r.Context())
	if err != nil {
		log.Printf("Error deconding paramters: %s", err)
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	cfg.fileserverHits.Store(0)

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "Counter reset to 0")

}

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	platform       string
	secret         string
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)

		next.ServeHTTP(w, r)
	})
}

func jsonRequestHandler(w http.ResponseWriter, r *http.Request) {

	const maxChirpLength = 140
	bannedWords := [3]string{"kerfuffle", "sharbert", "fornax"}

	type parameters struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error deconding paramters: %s", err)
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	if len(params.Body) > maxChirpLength {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Chirp is too long"})
		return
	}

	bannedSet := make(map[string]struct{}, len(bannedWords))

	for _, b := range bannedWords {
		bannedSet[strings.ToLower(b)] = struct{}{}
	}

	// split the body into words (preserves original casing/punctuation in output)
	words := strings.Fields(params.Body)

	for i, w := range words {
		// compare lowercased
		if _, isBanned := bannedSet[strings.ToLower(w)]; isBanned {
			words[i] = "****"
		}
	}

	cleanBody := strings.Join(words, " ")

	type returnVals struct {
		Valid        bool   `json:"valid"`
		Cleaned_body string `json:"cleaned_body"`
	}
	respBody := returnVals{
		Valid:        true,
		Cleaned_body: cleanBody,
	}
	data, err := json.Marshal(respBody)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(data)

}

type User struct {
	ID             uuid.UUID `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Email          string    `json:"email"`
	HashedPassword string    `json:"hashed_password"`
	Token          string    `json:"token"`
	RefreshToken   string    `json:"refresh_token"`
}

func (cfg *apiConfig) handleUser(w http.ResponseWriter, r *http.Request) {

	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error deconding paramters: %s", err)
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	hashPwd, err := auth.HashPassword(params.Password)
	if err != nil {
		log.Printf("Error hashing password: %s", err)
		w.WriteHeader(500)
		return
	}

	user, err := cfg.dbQueries.CreateUser(r.Context(), database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashPwd,
	})
	if err != nil {
		log.Printf("Error running query: %s", err)
		w.WriteHeader(500)
		return
	}

	respBody := User{
		ID:             user.ID,
		CreatedAt:      user.CreatedAt,
		UpdatedAt:      user.UpdatedAt,
		Email:          user.Email,
		HashedPassword: user.HashedPassword,
	}
	data, err := json.Marshal(respBody)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(data)

}

func (cfg *apiConfig) handleChirps(w http.ResponseWriter, r *http.Request) {

	const maxChirpLength = 140
	bannedWords := [3]string{"kerfuffle", "sharbert", "fornax"}

	bearertoken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		http.Error(w, "missing or malformed Authorization header", http.StatusUnauthorized)
		return
	}
	userid, err := auth.ValidateJWT(bearertoken, cfg.secret)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	type parameters struct {
		Body string `json:"body"`
	}

	type chirpy struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		log.Printf("Error deconding paramters: %s", err)
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	if len(params.Body) > maxChirpLength {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Chirp is too long"})
		return
	}

	bannedSet := make(map[string]struct{}, len(bannedWords))

	for _, b := range bannedWords {
		bannedSet[strings.ToLower(b)] = struct{}{}
	}

	// split the body into words (preserves original casing/punctuation in output)
	words := strings.Fields(params.Body)

	for i, w := range words {
		// compare lowercased
		if _, isBanned := bannedSet[strings.ToLower(w)]; isBanned {
			words[i] = "****"
		}
	}

	cleanBody := strings.Join(words, " ")

	chirp, err := cfg.dbQueries.CreateChirp(
		r.Context(),
		database.CreateChirpParams{
			Body:   cleanBody,
			UserID: userid,
		})
	if err != nil {
		log.Printf("Error running query: %s", err)
		w.WriteHeader(500)
		return
	}

	respBody := chirpy{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}
	data, err := json.Marshal(respBody)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(data)

}

func (cfg *apiConfig) getChirps(w http.ResponseWriter, r *http.Request) {

	dbChirps, err := cfg.dbQueries.GetChirps(
		context.Background(),
	)
	if err != nil {
		log.Printf("Error running query: %s", err)
		w.WriteHeader(500)
		return
	}

	type chirpy struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}

	respBody := make([]chirpy, 0, len(dbChirps))
	for _, c := range dbChirps {
		respBody = append(respBody, chirpy{
			ID:        c.ID,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
			Body:      c.Body,
			UserID:    c.UserID,
		})
	}

	data, err := json.Marshal(respBody)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(data)

}

func (cfg *apiConfig) getChirpByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("chirpID")
	if id == "" {
		http.Error(w, "missing chirp ID", http.StatusBadRequest)
		return
	}

	chirpid, err := uuid.Parse(id)
	if err != nil {
		http.Error(w, "invalid chirp ID", http.StatusBadRequest)
		return
	}

	chirp, err := cfg.dbQueries.GetChirpByID(
		r.Context(),
		chirpid,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "chirp not found", http.StatusNotFound)
		} else {
			log.Printf("db error: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	type chirpy struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Body      string    `json:"body"`
		UserID    uuid.UUID `json:"user_id"`
	}

	resp := chirpy{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("encode error: %v", err)
	}

}

func (cfg *apiConfig) handleLogin(w http.ResponseWriter, r *http.Request) {

	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error deconding paramters: %s", err)
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	user, err := cfg.dbQueries.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		http.Error(w, "user does not exist", http.StatusBadRequest)
		return
	}

	user.Token, err = auth.MakeJWT(user.ID, cfg.secret, time.Duration(time.Hour*60))
	if err != nil {
		log.Printf("token issue: %s", err)
		w.WriteHeader(401)
		return
	}

	_, err = cfg.dbQueries.StoreUserToken(r.Context(), database.StoreUserTokenParams{
		Token: user.Token,
		Email: params.Email,
	})
	if err != nil {
		log.Printf("error updating access token: %s", err)
		w.WriteHeader(401)
		return
	}

	log.Printf("DEBUG: fetched hashed password = %q", user.HashedPassword)

	if user.HashedPassword == "unset" /* or == "unset" */ {
		http.Error(w, "credentials not set; please reset your password", http.StatusUnauthorized)
		return
	}

	err = auth.CheckPasswordHash(params.Password, user.HashedPassword)
	if err != nil {
		log.Printf("password mismatch: %s", err)
		w.WriteHeader(401)
		return
	}

	r_token, err := auth.MakeRefreshToken()
	if err != nil {
		log.Printf("error making new token: %s", err)
		w.WriteHeader(401)
		return
	}

	refresh_token, err := cfg.dbQueries.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:     r_token,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(time.Hour * 24 * 60),
	})
	if err != nil {
		log.Printf("unable to make refresh token: %s", err)
		w.WriteHeader(401)
		return
	}

	respBody := User{
		ID:           user.ID,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
		Email:        user.Email,
		Token:        user.Token,
		RefreshToken: refresh_token.Token,
	}

	data, err := json.Marshal(respBody)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(data)

}

func (cfg *apiConfig) handleRefresh(w http.ResponseWriter, r *http.Request) {

	bearertoken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		http.Error(w, "missing or malformed Authorization header", http.StatusUnauthorized)
		return
	}

	token, err := cfg.dbQueries.GetUserByRefreshToken(r.Context(), bearertoken)
	if err != nil {
		log.Printf("Error retrieving refresh token entry: %s", err)
		w.WriteHeader(401)
		return
	}
	if token.Token == "" {
		log.Printf("Returned a blank refresh token")
		w.WriteHeader(401)
		return
	}
	if token.ExpiresAt.Before(time.Now()) {
		log.Printf("token is expired")
		w.WriteHeader(401)
		return
	}
	if token.RevokedAt.Valid {
		log.Printf("token is expired")
		w.WriteHeader(401)
		return
	}

	type Response struct {
		Token string `json:"token"`
	}

	tokenSet, err := cfg.dbQueries.GetUserInfoByRefreshToken(r.Context(), token.Token)
	if err != nil {
		log.Printf("Error retrieving user token: %s", err)
		w.WriteHeader(401)
		return
	}
	respToken := tokenSet.UserToken.String
	log.Println(respToken)

	respBody := Response{
		Token: respToken,
	}

	data, err := json.Marshal(respBody)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(data)

}

func (cfg *apiConfig) handleRevoke(w http.ResponseWriter, r *http.Request) {

	bearertoken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		http.Error(w, "missing or malformed Authorization header", http.StatusUnauthorized)
		return
	}

	_, err = cfg.dbQueries.RevokeRefreshToken(r.Context(), bearertoken)
	if err != nil {
		log.Printf("Error updating refresh token: %s", err)
		w.WriteHeader(401)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(204)

}

func (cfg *apiConfig) handleUpdateUser(w http.ResponseWriter, r *http.Request) {

	bearertoken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		http.Error(w, "missing or malformed Authorization header", http.StatusUnauthorized)
		return
	}

	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		log.Printf("Error deconding paramters: %s", err)
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	hashPwd, err := auth.HashPassword(params.Password)
	if err != nil {
		log.Printf("Error hashing password: %s", err)
		w.WriteHeader(401)
		return
	}

	_, err = cfg.dbQueries.UpdateUserLogin(r.Context(), database.UpdateUserLoginParams{
		Email:          params.Email,
		HashedPassword: hashPwd,
		Token:          bearertoken,
	})
	if err != nil {
		log.Printf("Error updating user: %s", err)
		w.WriteHeader(401)
		return
	}

	user, err := cfg.dbQueries.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		log.Printf("Error running query: %s", err)
		w.WriteHeader(401)
		return
	}

	respBody := User{
		ID:             user.ID,
		CreatedAt:      user.CreatedAt,
		UpdatedAt:      user.UpdatedAt,
		Email:          user.Email,
		HashedPassword: user.HashedPassword,
	}
	data, err := json.Marshal(respBody)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(data)

}

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Printf("Error connecting to postgres: %s", err)
		return
	}
	dbQueries := database.New(db)

	const port = "8080"
	const filepathRoot = "."

	var cfg apiConfig
	cfg.dbQueries = dbQueries
	cfg.platform = os.Getenv("PLATFORM")
	cfg.secret = os.Getenv("SECRET")

	mux := http.NewServeMux()
	mux.Handle("/app/", cfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(filepathRoot)))))
	mux.HandleFunc("GET /api/healthz", handler)
	mux.HandleFunc("GET /admin/metrics", cfg.numRequests)
	mux.HandleFunc("POST /admin/reset", cfg.resetHandler)
	mux.HandleFunc("POST /api/users", cfg.handleUser)
	mux.HandleFunc("POST /api/chirps", cfg.handleChirps)
	mux.HandleFunc("GET /api/chirps", cfg.getChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", cfg.getChirpByID)
	mux.HandleFunc("POST /api/login", cfg.handleLogin)
	mux.HandleFunc("POST /api/refresh", cfg.handleRefresh)
	mux.HandleFunc("POST /api/revoke", cfg.handleRevoke)
	mux.HandleFunc("PUT /api/users", cfg.handleUpdateUser)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Printf("Serving on port: %s\n", port)
	log.Fatal(srv.ListenAndServe())
}
