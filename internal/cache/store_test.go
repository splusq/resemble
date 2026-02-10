package cache

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir, err := os.MkdirTemp("", "resemble-store-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return NewStore(dir)
}

func sampleEntry() *Entry {
	return &Entry{
		Request: RequestMeta{
			Method:   "GET",
			Path:     "/v1/users",
			Query:    "page=1",
			BodyHash: "",
		},
		Response: ResponseMeta{
			Status:  200,
			Headers: map[string]string{"Content-Type": "application/json"},
		},
		TTL: "24h",
	}
}

func TestStore_PutAndGet(t *testing.T) {
	s := tempStore(t)
	entry := sampleEntry()
	body := []byte(`{"users":[]}`)

	if err := s.Put("api.example.com", "abc123", entry, body); err != nil {
		t.Fatal(err)
	}

	got, gotBody, err := s.Get("api.example.com", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected cache hit, got miss")
	}
	if got.Response.Status != 200 {
		t.Errorf("status = %d, want 200", got.Response.Status)
	}
	if string(gotBody) != `{"users":[]}` {
		t.Errorf("body = %q, want %q", gotBody, `{"users":[]}`)
	}
	if got.Key != "abc123" {
		t.Errorf("key = %q, want %q", got.Key, "abc123")
	}
	if got.Version != 1 {
		t.Errorf("version = %d, want 1", got.Version)
	}
	if got.Response.BodyFile != "abc123.body" {
		t.Errorf("body_file = %q, want %q", got.Response.BodyFile, "abc123.body")
	}
}

func TestStore_CacheMiss(t *testing.T) {
	s := tempStore(t)

	got, body, err := s.Get("api.example.com", "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil entry for cache miss")
	}
	if body != nil {
		t.Error("expected nil body for cache miss")
	}
}

func TestStore_Overwrite(t *testing.T) {
	s := tempStore(t)

	entry1 := sampleEntry()
	entry1.Response.Status = 200
	s.Put("api.example.com", "abc123", entry1, []byte("body1"))

	entry2 := sampleEntry()
	entry2.Response.Status = 201
	s.Put("api.example.com", "abc123", entry2, []byte("body2"))

	got, body, _ := s.Get("api.example.com", "abc123")
	if got.Response.Status != 201 {
		t.Errorf("status = %d, want 201", got.Response.Status)
	}
	if string(body) != "body2" {
		t.Errorf("body = %q, want %q", body, "body2")
	}
}

func TestStore_Delete(t *testing.T) {
	s := tempStore(t)

	s.Put("api.example.com", "abc123", sampleEntry(), []byte("body"))
	s.Delete("api.example.com", "abc123")

	got, _, _ := s.Get("api.example.com", "abc123")
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestStore_DeleteByPattern(t *testing.T) {
	s := tempStore(t)

	s.Put("api.example.com", "aaa111", sampleEntry(), []byte("body1"))
	s.Put("api.example.com", "aaa222", sampleEntry(), []byte("body2"))
	s.Put("api.example.com", "bbb333", sampleEntry(), []byte("body3"))

	count, err := s.DeleteByPattern("aaa*.meta.json")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("deleted %d, want 2", count)
	}

	got, _, _ := s.Get("api.example.com", "bbb333")
	if got == nil {
		t.Error("non-matching entry should not be deleted")
	}
}

func TestStore_ConcurrentReads(t *testing.T) {
	s := tempStore(t)
	s.Put("api.example.com", "abc123", sampleEntry(), []byte("body"))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, _, err := s.Get("api.example.com", "abc123")
			if err != nil {
				t.Errorf("concurrent read error: %v", err)
			}
			if got == nil {
				t.Error("concurrent read returned nil")
			}
		}()
	}
	wg.Wait()
}

func TestStore_CorruptMetaFile(t *testing.T) {
	s := tempStore(t)

	// Write a corrupt meta file
	dir := filepath.Join(s.Dir(), "cache", "api.example.com")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "corrupt.meta.json"), []byte("not json"), 0644)
	os.WriteFile(filepath.Join(dir, "corrupt.body"), []byte("body"), 0644)

	got, _, err := s.Get("api.example.com", "corrupt")
	if err != nil {
		t.Errorf("corrupt meta should not return error, got: %v", err)
	}
	if got != nil {
		t.Error("corrupt meta should be treated as miss")
	}
}

func TestStore_BinaryBodyRoundTrip(t *testing.T) {
	s := tempStore(t)

	// Binary data with null bytes, high bytes, etc.
	binaryBody := []byte{0x00, 0x01, 0xFF, 0xFE, 0x89, 0x50, 0x4E, 0x47}

	s.Put("api.example.com", "binary1", sampleEntry(), binaryBody)

	_, body, _ := s.Get("api.example.com", "binary1")
	if len(body) != len(binaryBody) {
		t.Fatalf("body length = %d, want %d", len(body), len(binaryBody))
	}
	for i, b := range body {
		if b != binaryBody[i] {
			t.Errorf("byte %d: got %x, want %x", i, b, binaryBody[i])
		}
	}
}

func TestStore_Stats(t *testing.T) {
	s := tempStore(t)

	s.Put("api.example.com", "abc123", sampleEntry(), []byte("body1"))
	s.Put("api.example.com", "def456", sampleEntry(), []byte("body2"))
	s.Put("auth.example.com", "ghi789", sampleEntry(), []byte("body3"))

	stats, err := s.Stats(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if stats.TotalEntries != 3 {
		t.Errorf("total entries = %d, want 3", stats.TotalEntries)
	}
	if stats.Hosts["api.example.com"] != 2 {
		t.Errorf("api.example.com entries = %d, want 2", stats.Hosts["api.example.com"])
	}
	if stats.Hosts["auth.example.com"] != 1 {
		t.Errorf("auth.example.com entries = %d, want 1", stats.Hosts["auth.example.com"])
	}
}

func TestStore_StatsEmpty(t *testing.T) {
	s := tempStore(t)

	stats, err := s.Stats(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalEntries != 0 {
		t.Errorf("total entries = %d, want 0", stats.TotalEntries)
	}
}

func TestStore_List(t *testing.T) {
	s := tempStore(t)

	s.Put("api.example.com", "abc123", sampleEntry(), []byte("body1"))
	s.Put("api.example.com", "def456", sampleEntry(), []byte("body2"))

	entries, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("list returned %d entries, want 2", len(entries))
	}
}

func TestEntry_IsStale(t *testing.T) {
	tests := []struct {
		name     string
		entry    Entry
		ttl      time.Duration
		expected bool
	}{
		{
			name:     "zero TTL never stale",
			entry:    Entry{LastRefreshed: time.Now().Add(-48 * time.Hour)},
			ttl:      0,
			expected: false,
		},
		{
			name:     "within TTL",
			entry:    Entry{LastRefreshed: time.Now().Add(-1 * time.Hour)},
			ttl:      24 * time.Hour,
			expected: false,
		},
		{
			name:     "past TTL",
			entry:    Entry{LastRefreshed: time.Now().Add(-48 * time.Hour)},
			ttl:      24 * time.Hour,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.entry.IsStale(tt.ttl)
			if got != tt.expected {
				t.Errorf("IsStale() = %v, want %v", got, tt.expected)
			}
		})
	}
}
