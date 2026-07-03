package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocal_PutGetDelete(t *testing.T) {
	root := t.TempDir()
	s, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	body := []byte("hello skill")
	loc, err := s.Put(ctx, "a/b/skill.txt", bytes.NewReader(body), int64(len(body)), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if loc == "" {
		t.Fatal("empty location")
	}
	rc, err := s.Get(ctx, "a/b/skill.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, body) {
		t.Fatalf("got %q", got)
	}
	info, err := s.Stat(ctx, "a/b/skill.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != int64(len(body)) || info.ContentType != "text/plain" {
		t.Fatalf("info=%+v", info)
	}
	if err := s.Delete(ctx, "a/b/skill.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stat(ctx, "a/b/skill.txt"); !os.IsNotExist(err) {
		t.Fatalf("expected not-exist, got %v", err)
	}
}

func TestLocal_NewLocal_MissingRoot(t *testing.T) {
	if _, err := NewLocal(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected error for missing root")
	}
}
