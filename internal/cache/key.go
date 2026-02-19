package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

type KeyOptions struct {
	IgnoreHeaders []string
	IgnoreQuery   []string
	IgnoreBody    []string // JSON field names to ignore
	IgnoreAllBody bool     // skip entire body from cache key
}

func GenerateKey(r *http.Request, body []byte, opts KeyOptions) string {
	h := sha256.New()

	// Method
	fmt.Fprintf(h, "%s\n", r.Method)

	// Normalized path
	path := normalizePath(r.URL.Path)
	fmt.Fprintf(h, "%s\n", path)

	// Sorted query params (excluding ignored)
	query := normalizeQuery(r.URL.Query(), opts.IgnoreQuery)
	fmt.Fprintf(h, "%s\n", query)

	// Body hash
	ignoreAllBody := opts.IgnoreAllBody || containsWildcard(opts.IgnoreBody)
	bodyHash := hashBody(body, opts.IgnoreBody, ignoreAllBody)
	fmt.Fprintf(h, "%s\n", bodyHash)

	sum := h.Sum(nil)
	return fmt.Sprintf("%x", sum)[:16]
}

func normalizePath(p string) string {
	// Remove trailing slash (except root)
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	// Unescape then re-escape for normalization
	unescaped, err := url.PathUnescape(p)
	if err != nil {
		return p
	}
	return unescaped
}

func containsWildcard(list []string) bool {
	for _, v := range list {
		if v == "*" {
			return true
		}
	}
	return false
}

func normalizeQuery(params url.Values, ignore []string) string {
	if containsWildcard(ignore) {
		return ""
	}

	ignoreSet := make(map[string]bool, len(ignore))
	for _, k := range ignore {
		ignoreSet[strings.ToLower(k)] = true
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		if !ignoreSet[strings.ToLower(k)] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		vals := params[k]
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return strings.Join(parts, "&")
}

func hashBody(body []byte, ignoreFields []string, ignoreAll bool) string {
	if ignoreAll || len(body) == 0 {
		return ""
	}

	// If we have fields to ignore and body is JSON, strip them
	if len(ignoreFields) > 0 {
		var parsed map[string]interface{}
		if json.Unmarshal(body, &parsed) == nil {
			for _, field := range ignoreFields {
				delete(parsed, field)
			}
			normalized, err := json.Marshal(parsed)
			if err == nil {
				body = normalized
			}
		}
	}

	h := sha256.New()
	h.Write(body)
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

func HashBodyFromReader(r io.Reader) ([]byte, string) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, ""
	}
	if len(body) == 0 {
		return body, ""
	}
	h := sha256.New()
	h.Write(body)
	return body, fmt.Sprintf("%x", h.Sum(nil))[:16]
}
