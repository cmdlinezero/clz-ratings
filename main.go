package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type RatingFile struct {
	Votes [5]uint64 `json:"votes"`
}

func (r RatingFile) Count() uint64 {
	var n uint64
	for _, v := range r.Votes {
		n += v
	}
	return n
}

func (r RatingFile) Average() *float64 {
	count := r.Count()
	if count == 0 {
		return nil
	}
	var total uint64
	for i, v := range r.Votes {
		total += uint64(i+1) * v
	}
	avg := float64(total) / float64(count)
	return &avg
}

type Store interface {
	Get(ctx context.Context, id string) (RatingFile, error)
	Vote(ctx context.Context, id string, stars int) (RatingFile, error)
}

type API struct {
	store        Store
	imageBaseURL string
}

type ratingResponse struct {
	ID     string   `json:"id"`
	Rating *float64 `json:"rating"`
	Count  uint64   `json:"count"`
	Max    int      `json:"max"`
}

type voteRequest struct {
	Rating int `json:"rating"`
}

type imageResponse struct {
	ID    string `json:"id"`
	Image string `json:"image"`
}

func main() {
	ctx := context.Background()
	storageURI := getenv("RATINGS_STORAGE_URI", "gs://clz-certin/ratings")
	store, closeStore, err := newStore(ctx, storageURI)
	if err != nil {
		log.Fatal(err)
	}
	defer closeStore()

	api := &API{
		store:        store,
		imageBaseURL: strings.TrimRight(os.Getenv("IMAGE_BASE_URL"), "/"),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /v1/ratings/{id}", api.getRatingRoute)
	mux.HandleFunc("GET /v1/ratings/{id}/image", api.getRatingImage)
	mux.HandleFunc("POST /v1/ratings/{id}/vote", api.postVote)

	handler := requestLog(mux)
	port := getenv("PORT", "8080")
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("rating API listening on :%s (storage: %s)", port, storageURI)
	log.Fatal(srv.ListenAndServe())
}

func newStore(ctx context.Context, storageURI string) (Store, func(), error) {
	if strings.HasPrefix(storageURI, "gs://") {
		store, err := NewGCSStore(ctx, storageURI)
		if err != nil {
			return nil, func() {}, err
		}
		return store, func() { _ = store.Close() }, nil
	}

	if strings.HasPrefix(storageURI, "file://") {
		dir := strings.TrimPrefix(storageURI, "file://")
		if dir == "" {
			return nil, func() {}, errors.New("file storage URI requires a directory")
		}
		store, err := NewFileStore(dir)
		return store, func() {}, err
	}

	return nil, func() {}, fmt.Errorf("unsupported RATINGS_STORAGE_URI %q: use gs://bucket/prefix or file://directory", storageURI)
}

func (a *API) getRatingRoute(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.PathValue("id"), ".svg") {
		a.getRatingSVG(w, r)
		return
	}
	a.getRating(w, r)
}

func (a *API) getRating(w http.ResponseWriter, r *http.Request) {
	id, ok := contentID(w, r)
	if !ok {
		return
	}

	rating, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "failed to load rating", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=300")
	_ = json.NewEncoder(w).Encode(toResponse(id, rating))
}

func (a *API) getRatingImage(w http.ResponseWriter, r *http.Request) {
	id, ok := contentID(w, r)
	if !ok {
		return
	}

	imageURL := a.imageBaseURL
	if imageURL == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded != "" {
			scheme = strings.TrimSpace(strings.Split(forwarded, ",")[0])
		}
		imageURL = fmt.Sprintf("%s://%s/v1/ratings", scheme, r.Host)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300, stale-while-revalidate=3600")
	_ = json.NewEncoder(w).Encode(imageResponse{
		ID:    id,
		Image: imageURL + "/" + id + ".svg",
	})
}

func (a *API) postVote(w http.ResponseWriter, r *http.Request) {
	id, ok := contentID(w, r)
	if !ok {
		return
	}

	var req voteRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "body must be JSON like {\"rating\":4}", http.StatusBadRequest)
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		http.Error(w, "rating must be between 1 and 5", http.StatusBadRequest)
		return
	}

	rating, err := a.store.Vote(r.Context(), id, req.Rating)
	if err != nil {
		http.Error(w, "failed to save vote", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(toResponse(id, rating))
}

func (a *API) getRatingSVG(w http.ResponseWriter, r *http.Request) {
	id, ok := contentID(w, r)
	if !ok {
		return
	}

	rating, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "failed to load rating", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=30, stale-while-revalidate=300")
	_, _ = fmt.Fprint(w, renderSVG(rating))
}

func toResponse(id string, r RatingFile) ratingResponse {
	return ratingResponse{ID: id, Rating: r.Average(), Count: r.Count(), Max: 5}
}

func contentID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	id = strings.TrimSuffix(id, ".svg")
	if id == "" || len(id) > 128 {
		http.Error(w, "invalid content id", http.StatusBadRequest)
		return "", false
	}
	for _, c := range id {
		if !(c == '-' || c == '_' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			http.Error(w, "content id may contain letters, numbers, dot, dash and underscore only", http.StatusBadRequest)
			return "", false
		}
	}
	return id, true
}

func renderSVG(r RatingFile) string {
	count := r.Count()
	avg := 0.0
	label := "No ratings"
	if a := r.Average(); a != nil {
		avg = *a
		label = fmt.Sprintf("%.1f out of 5 from %d ratings", avg, count)
	}

	fillWidth := avg / 5 * 90
	text := "No ratings"
	if count > 0 {
		text = fmt.Sprintf("%.1f · %d", avg, count)
	}

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="180" height="26" viewBox="0 0 180 26" role="img" aria-label="%s">
  <defs>
    <clipPath id="fill"><rect x="0" y="0" width="%.2f" height="26"/></clipPath>
  </defs>
  <g font-family="system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif">
    <text x="0" y="19" font-size="20" fill="#c7c7c7">★★★★★</text>
    <text x="0" y="19" font-size="20" fill="#f5b301" clip-path="url(#fill)">★★★★★</text>
    <text x="100" y="18" font-size="13" fill="currentColor">%s</text>
  </g>
</svg>`, html.EscapeString(label), fillWidth, html.EscapeString(text))
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

var _ = errors.New
var _ = strconv.Itoa
