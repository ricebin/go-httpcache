package roundtripper_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ricebin/go-tools/httpcache/roundtripper"
)

func TestWrap_Happy(t *testing.T) {
	hitCounters := make(map[string]int)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter := hitCounters[r.URL.String()]
		hitCounters[r.URL.String()] = counter + 1
		fmt.Fprint(w, r.Method, ":", counter, ":", r.URL.String())
	}))
	defer ts.Close()

	fc := &fakeClock{
		now: time.Now(),
	}
	cache := &inmemoryCache{
		cache: make(map[string]*cachedResult),
		clock: fc,
	}
	cacheTransport := roundtripper.Wrap(http.DefaultTransport, cache, time.Hour)

	hc := &http.Client{
		Transport: cacheTransport,
	}

	assertResponseBody(t, hc, ts.URL+"/one", "GET:0:/one")

	// increment clock
	fc.Add(30 * time.Minute)

	// read the same url, assert cache hit
	assertResponseBody(t, hc, ts.URL+"/one", "GET:0:/one")
	assertResponseBody(t, hc, ts.URL+"/two", "GET:0:/two")

	// increment clock
	fc.Add(45 * time.Minute)

	// one expired
	assertResponseBody(t, hc, ts.URL+"/one", "GET:1:/one")
	// two hasnt expired
	assertResponseBody(t, hc, ts.URL+"/two", "GET:0:/two")
}

func assertResponseBody(t *testing.T, hc *http.Client, url string, expected string) {
	// read the same url
	if resp, err := hc.Get(url); err != nil {
		t.Fatal(err)
	} else {
		got, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != expected {
			t.Errorf("expectect: %v but got: %v", expected, string(got))
		}
	}
}

type Clock interface {
	Now() time.Time
}

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) Add(d time.Duration) {
	c.now = c.now.Add(d)
}

type cachedResult struct {
	value      []byte
	expiration time.Time
}

func (cr *cachedResult) Value() []byte {
	return cr.value
}

type inmemoryCache struct {
	cache map[string]*cachedResult
	clock Clock
}

func (c *inmemoryCache) Get(ctx context.Context, url string) (roundtripper.CachedResult, error) {
	v, ok := c.cache[url]
	if !ok {
		return nil, nil
	}
	if v.expiration.After(c.clock.Now()) {
		return v, nil
	}
	return nil, nil
}

func (c *inmemoryCache) Set(ctx context.Context, url string, rawResponse []byte, expiration time.Duration) error {
	c.cache[url] = &cachedResult{
		value:      rawResponse,
		expiration: c.clock.Now().Add(expiration),
	}
	return nil
}
