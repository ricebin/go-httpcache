package roundtripper

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httputil"
	"time"

	"golang.org/x/sync/singleflight"
)

type CachedRoundTripper struct {
	delegate http.RoundTripper
	cache    Cache

	now func() time.Time
	g   singleflight.Group
}

func Wrap(delegate http.RoundTripper, cache Cache) *CachedRoundTripper {
	return WrapWithClock(delegate, cache, time.Now)
}

func WrapWithClock(delegate http.RoundTripper, cache Cache, now func() time.Time) *CachedRoundTripper {
	return &CachedRoundTripper{
		delegate: delegate,
		cache:    cache,
		now:      now,
	}
}

func (c *CachedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// we only cache get requests for now
	if req.Method != http.MethodGet {
		return c.delegate.RoundTrip(req)
	}

	var expiration time.Duration
	if expireSecsHeader := req.Header.Get(CacheExpirationHeader); expireSecsHeader == "" {
		return c.delegate.RoundTrip(req)
	} else if d, err := time.ParseDuration(expireSecsHeader); err != nil {
		return nil, fmt.Errorf("invalid %s header value: %s", CacheExpirationHeader, expireSecsHeader)
	} else {
		expiration = d
	}

	urlKey := req.URL.String()
	ctx := req.Context()

	if cached, insertionTime, err := c.cache.Get(ctx, urlKey); err != nil {
		// TODO(ricebin): customize this
		return nil, err
	} else if cached != nil && insertionTime != nil && insertionTime.Add(expiration).After(c.now()) {
		return http.ReadResponse(bufio.NewReader(bytes.NewReader(cached)), req)
	}

	if resp, err, _ := c.g.Do(urlKey, func() (any, error) {
		if realResponse, fetchErr := c.delegate.RoundTrip(req); fetchErr != nil {
			return nil, fetchErr
		} else {
			// TODO(ricebin): make this configurable
			if realResponse.StatusCode != http.StatusOK {
				return realResponse, nil
			}

			// TODO(ricebin): should cache header
			// TODO(ricebin): expiration header
			rawResp, err := httputil.DumpResponse(realResponse, true)
			if err != nil {
				// TODO(ricebin): customize this
				return nil, err
			}
			if err := c.cache.Set(ctx, urlKey, rawResp, expiration); err != nil {
				// TODO(ricebin): customize this
				return nil, err
			}
			return realResponse, nil
		}
	}); err != nil {
		return nil, err
	} else if httpResp, ok := resp.(*http.Response); !ok {
		// this should never happen
		return nil, fmt.Errorf("unexpected response type")
	} else {

		return httpResp, nil
	}
}
