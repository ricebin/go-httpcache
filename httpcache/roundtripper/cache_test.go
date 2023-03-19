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
	cacheTransport := roundtripper.Wrap(http.DefaultTransport, cache)

	hc := &http.Client{
		Transport: cacheTransport,
	}

	expiration := 1 * time.Hour

	assertResponse(t, hc, ts.URL+"/one", &expiration, http.StatusOK, "GET:0:/one")

	// increment clock
	fc.Add(30 * time.Minute)

	// read the same url, assert cache hit
	assertResponse(t, hc, ts.URL+"/one", &expiration, http.StatusOK, "GET:0:/one")
	assertResponse(t, hc, ts.URL+"/two", &expiration, http.StatusOK, "GET:0:/two")

	// increment clock
	fc.Add(45 * time.Minute)

	// one expired
	assertResponse(t, hc, ts.URL+"/one", &expiration, http.StatusOK, "GET:1:/one")
	// two hasnt expired
	assertResponse(t, hc, ts.URL+"/two", &expiration, http.StatusOK, "GET:0:/two")
}

func TestWrap_DontCache_OnError(t *testing.T) {
	nextStatusCode := http.StatusBadRequest
	hitCounters := make(map[string]int)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter := hitCounters[r.URL.String()]
		hitCounters[r.URL.String()] = counter + 1
		w.WriteHeader(nextStatusCode)
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
	cacheTransport := roundtripper.Wrap(http.DefaultTransport, cache)

	hc := &http.Client{
		Transport: cacheTransport,
	}

	expiration := 1 * time.Hour

	assertResponse(t, hc, ts.URL+"/one", &expiration, http.StatusBadRequest, "GET:0:/one")
	if (len(cache.cache)) != 0 {
		t.Errorf("should not cache anything")
	}

	nextStatusCode = http.StatusInternalServerError
	assertResponse(t, hc, ts.URL+"/one", &expiration, http.StatusInternalServerError, "GET:1:/one")
	if (len(cache.cache)) != 0 {
		t.Errorf("should not cache anything")
	}
}

func TestWrap_DontCache_NoHeader(t *testing.T) {
	nextStatusCode := http.StatusBadRequest
	hitCounters := make(map[string]int)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter := hitCounters[r.URL.String()]
		hitCounters[r.URL.String()] = counter + 1
		w.WriteHeader(nextStatusCode)
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
	cacheTransport := roundtripper.Wrap(http.DefaultTransport, cache)

	hc := &http.Client{
		Transport: cacheTransport,
	}

	assertResponse(t, hc, ts.URL+"/one", nil, http.StatusBadRequest, "GET:0:/one")
	if (len(cache.cache)) != 0 {
		t.Errorf("should not cache anything")
	}
}

func assertResponse(t *testing.T, hc *http.Client, url string, expire *time.Duration, expectedCode int, expected string) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if expire != nil {
		req.Header.Add(roundtripper.CacheExpirationHeader, expire.String())
	}
	if resp, err := hc.Do(req); err != nil {
		t.Fatal(err)
	} else {
		if expectedCode != resp.StatusCode {
			t.Errorf("expectect: %v but got: %v", expectedCode, resp.StatusCode)
		}
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

func (c *inmemoryCache) Get(_ context.Context, url string) ([]byte, error) {
	v, ok := c.cache[url]
	if !ok {
		return nil, nil
	}
	if v.expiration.After(c.clock.Now()) {
		return v.value, nil
	}
	return nil, nil
}

func (c *inmemoryCache) Set(_ context.Context, url string, rawResponse []byte, expiration time.Duration) error {
	c.cache[url] = &cachedResult{
		value:      rawResponse,
		expiration: c.clock.Now().Add(expiration),
	}
	return nil
}
