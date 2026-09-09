package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type FileStore struct {
	dir string
	mu  sync.Mutex
}

func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &FileStore{dir: dir}, nil
}

func (s *FileStore) Get(_ context.Context, id string) (RatingFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(id)
}

func (s *FileStore) Vote(_ context.Context, id string, stars int) (RatingFile, error) {
	if stars < 1 || stars > 5 {
		return RatingFile{}, errors.New("stars out of range")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	r, err := s.read(id)
	if err != nil {
		return RatingFile{}, err
	}
	r.Votes[stars-1]++

	raw, err := json.Marshal(r)
	if err != nil {
		return RatingFile{}, err
	}

	tmp, err := os.CreateTemp(s.dir, id+"-*.tmp")
	if err != nil {
		return RatingFile{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return RatingFile{}, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return RatingFile{}, err
	}
	if err := tmp.Close(); err != nil {
		return RatingFile{}, err
	}
	if err := os.Rename(tmpName, s.path(id)); err != nil {
		return RatingFile{}, err
	}

	return r, nil
}

func (s *FileStore) read(id string) (RatingFile, error) {
	raw, err := os.ReadFile(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return RatingFile{}, nil
	}
	if err != nil {
		return RatingFile{}, err
	}

	var r RatingFile
	if err := json.Unmarshal(raw, &r); err != nil {
		return RatingFile{}, fmt.Errorf("decode %s: %w", id, err)
	}
	return r, nil
}

func (s *FileStore) path(id string) string {
	return filepath.Join(s.dir, id+".json")
}
