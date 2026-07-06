package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type LocalStore struct {
	root string
}

func NewLocal(root string) (*LocalStore, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("abs root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	return &LocalStore{root: abs}, nil
}

func (s *LocalStore) path(key string) (string, error) {
	p := filepath.Join(s.root, filepath.FromSlash(key))
	rel, err := filepath.Rel(s.root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid key escapes root: %s", key)
	}
	return p, nil
}

func (s *LocalStore) metaPath(key string) (string, error) {
	p, err := s.path(key)
	if err != nil {
		return "", err
	}
	return p + ".meta", nil
}

func (s *LocalStore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error) {
	p, err := s.path(key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.Create(p)
	if err != nil {
		return "", fmt.Errorf("create: %w", err)
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return "", fmt.Errorf("write: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close: %w", err)
	}
	if contentType != "" {
		mp, _ := s.metaPath(key)
		if err := os.WriteFile(mp, []byte(contentType), 0o644); err != nil {
			return "", fmt.Errorf("write meta: %w", err)
		}
	}
	return p, nil
}

func (s *LocalStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *LocalStore) Delete(ctx context.Context, key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	mp, _ := s.metaPath(key)
	os.Remove(mp)
	return nil
}

func (s *LocalStore) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	p, err := s.path(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	fi, err := os.Stat(p)
	if err != nil {
		return ObjectInfo{}, err
	}
	info := ObjectInfo{
		Key:          key,
		Size:         fi.Size(),
		LastModified: fi.ModTime(),
	}
	if mp, _ := s.metaPath(key); mp != "" {
		if b, err := os.ReadFile(mp); err == nil {
			info.ContentType = string(b)
		}
	}
	return info, nil
}
