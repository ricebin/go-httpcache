package roundtripper

import (
	"context"
	"time"
)

type CachedResult interface {
	Value() []byte
}

type Cache interface {
	Get(ctx context.Context, url string) (CachedResult, error)
	Set(ctx context.Context, url string, body []byte, expiration time.Duration) error
}
