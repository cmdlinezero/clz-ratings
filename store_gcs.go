package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
)

const gcsVoteMaxAttempts = 8

type GCSStore struct {
	client *storage.Client
	bucket *storage.BucketHandle
	prefix string
}

func NewGCSStore(ctx context.Context, storageURI string) (*GCSStore, error) {
	bucket, prefix, err := parseGCSURI(storageURI)
	if err != nil {
		return nil, err
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}

	return &GCSStore{
		client: client,
		bucket: client.Bucket(bucket),
		prefix: prefix,
	}, nil
}

func (s *GCSStore) Close() error {
	return s.client.Close()
}

func (s *GCSStore) Get(ctx context.Context, id string) (RatingFile, error) {
	rating, _, exists, err := s.read(ctx, id)
	if err != nil {
		return RatingFile{}, err
	}
	if !exists {
		return RatingFile{}, nil
	}
	return rating, nil
}

func (s *GCSStore) Vote(ctx context.Context, id string, stars int) (RatingFile, error) {
	if stars < 1 || stars > 5 {
		return RatingFile{}, errors.New("stars out of range")
	}

	for attempt := 0; attempt < gcsVoteMaxAttempts; attempt++ {
		rating, generation, exists, err := s.read(ctx, id)
		if err != nil {
			return RatingFile{}, err
		}
		rating.Votes[stars-1]++

		obj := s.bucket.Object(s.objectName(id))
		if exists {
			obj = obj.If(storage.Conditions{GenerationMatch: generation})
		} else {
			obj = obj.If(storage.Conditions{DoesNotExist: true})
		}

		w := obj.NewWriter(ctx)
		w.ContentType = "application/json"
		w.CacheControl = "no-store"

		if err := json.NewEncoder(w).Encode(rating); err != nil {
			_ = w.Close()
			return RatingFile{}, fmt.Errorf("encode rating object: %w", err)
		}
		if err := w.Close(); err != nil {
			if isPreconditionFailed(err) {
				select {
				case <-ctx.Done():
					return RatingFile{}, ctx.Err()
				case <-time.After(time.Duration(attempt+1) * 10 * time.Millisecond):
					continue
				}
			}
			return RatingFile{}, fmt.Errorf("write rating object: %w", err)
		}

		return rating, nil
	}

	return RatingFile{}, fmt.Errorf("save vote: too much concurrent contention")
}

func (s *GCSStore) read(ctx context.Context, id string) (RatingFile, int64, bool, error) {
	r, err := s.bucket.Object(s.objectName(id)).NewReader(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return RatingFile{}, 0, false, nil
	}
	if err != nil {
		return RatingFile{}, 0, false, fmt.Errorf("open rating object: %w", err)
	}
	defer r.Close()

	raw, err := io.ReadAll(io.LimitReader(r, 64<<10))
	if err != nil {
		return RatingFile{}, 0, false, fmt.Errorf("read rating object: %w", err)
	}

	var rating RatingFile
	if err := json.Unmarshal(raw, &rating); err != nil {
		return RatingFile{}, 0, false, fmt.Errorf("decode rating object %q: %w", s.objectName(id), err)
	}
	return rating, r.Attrs.Generation, true, nil
}

func parseGCSURI(storageURI string) (bucket, prefix string, err error) {
	u, err := url.Parse(storageURI)
	if err != nil {
		return "", "", fmt.Errorf("parse storage URI: %w", err)
	}
	if u.Scheme != "gs" || u.Host == "" {
		return "", "", fmt.Errorf("storage URI must look like gs://bucket/prefix")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", "", fmt.Errorf("storage URI must not contain query or fragment")
	}
	prefix = strings.Trim(u.Path, "/")
	return u.Host, prefix, nil
}

func (s *GCSStore) objectName(id string) string {
	name := id + ".json"
	if s.prefix == "" {
		return name
	}
	return path.Join(s.prefix, name)
}

func isPreconditionFailed(err error) bool {
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == http.StatusPreconditionFailed
}
