package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Result struct {
	v []byte
}

func (r *Result) Value() []byte {
	return r.v
}

type Cache struct {
	c *redis.Client
}

func New(c *redis.Client) *Cache {
	return &Cache{c: c}
}

func (c *Cache) Get(ctx context.Context, url string) ([]byte, error) {
	b, err := c.c.Get(ctx, url).Bytes()
	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		return nil, err
	} else {
		return b, nil
	}
}

func (c *Cache) Set(ctx context.Context, url string, body []byte, expiration time.Duration) error {
	_, err := c.c.Set(ctx, url, body, expiration).Result()
	return err
}
