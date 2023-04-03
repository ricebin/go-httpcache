package redis_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/ricebin/go-tools/httpcache/roundtripper"

	httpcache_redis "github.com/ricebin/go-tools/httpcache/redis"
)

var (
	defaultUrlKeyFunc = func(url string) string {
		return url
	}
)

func TestWrap_Happy(t *testing.T) {
	hitCounters := make(map[string]int)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter := hitCounters[r.URL.String()]
		hitCounters[r.URL.String()] = counter + 1
		fmt.Fprint(w, r.Method, ":", counter, ":", r.URL.String())
	}))
	defer ts.Close()

	clock := &fakeClock{
		now: time.Now(),
	}

	s := miniredis.RunT(t)
	defer s.Close()
	rc := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	defer rc.Close()

	cache := httpcache_redis.NewWithClock(rc, defaultUrlKeyFunc, clock.Now)
	cacheTransport := roundtripper.WrapWithClock(http.DefaultTransport, cache, clock.Now)

	hc := &http.Client{
		Transport: cacheTransport,
	}

	expiration := 1 * time.Hour
	assertResponseBody(t, hc, ts.URL+"/one", &expiration, []byte("GET:0:/one"))

	// increment clock
	s.FastForward(30 * time.Minute)
	clock.Add(30 * time.Minute)

	// read the same url, assert cache hit
	assertResponseBody(t, hc, ts.URL+"/one", &expiration, []byte("GET:0:/one"))
	assertResponseBody(t, hc, ts.URL+"/two", &expiration, []byte("GET:0:/two"))

	// increment clock
	s.FastForward(45 * time.Minute)
	clock.Add(45 * time.Minute)

	// one expired
	assertResponseBody(t, hc, ts.URL+"/one", &expiration, []byte("GET:1:/one"))
	// two hasnt expired
	assertResponseBody(t, hc, ts.URL+"/two", &expiration, []byte("GET:0:/two"))

	// query two with different expiration
	{
		newExpiration := 1 * time.Minute
		assertResponseBody(t, hc, ts.URL+"/two", &newExpiration, []byte("GET:1:/two"))
	}
}

func TestWrap_Binary(t *testing.T) {
	payload := []byte{1, 2, 3, 4, 5}
	counter := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter = counter + 1
		w.Write(payload)
	}))
	defer ts.Close()

	clock := &fakeClock{
		now: time.Now(),
	}

	s := miniredis.RunT(t)
	defer s.Close()
	rc := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	defer rc.Close()

	cache := httpcache_redis.NewWithClock(rc, defaultUrlKeyFunc, clock.Now)
	cacheTransport := roundtripper.WrapWithClock(http.DefaultTransport, cache, clock.Now)

	hc := &http.Client{
		Transport: cacheTransport,
	}

	oneHourExpiration := 1 * time.Hour
	assertResponseBody(t, hc, ts.URL+"/one", &oneHourExpiration, payload)

	clock.Add(30 * time.Minute)
	assertResponseBody(t, hc, ts.URL+"/one", &oneHourExpiration, payload)
	if counter != 1 {
		t.Errorf("expected counter to be 1")
	}
}

func assertResponseBody(t *testing.T, hc *http.Client, url string, expiration *time.Duration, expected []byte) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if expiration != nil {
		req.Header.Add(roundtripper.CacheExpirationHeader, expiration.String())
	}
	if resp, err := hc.Do(req); err != nil {
		t.Fatal(err)
	} else {
		got, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, expected) {
			t.Errorf("expectect: %v but got: %v", expected, string(got))
		}
	}
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
