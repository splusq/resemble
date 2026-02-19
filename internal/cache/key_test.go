package cache

import (
	"net/http"
	"strings"
	"testing"
)

func TestGenerateKey_Deterministic(t *testing.T) {
	r1, _ := http.NewRequest("GET", "http://localhost/v1/users?page=1&limit=10", nil)
	r2, _ := http.NewRequest("GET", "http://localhost/v1/users?page=1&limit=10", nil)

	key1 := GenerateKey(r1, nil, KeyOptions{})
	key2 := GenerateKey(r2, nil, KeyOptions{})

	if key1 != key2 {
		t.Errorf("identical requests produced different keys: %s vs %s", key1, key2)
	}
}

func TestGenerateKey_DifferentMethodsDifferentKeys(t *testing.T) {
	r1, _ := http.NewRequest("GET", "http://localhost/v1/users", nil)
	r2, _ := http.NewRequest("POST", "http://localhost/v1/users", nil)

	key1 := GenerateKey(r1, nil, KeyOptions{})
	key2 := GenerateKey(r2, nil, KeyOptions{})

	if key1 == key2 {
		t.Error("different methods produced same key")
	}
}

func TestGenerateKey_DifferentPathsDifferentKeys(t *testing.T) {
	r1, _ := http.NewRequest("GET", "http://localhost/v1/users", nil)
	r2, _ := http.NewRequest("GET", "http://localhost/v1/posts", nil)

	key1 := GenerateKey(r1, nil, KeyOptions{})
	key2 := GenerateKey(r2, nil, KeyOptions{})

	if key1 == key2 {
		t.Error("different paths produced same key")
	}
}

func TestGenerateKey_DifferentQueriesDifferentKeys(t *testing.T) {
	r1, _ := http.NewRequest("GET", "http://localhost/v1/users?page=1", nil)
	r2, _ := http.NewRequest("GET", "http://localhost/v1/users?page=2", nil)

	key1 := GenerateKey(r1, nil, KeyOptions{})
	key2 := GenerateKey(r2, nil, KeyOptions{})

	if key1 == key2 {
		t.Error("different queries produced same key")
	}
}

func TestGenerateKey_QueryOrderIrrelevant(t *testing.T) {
	r1, _ := http.NewRequest("GET", "http://localhost/v1/users?page=1&limit=10", nil)
	r2, _ := http.NewRequest("GET", "http://localhost/v1/users?limit=10&page=1", nil)

	key1 := GenerateKey(r1, nil, KeyOptions{})
	key2 := GenerateKey(r2, nil, KeyOptions{})

	if key1 != key2 {
		t.Errorf("query param order affected key: %s vs %s", key1, key2)
	}
}

func TestGenerateKey_IgnoreHeaders(t *testing.T) {
	r1, _ := http.NewRequest("GET", "http://localhost/v1/users", nil)
	r1.Header.Set("Authorization", "Bearer token1")

	r2, _ := http.NewRequest("GET", "http://localhost/v1/users", nil)
	r2.Header.Set("Authorization", "Bearer token2")

	opts := KeyOptions{IgnoreHeaders: []string{"Authorization"}}
	key1 := GenerateKey(r1, nil, opts)
	key2 := GenerateKey(r2, nil, opts)

	if key1 != key2 {
		t.Errorf("ignored header affected key: %s vs %s", key1, key2)
	}
}

func TestGenerateKey_IgnoreQuery(t *testing.T) {
	r1, _ := http.NewRequest("GET", "http://localhost/v1/users?page=1&timestamp=111", nil)
	r2, _ := http.NewRequest("GET", "http://localhost/v1/users?page=1&timestamp=222", nil)

	opts := KeyOptions{IgnoreQuery: []string{"timestamp"}}
	key1 := GenerateKey(r1, nil, opts)
	key2 := GenerateKey(r2, nil, opts)

	if key1 != key2 {
		t.Errorf("ignored query param affected key: %s vs %s", key1, key2)
	}
}

func TestGenerateKey_IgnoreBodyFields(t *testing.T) {
	body1 := []byte(`{"name":"test","request_id":"abc123"}`)
	body2 := []byte(`{"name":"test","request_id":"xyz789"}`)

	r1, _ := http.NewRequest("POST", "http://localhost/v1/users", nil)
	r2, _ := http.NewRequest("POST", "http://localhost/v1/users", nil)

	opts := KeyOptions{IgnoreBody: []string{"request_id"}}
	key1 := GenerateKey(r1, body1, opts)
	key2 := GenerateKey(r2, body2, opts)

	if key1 != key2 {
		t.Errorf("ignored body field affected key: %s vs %s", key1, key2)
	}
}

func TestGenerateKey_DifferentBodyDifferentKeys(t *testing.T) {
	body1 := []byte(`{"name":"alice"}`)
	body2 := []byte(`{"name":"bob"}`)

	r1, _ := http.NewRequest("POST", "http://localhost/v1/users", nil)
	r2, _ := http.NewRequest("POST", "http://localhost/v1/users", nil)

	key1 := GenerateKey(r1, body1, KeyOptions{})
	key2 := GenerateKey(r2, body2, KeyOptions{})

	if key1 == key2 {
		t.Error("different bodies produced same key")
	}
}

func TestGenerateKey_TrailingSlashNormalized(t *testing.T) {
	r1, _ := http.NewRequest("GET", "http://localhost/v1/users", nil)
	r2, _ := http.NewRequest("GET", "http://localhost/v1/users/", nil)

	key1 := GenerateKey(r1, nil, KeyOptions{})
	key2 := GenerateKey(r2, nil, KeyOptions{})

	if key1 != key2 {
		t.Errorf("trailing slash affected key: %s vs %s", key1, key2)
	}
}

func TestGenerateKey_KeyLength(t *testing.T) {
	r, _ := http.NewRequest("GET", "http://localhost/v1/users", nil)
	key := GenerateKey(r, nil, KeyOptions{})

	if len(key) != 16 {
		t.Errorf("key length should be 16, got %d: %s", len(key), key)
	}
}

func TestGenerateKey_StableAcrossRuns(t *testing.T) {
	// Key should not contain any randomness
	r, _ := http.NewRequest("GET", "http://localhost/v1/users?page=1", nil)

	keys := make(map[string]bool)
	for i := 0; i < 100; i++ {
		key := GenerateKey(r, nil, KeyOptions{})
		keys[key] = true
	}

	if len(keys) != 1 {
		t.Errorf("key was not stable across runs, got %d unique keys", len(keys))
	}
}

func TestGenerateKey_URLEncodingNormalized(t *testing.T) {
	r1, _ := http.NewRequest("GET", "http://localhost/v1/users%2Ftest", nil)
	r2, _ := http.NewRequest("GET", "http://localhost/v1/users/test", nil)

	key1 := GenerateKey(r1, nil, KeyOptions{})
	key2 := GenerateKey(r2, nil, KeyOptions{})

	if key1 != key2 {
		t.Errorf("URL encoding affected key: %s vs %s", key1, key2)
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/v1/users", "/v1/users"},
		{"/v1/users/", "/v1/users"},
		{"/", "/"},
		{"/v1/users%2Ftest", "/v1/users/test"},
	}

	for _, tt := range tests {
		got := normalizePath(tt.input)
		if got != tt.expected {
			t.Errorf("normalizePath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNormalizeQuery(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		ignore []string
		want   string
	}{
		{
			name:  "sorted",
			input: "b=2&a=1",
			want:  "a=1&b=2",
		},
		{
			name:   "ignored",
			input:  "a=1&timestamp=123",
			ignore: []string{"timestamp"},
			want:   "a=1",
		},
		{
			name:   "case insensitive ignore",
			input:  "a=1&Timestamp=123",
			ignore: []string{"timestamp"},
			want:   "a=1",
		},
		{
			name:   "wildcard ignores all",
			input:  "a=1&b=2&c=3",
			ignore: []string{"*"},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", "http://localhost/?"+tt.input, nil)
			got := normalizeQuery(r.URL.Query(), tt.ignore)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHashBody(t *testing.T) {
	t.Run("empty body", func(t *testing.T) {
		got := hashBody(nil, nil, false)
		if got != "" {
			t.Errorf("empty body should return empty hash, got %q", got)
		}
	})

	t.Run("deterministic", func(t *testing.T) {
		body := []byte(`{"key":"value"}`)
		h1 := hashBody(body, nil, false)
		h2 := hashBody(body, nil, false)
		if h1 != h2 {
			t.Errorf("hashes differ: %s vs %s", h1, h2)
		}
	})

	t.Run("ignore fields", func(t *testing.T) {
		body1 := []byte(`{"name":"test","id":"123"}`)
		body2 := []byte(`{"name":"test","id":"456"}`)
		h1 := hashBody(body1, []string{"id"}, false)
		h2 := hashBody(body2, []string{"id"}, false)
		if h1 != h2 {
			t.Errorf("ignoring field should produce same hash: %s vs %s", h1, h2)
		}
	})

	t.Run("ignore all", func(t *testing.T) {
		body := []byte(`{"name":"test","id":"123"}`)
		got := hashBody(body, nil, true)
		if got != "" {
			t.Errorf("ignore all should return empty string, got %q", got)
		}
	})
}

func TestGenerateKey_IgnoreAllQuery(t *testing.T) {
	r1, _ := http.NewRequest("GET", "http://localhost/v1/users?page=1&limit=10", nil)
	r2, _ := http.NewRequest("GET", "http://localhost/v1/users?page=2&sort=desc", nil)

	opts := KeyOptions{IgnoreQuery: []string{"*"}}
	key1 := GenerateKey(r1, nil, opts)
	key2 := GenerateKey(r2, nil, opts)

	if key1 != key2 {
		t.Errorf("wildcard ignore_query should produce same key: %s vs %s", key1, key2)
	}
}

func TestGenerateKey_IgnoreAllBody(t *testing.T) {
	body1 := []byte(`{"name":"alice","age":30}`)
	body2 := []byte(`{"name":"bob","age":25}`)

	r1, _ := http.NewRequest("POST", "http://localhost/v1/users", nil)
	r2, _ := http.NewRequest("POST", "http://localhost/v1/users", nil)

	opts := KeyOptions{IgnoreAllBody: true}
	key1 := GenerateKey(r1, body1, opts)
	key2 := GenerateKey(r2, body2, opts)

	if key1 != key2 {
		t.Errorf("ignore_all_body should produce same key: %s vs %s", key1, key2)
	}
}

func TestGenerateKey_IgnoreAllBodyViaWildcard(t *testing.T) {
	body1 := []byte(`{"name":"alice"}`)
	body2 := []byte(`{"name":"bob"}`)

	r1, _ := http.NewRequest("POST", "http://localhost/v1/users", nil)
	r2, _ := http.NewRequest("POST", "http://localhost/v1/users", nil)

	opts := KeyOptions{IgnoreBody: []string{"*"}}
	key1 := GenerateKey(r1, body1, opts)
	key2 := GenerateKey(r2, body2, opts)

	if key1 != key2 {
		t.Errorf("wildcard ignore_body should produce same key: %s vs %s", key1, key2)
	}
}

func TestHashBodyFromReader(t *testing.T) {
	body, hash := HashBodyFromReader(strings.NewReader(`hello`))
	if string(body) != "hello" {
		t.Errorf("body mismatch: %q", body)
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}
	if len(hash) != 16 {
		t.Errorf("hash length should be 16, got %d", len(hash))
	}
}
