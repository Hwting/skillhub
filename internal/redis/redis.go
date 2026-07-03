package redis

import (
	"context"
	"fmt"

	"github.com/skillhub/skillhub/internal/config"
	rdb "github.com/redis/go-redis/v9"
)

func New(cfg config.RedisConfig) (*rdb.Client, error) {
	c := rdb.NewClient(&rdb.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	return c, nil
}

func Ping(ctx context.Context, c *rdb.Client) error {
	if err := c.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}
