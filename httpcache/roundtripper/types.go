package roundtripper

import (
	"context"
	"net/http"
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

func ListenerOption(l EventListener) Option {
	return func(opt *requestOption) {
		opt.listeners = append(opt.listeners, l)
	}
}

type EventListener interface {
	Miss(req *http.Request)
	Hit(req *http.Request)
}

type requestOption struct {
	expiration *time.Duration
	listeners  []EventListener
}
