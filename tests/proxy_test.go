package tests

import (
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splusq/resemble/internal/cache"
	"github.com/splusq/resemble/internal/proxy"
	"github.com/splusq/resemble/internal/testutil"
)

func TestProxy_RecordAndReplay(t *testing.T) {
	upstream := testutil.TestUpstream(t, 200, `{"status":"ok"}`)
	cacheDir := testutil.TempDir(t)
	store := cache.NewStore(cacheDir)

	srv := proxy.NewServer(":0", upstream.URL, "auto", 24*time.Hour, store, cache.KeyOptions{})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	proxyURL := "http://" + srv.Addr()

	// First request - should record
	resp, err := http.Get(proxyURL + "/v1/test")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if string(body) != `{"status":"ok"}` {
		t.Errorf("body = %q", body)
	}

	// Second request - should hit cache
	resp2, err := http.Get(proxyURL + "/v1/test")
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if string(body2) != `{"status":"ok"}` {
		t.Errorf("cached body = %q", body2)
	}
	if resp2.Header.Get("X-Resemble-Cache") != "hit" {
		t.Error("expected X-Resemble-Cache: hit header")
	}
}

func TestProxy_ReplayAfterUpstreamDown(t *testing.T) {
	var callCount int32
	upstream := testutil.TestUpstreamFunc(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"data":"cached"}`))
	})

	cacheDir := testutil.TempDir(t)
	store := cache.NewStore(cacheDir)

	srv := proxy.NewServer(":0", upstream.URL, "auto", 24*time.Hour, store, cache.KeyOptions{})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	proxyURL := "http://" + srv.Addr()

	// First request - record
	resp, _ := http.Get(proxyURL + "/v1/data")
	io.ReadAll(resp.Body)
	resp.Body.Close()

	// Shut down upstream
	upstream.Close()

	// Second request - should replay from cache
	resp2, err := http.Get(proxyURL + "/v1/data")
	if err != nil {
		t.Fatal(err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if resp2.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp2.StatusCode)
	}
	if string(body2) != `{"data":"cached"}` {
		t.Errorf("body = %q", body2)
	}

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("upstream called %d times, want 1", callCount)
	}
}

func TestProxy_ReplayModeFailsOnMiss(t *testing.T) {
	upstream := testutil.TestUpstream(t, 200, `{"status":"ok"}`)
	cacheDir := testutil.TempDir(t)
	store := cache.NewStore(cacheDir)

	srv := proxy.NewServer(":0", upstream.URL, "replay", 24*time.Hour, store, cache.KeyOptions{})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	proxyURL := "http://" + srv.Addr()

	resp, err := http.Get(proxyURL + "/v1/uncached")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want %d (bad gateway for miss in replay)", resp.StatusCode, http.StatusBadGateway)
	}
}

func TestProxy_RecordModeAlwaysHitsUpstream(t *testing.T) {
	var callCount int32
	upstream := testutil.TestUpstreamFunc(t, func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprintf(w, `{"call":%d}`, count)
	})

	cacheDir := testutil.TempDir(t)
	store := cache.NewStore(cacheDir)

	srv := proxy.NewServer(":0", upstream.URL, "record", 24*time.Hour, store, cache.KeyOptions{})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	proxyURL := "http://" + srv.Addr()

	// Two identical requests should both hit upstream
	http.Get(proxyURL + "/v1/test")
	resp, _ := http.Get(proxyURL + "/v1/test")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("upstream called %d times, want 2", callCount)
	}
	if string(body) != `{"call":2}` {
		t.Errorf("body = %q, want latest response", body)
	}
}

func TestProxy_DifferentPathsDifferentCacheEntries(t *testing.T) {
	upstream := testutil.TestUpstreamFunc(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprintf(w, `{"path":"%s"}`, r.URL.Path)
	})

	cacheDir := testutil.TempDir(t)
	store := cache.NewStore(cacheDir)

	srv := proxy.NewServer(":0", upstream.URL, "auto", 24*time.Hour, store, cache.KeyOptions{})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	proxyURL := "http://" + srv.Addr()

	resp1, _ := http.Get(proxyURL + "/v1/users")
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()

	resp2, _ := http.Get(proxyURL + "/v1/posts")
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	if string(body1) != `{"path":"/v1/users"}` {
		t.Errorf("body1 = %q", body1)
	}
	if string(body2) != `{"path":"/v1/posts"}` {
		t.Errorf("body2 = %q", body2)
	}
}

func TestProxy_IgnoredHeadersDontAffectCache(t *testing.T) {
	var callCount int32
	upstream := testutil.TestUpstreamFunc(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	})

	cacheDir := testutil.TempDir(t)
	store := cache.NewStore(cacheDir)

	keyOpts := cache.KeyOptions{
		IgnoreHeaders: []string{"Authorization"},
	}

	srv := proxy.NewServer(":0", upstream.URL, "auto", 24*time.Hour, store, keyOpts)
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	proxyURL := "http://" + srv.Addr()

	// First request with token A
	req1, _ := http.NewRequest("GET", proxyURL+"/v1/me", nil)
	req1.Header.Set("Authorization", "Bearer token-A")
	http.DefaultClient.Do(req1)

	// Second request with token B - should hit cache
	req2, _ := http.NewRequest("GET", proxyURL+"/v1/me", nil)
	req2.Header.Set("Authorization", "Bearer token-B")
	http.DefaultClient.Do(req2)

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("upstream called %d times, want 1 (auth header should be ignored)", callCount)
	}
}
