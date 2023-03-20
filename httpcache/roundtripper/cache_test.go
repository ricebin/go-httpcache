package roundtripper_test

import (
	"bytes"
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

	listener := &statsCounter{}

	fc := &fakeClock{
		now: time.Now(),
	}
	cache := &inmemoryCache{
		cache: make(map[string]*cachedResult),
		clock: fc,
	}
	cacheTransport := roundtripper.WrapWithClock(http.DefaultTransport, cache, fc.Now, roundtripper.ListenerOption(listener))

	hc := &http.Client{
		Transport: cacheTransport,
	}

	expiration := 1 * time.Hour

	assertResponse(t, hc, ts.URL+"/one", &expiration, http.StatusOK, []byte("GET:0:/one"))
	listener.asserStats(t, 0, 1)

	// increment clock
	fc.Add(30 * time.Minute)

	// read the same url, assert cache hit
	assertResponse(t, hc, ts.URL+"/one", &expiration, http.StatusOK, []byte("GET:0:/one"))
	listener.asserStats(t, 1, 1)

	// fresh /two
	assertResponse(t, hc, ts.URL+"/two", &expiration, http.StatusOK, []byte("GET:0:/two"))
	listener.asserStats(t, 1, 2)

	// increment clock
	fc.Add(45 * time.Minute)

	// one expired
	assertResponse(t, hc, ts.URL+"/one", &expiration, http.StatusOK, []byte("GET:1:/one"))
	listener.asserStats(t, 1, 3)

	// two hasnt expired
	assertResponse(t, hc, ts.URL+"/two", &expiration, http.StatusOK, []byte("GET:0:/two"))
	listener.asserStats(t, 2, 3)
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
	cacheTransport := roundtripper.WrapWithClock(http.DefaultTransport, cache, fc.Now)

	hc := &http.Client{
		Transport: cacheTransport,
	}

	expiration := 1 * time.Hour

	assertResponse(t, hc, ts.URL+"/one", &expiration, http.StatusBadRequest, []byte("GET:0:/one"))
	if (len(cache.cache)) != 0 {
		t.Errorf("should not cache anything")
	}

	nextStatusCode = http.StatusInternalServerError
	assertResponse(t, hc, ts.URL+"/one", &expiration, http.StatusInternalServerError, []byte("GET:1:/one"))
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
	cacheTransport := roundtripper.WrapWithClock(http.DefaultTransport, cache, fc.Now)

	hc := &http.Client{
		Transport: cacheTransport,
	}

	assertResponse(t, hc, ts.URL+"/one", nil, http.StatusBadRequest, []byte("GET:0:/one"))
	if (len(cache.cache)) != 0 {
		t.Errorf("should not cache anything")
	}
}

func TestWrap_ChangeExpiration(t *testing.T) {
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
	cacheTransport := roundtripper.WrapWithClock(http.DefaultTransport, cache, fc.Now)

	hc := &http.Client{
		Transport: cacheTransport,
	}

	oneHourExpiration := 1 * time.Hour
	assertResponse(t, hc, ts.URL+"/one", &oneHourExpiration, http.StatusOK, []byte("GET:0:/one"))

	fc.Add(30 * time.Minute)
	assertResponse(t, hc, ts.URL+"/one", &oneHourExpiration, http.StatusOK, []byte("GET:0:/one"))

	newExpiration := 10 * time.Minute
	assertResponse(t, hc, ts.URL+"/one", &newExpiration, http.StatusOK, []byte("GET:1:/one"))
}

func TestWrap_DefaultExpiration(t *testing.T) {
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

	oneHourExpiration := 1 * time.Hour
	cacheTransport := roundtripper.WrapWithClock(http.DefaultTransport, cache, fc.Now, roundtripper.DefaultExpirationOption(oneHourExpiration))

	hc := &http.Client{
		Transport: cacheTransport,
	}

	assertResponse(t, hc, ts.URL+"/one", nil, http.StatusOK, []byte("GET:0:/one"))

	fc.Add(30 * time.Minute)
	assertResponse(t, hc, ts.URL+"/one", nil, http.StatusOK, []byte("GET:0:/one"))

}

func TestWrap_Binary(t *testing.T) {
	payload := []byte{1, 2, 3, 4, 5}
	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter = counter + 1
		w.Write(payload)
	}))
	defer ts.Close()

	fc := &fakeClock{
		now: time.Now(),
	}
	cache := &inmemoryCache{
		cache: make(map[string]*cachedResult),
		clock: fc,
	}
	cacheTransport := roundtripper.WrapWithClock(http.DefaultTransport, cache, fc.Now)

	hc := &http.Client{
		Transport: cacheTransport,
	}

	oneHourExpiration := 1 * time.Hour
	assertResponse(t, hc, ts.URL+"/one", &oneHourExpiration, http.StatusOK, payload)

	fc.Add(30 * time.Minute)
	assertResponse(t, hc, ts.URL+"/one", &oneHourExpiration, http.StatusOK, payload)
	if counter != 1 {
		t.Errorf("expected counter to be 1")
	}
}

func assertResponse(t *testing.T, hc *http.Client, url string, expire *time.Duration, expectedCode int, expected []byte) {
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
		if !bytes.Equal(got, expected) {
			t.Errorf("expectect: %v but got: %v", expected, got)
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
	insertion  time.Time
	expiration time.Time
}

func (cr *cachedResult) Value() []byte {
	return cr.value
}

type inmemoryCache struct {
	cache map[string]*cachedResult
	clock Clock
}

func (c *inmemoryCache) Get(_ context.Context, url string) ([]byte, *time.Time, error) {
	now := time.Now()
	v, ok := c.cache[url]
	if ok && v.expiration.After(now) {
		return v.value, &v.insertion, nil
	}
	return nil, nil, nil
}

func (c *inmemoryCache) Set(_ context.Context, url string, rawResponse []byte, expiration time.Duration) error {
	now := c.clock.Now()
	c.cache[url] = &cachedResult{
		value:      rawResponse,
		insertion:  now,
		expiration: now.Add(expiration),
	}
	return nil
}

type statsCounter struct {
	hits   int
	misses int
}

func (s *statsCounter) Miss(_ *http.Request) {
	s.misses = s.misses + 1
}

func (s *statsCounter) Hit(_ *http.Request) {
	s.hits = s.hits + 1
}

func (s *statsCounter) asserStats(t *testing.T, expectedHits, expectedMisses int) {
	if expectedHits != s.hits {
		t.Errorf("expected %d hits but got: %d", expectedHits, s.hits)
	}
	if expectedMisses != s.misses {
		t.Errorf("expected %d misses but got: %d", expectedMisses, s.misses)
	}
}
