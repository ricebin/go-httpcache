package roundtripper

import (
	"context"
	"time"
)

type Cache interface {
	Get(ctx context.Context, url string) (response []byte, insertion *time.Time, err error)
	Set(ctx context.Context, url string, response []byte, expiration time.Duration) error
}

const CacheExpirationHeader = "x-httpclient-cache-expiration"

type Option func(*requestOption)

func DefaultExpirationOption(expiration time.Duration) Option {
	return func(opt *requestOption) {
		opt.expiration = &expiration
	}
}

type requestOption struct {
	expiration *time.Duration
}
