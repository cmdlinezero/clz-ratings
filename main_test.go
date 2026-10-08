package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRatingMath(t *testing.T) {
	r := RatingFile{Votes: [5]uint64{0, 0, 1, 1, 2}}
	if got := r.Count(); got != 4 {
		t.Fatalf("count=%d", got)
	}
	avg := r.Average()
	if avg == nil || *avg != 4.25 {
		t.Fatalf("average=%v", avg)
	}
}

func TestVoteAndGet(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	api := &API{store: store}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/ratings/{id}", api.getRating)
	mux.HandleFunc("POST /v1/ratings/{id}/vote", api.postVote)

	req := httptest.NewRequest(http.MethodPost, "/v1/ratings/post-1/vote", strings.NewReader(`{"rating":5}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST status=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/ratings/post-1", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status=%d", rr.Code)
	}

	var got ratingResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Count != 1 || got.Rating == nil || *got.Rating != 5 {
		t.Fatalf("got=%+v", got)
	}
}

func TestUnknownRating(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	r, err := store.Get(context.Background(), "new-post")
	if err != nil {
		t.Fatal(err)
	}
	if r.Count() != 0 || r.Average() != nil {
		t.Fatalf("unexpected rating: %+v", r)
	}
}

func TestGetRatingImageUsesConfiguredBaseURL(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	api := &API{store: store, imageBaseURL: "https://storage.googleapis.com/clz-certin/images"}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/ratings/{id}/image", api.getRatingImage)

	req := httptest.NewRequest(http.MethodGet, "/v1/ratings/bash/image", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET image status=%d body=%s", rr.Code, rr.Body.String())
	}

	var got imageResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "bash" || got.Image != "https://storage.googleapis.com/clz-certin/images/bash.svg" {
		t.Fatalf("got=%+v", got)
	}
}

func TestGetRatingImageFallsBackToAPISVG(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	api := &API{store: store}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/ratings/{id}/image", api.getRatingImage)

	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/v1/ratings/my-post/image", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	var got imageResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Image != "http://localhost:8080/v1/ratings/my-post.svg" {
		t.Fatalf("image=%q", got.Image)
	}
}

func TestParseGCSURI(t *testing.T) {
	bucket, prefix, err := parseGCSURI("gs://clz-certin/ratings")
	if err != nil {
		t.Fatal(err)
	}
	if bucket != "clz-certin" || prefix != "ratings" {
		t.Fatalf("bucket=%q prefix=%q", bucket, prefix)
	}
}

func TestGCSObjectName(t *testing.T) {
	s := &GCSStore{prefix: "ratings"}
	if got := s.objectName("my-post"); got != "ratings/my-post.json" {
		t.Fatalf("object name=%q", got)
	}
}

func TestNewStoreFile(t *testing.T) {
	store, closeStore, err := newStore(context.Background(), "file://"+t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()
	if _, ok := store.(*FileStore); !ok {
		t.Fatalf("store type=%T", store)
	}
}

func TestParseAllowedOrigins(t *testing.T) {
	got := parseAllowedOrigins(" http://localhost:* , https://example.com/ ,,http://127.0.0.1:* ")
	want := []string{"http://localhost:*", "https://example.com", "http://127.0.0.1:*"}
	if len(got) != len(want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d]=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestAllowedOrigins(t *testing.T) {
	allowed := parseAllowedOrigins(
		"http://localhost:*,http://127.0.0.1:*,https://example.com",
	)

	tests := []struct {
		origin string
		want   bool
	}{
		{"http://localhost:8080", true},
		{"http://localhost:8087", true},
		{"http://localhost:1313", true},
		{"http://127.0.0.1:8088", true},
		{"https://example.com", true},
		{"http://localhost", false},
		{"https://localhost:8080", false},
		{"http://localhost.evil.example:8080", false},
		{"https://www.example.com", false},
		{"https://evil.example", false},
	}

	for _, tc := range tests {
		t.Run(tc.origin, func(t *testing.T) {
			if got := isAllowedOrigin(tc.origin, allowed); got != tc.want {
				t.Fatalf("isAllowedOrigin(%q)=%v want=%v", tc.origin, got, tc.want)
			}
		})
	}
}

func TestCORSPreflightAllowsLocalhostWildcardPort(t *testing.T) {
	allowed := parseAllowedOrigins("http://localhost:*,https://example.com")
	handler := cors(allowed, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight should not reach wrapped handler")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/v1/ratings/post-1/vote", nil)
	req.Header.Set("Origin", "http://localhost:8087")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8087" {
		t.Fatalf("Access-Control-Allow-Origin=%q", got)
	}
}

func TestCORSPreflightRejectsUnconfiguredOrigin(t *testing.T) {
	allowed := parseAllowedOrigins("http://localhost:*,https://example.com")
	handler := cors(allowed, http.NotFoundHandler())

	req := httptest.NewRequest(http.MethodOptions, "/v1/ratings/post-1/vote", nil)
	req.Header.Set("Origin", "https://evil.example")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected Access-Control-Allow-Origin=%q", got)
	}
}
