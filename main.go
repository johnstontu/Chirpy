package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

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
	cfg.fileserverHits.Store(0)

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "Counter reset to 0")

}

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
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

func main() {

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

	mux := http.NewServeMux()
	mux.Handle("/app/", cfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(filepathRoot)))))
	mux.HandleFunc("GET /api/healthz", handler)
	mux.HandleFunc("GET /admin/metrics", cfg.numRequests)
	mux.HandleFunc("POST /admin/reset", cfg.resetHandler)
	mux.HandleFunc("POST /api/validate_chirp", jsonRequestHandler)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Printf("Serving on port: %s\n", port)
	log.Fatal(srv.ListenAndServe())
}
