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
