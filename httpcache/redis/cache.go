package redis

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	version = 1
)

type Result struct {
	v []byte
}

func (r *Result) Value() []byte {
	return r.v
}

type Cache struct {
	c          *redis.Client
	urlKeyFunc func(string) string
	now        func() time.Time
}

func New(c *redis.Client) *Cache {
	return NewWithKeyFunc(c, func(url string) string { return url })
}

func NewWithKeyFunc(c *redis.Client, urlKeyFunc func(url string) string) *Cache {
	return NewWithClock(c, urlKeyFunc, time.Now)
}

func NewWithClock(c *redis.Client, urlKeyFunc func(string) string, now func() time.Time) *Cache {
	return &Cache{c: c, urlKeyFunc: urlKeyFunc, now: now}
}

func (c *Cache) Get(ctx context.Context, url string) ([]byte, *time.Time, error) {
	key := c.urlKeyFunc(url)
	b, err := c.c.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil, nil
	} else if err != nil {
		return nil, nil, err
	} else {
		cachedVersion, bytesRead := binary.Uvarint(b)
		if bytesRead == 0 {
			return nil, nil, fmt.Errorf("unexpected format. unable to read version")
		}
		if version != cachedVersion {
			return nil, nil, nil
		}

		b = b[bytesRead:]
		insertionUnixSecs, bytesRead := binary.Uvarint(b)

		b = b[bytesRead:]
		insertion := time.Unix(int64(insertionUnixSecs), 0)
		return b, &insertion, nil
	}
}

func (c *Cache) Set(ctx context.Context, url string, rawResponse []byte, expiration time.Duration) error {
	// uvarint: version
	// uvarint: insertion time in unix sec
	// payload
	v := make([]byte, 0, binary.MaxVarintLen64+binary.MaxVarintLen64+len(rawResponse))
	v = binary.AppendUvarint(v, version)
	v = binary.AppendUvarint(v, uint64(c.now().Unix()))
	v = append(v, rawResponse...)

	key := c.urlKeyFunc(url)
	result, err := c.c.Set(ctx, key, v, expiration).Result()
	if err != nil {
		return err
	} else if result != "OK" {
		return fmt.Errorf("redis.Set failed: %s", result)
	} else {
		return nil
	}
}
