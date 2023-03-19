package roundtripper

import (
	"context"
	"time"
)

type Cache interface {
	Get(ctx context.Context, url string) ([]byte, error)
	Set(ctx context.Context, url string, body []byte, expiration time.Duration) error
}

const CacheExpirationHeader = "x-httpclient-cache-expiration"
