package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/skillhub/skillhub/internal/config"
)

type ObjectInfo struct {
	Key          string
	Size         int64
	ContentType  string
	LastModified time.Time
}

type Store interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (location string, err error)
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Stat(ctx context.Context, key string) (ObjectInfo, error)
}

func New(cfg config.StorageConfig) (Store, error) {
	switch cfg.Driver {
	case "local":
		return NewLocal(cfg.Local.Root)
	case "s3":
		return NewS3(cfg.S3)
	default:
		return nil, fmt.Errorf("unknown storage driver: %s", cfg.Driver)
	}
}
