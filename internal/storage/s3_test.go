//go:build integration

package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/skillhub/skillhub/internal/config"
)

func s3CfgForTest() config.S3Storage {
	return config.S3Storage{
		Endpoint:  os.Getenv("SKILLHUB_S3_ENDPOINT"),
		Bucket:    "skillhub",
		Region:    "us-east-1",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		UseSSL:    false,
	}
}

func TestS3_PutGetDelete(t *testing.T) {
	s, err := NewS3(s3CfgForTest())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	body := []byte("hello s3")
	if _, err := s.Put(ctx, "test/s3.txt", bytes.NewReader(body), int64(len(body)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	rc, err := s.Get(ctx, "test/s3.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, body) {
		t.Fatalf("got %q", got)
	}
	if err := s.Delete(ctx, "test/s3.txt"); err != nil {
		t.Fatal(err)
	}
}
