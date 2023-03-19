package redis_test

import (
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

func TestWrap_Happy(t *testing.T) {
	hitCounters := make(map[string]int)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter := hitCounters[r.URL.String()]
		hitCounters[r.URL.String()] = counter + 1
		fmt.Fprint(w, r.Method, ":", counter, ":", r.URL.String())
	}))
	defer ts.Close()

	s := miniredis.RunT(t)
	defer s.Close()
	rc := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})
	defer rc.Close()

	cache := httpcache_redis.New(rc)
	cacheTransport := roundtripper.Wrap(http.DefaultTransport, cache)

	hc := &http.Client{
		Transport: cacheTransport,
	}

	expiration := 1 * time.Hour
	assertResponseBody(t, hc, ts.URL+"/one", &expiration, "GET:0:/one")

	// increment clock
	s.FastForward(30 * time.Minute)

	// read the same url, assert cache hit
	assertResponseBody(t, hc, ts.URL+"/one", &expiration, "GET:0:/one")
	assertResponseBody(t, hc, ts.URL+"/two", &expiration, "GET:0:/two")

	// increment clock
	s.FastForward(45 * time.Minute)

	// one expired
	assertResponseBody(t, hc, ts.URL+"/one", &expiration, "GET:1:/one")
	// two hasnt expired
	assertResponseBody(t, hc, ts.URL+"/two", &expiration, "GET:0:/two")
}

func assertResponseBody(t *testing.T, hc *http.Client, url string, expiration *time.Duration, expected string) {
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
		if string(got) != expected {
			t.Errorf("expectect: %v but got: %v", expected, string(got))
		}
	}
}
