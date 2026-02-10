package tests

import (
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splusq/resemble/internal/cache"
	"github.com/splusq/resemble/internal/proxy"
	"github.com/splusq/resemble/internal/testutil"
)

func TestTTL_WithinTTLServesFromCache(t *testing.T) {
	var callCount int32
	upstream := testutil.TestUpstreamFunc(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"data":"fresh"}`))
	})

	cacheDir := testutil.TempDir(t)
	store := cache.NewStore(cacheDir)

	// Long TTL - entries should stay fresh
	srv := proxy.NewServer(":0", upstream.URL, "auto", 1*time.Hour, store, cache.KeyOptions{})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	proxyURL := "http://" + srv.Addr()

	// First request - record
	http.Get(proxyURL + "/v1/data")

	// Second request - should use cache
	http.Get(proxyURL + "/v1/data")

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("upstream called %d times, want 1", callCount)
	}
}

func TestTTL_ExpiredInAutoModeRefreshes(t *testing.T) {
	var callCount int32
	upstream := testutil.TestUpstreamFunc(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"data":"fresh"}`))
	})

	cacheDir := testutil.TempDir(t)
	store := cache.NewStore(cacheDir)

	// Very short TTL
	srv := proxy.NewServer(":0", upstream.URL, "auto", 1*time.Millisecond, store, cache.KeyOptions{})
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	proxyURL := "http://" + srv.Addr()

	// First request - record
	http.Get(proxyURL + "/v1/data")

	// Wait for TTL to expire
	time.Sleep(10 * time.Millisecond)

	// Second request - should refresh from upstream
	http.Get(proxyURL + "/v1/data")

	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("upstream called %d times, want 2", callCount)
	}
}

func TestTTL_ReplayModeIgnoresTTL(t *testing.T) {
	var callCount int32
	upstream := testutil.TestUpstreamFunc(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"data":"fresh"}`))
	})

	cacheDir := testutil.TempDir(t)
	store := cache.NewStore(cacheDir)

	// Record first with auto mode
	autoSrv := proxy.NewServer(":0", upstream.URL, "auto", 1*time.Millisecond, store, cache.KeyOptions{})
	if err := autoSrv.Start(); err != nil {
		t.Fatal(err)
	}

	autoURL := "http://" + autoSrv.Addr()
	http.Get(autoURL + "/v1/data")
	autoSrv.Stop()

	// Wait for TTL to expire
	time.Sleep(10 * time.Millisecond)

	// Now start in replay mode - should serve stale cache
	replaySrv := proxy.NewServer(":0", upstream.URL, "replay", 1*time.Millisecond, store, cache.KeyOptions{})
	if err := replaySrv.Start(); err != nil {
		t.Fatal(err)
	}
	defer replaySrv.Stop()

	replayURL := "http://" + replaySrv.Addr()
	resp, _ := http.Get(replayURL + "/v1/data")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if string(body) != `{"data":"fresh"}` {
		t.Errorf("body = %q", body)
	}

	// Upstream should only have been called once (during auto mode recording)
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("upstream called %d times, want 1", callCount)
	}
}
