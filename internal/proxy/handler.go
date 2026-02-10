package proxy

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/splusq/resemble/internal/cache"
)

type Handler struct {
	store       *cache.Store
	upstreamURL string
	mode        string
	ttl         time.Duration
	keyOpts     cache.KeyOptions
	client      *http.Client
}

func NewHandler(store *cache.Store, upstreamURL, mode string, ttl time.Duration, keyOpts cache.KeyOptions) *Handler {
	return &Handler{
		store:       store,
		upstreamURL: strings.TrimRight(upstreamURL, "/"),
		mode:        mode,
		ttl:         ttl,
		keyOpts:     keyOpts,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Read request body
	var body []byte
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		r.Body.Close()
	}

	host := upstreamHost(h.upstreamURL)
	key := cache.GenerateKey(r, body, h.keyOpts)

	switch h.mode {
	case "replay":
		h.handleReplay(w, r, host, key)
	case "record":
		h.handleRecord(w, r, host, key, body)
	default: // "auto"
		h.handleAuto(w, r, host, key, body)
	}
}

func (h *Handler) handleReplay(w http.ResponseWriter, r *http.Request, host, key string) {
	entry, body, err := h.store.Get(host, key)
	if err != nil {
		log.Printf("resemble: cache error: %v", err)
		http.Error(w, "cache read error", http.StatusInternalServerError)
		return
	}
	if entry == nil {
		log.Printf("resemble: MISS %s %s (replay mode, no cache)", r.Method, r.URL.Path)
		http.Error(w, fmt.Sprintf("resemble: cache miss in replay mode: %s %s", r.Method, r.URL.Path), http.StatusBadGateway)
		return
	}

	log.Printf("resemble: HIT %s %s [%s]", r.Method, r.URL.Path, key)
	writeResponse(w, entry, body)
}

func (h *Handler) handleRecord(w http.ResponseWriter, r *http.Request, host, key string, reqBody []byte) {
	entry, respBody, err := h.fetchUpstream(r, reqBody)
	if err != nil {
		log.Printf("resemble: upstream error: %v", err)
		http.Error(w, fmt.Sprintf("resemble: upstream error: %v", err), http.StatusBadGateway)
		return
	}

	entry.TTL = h.ttl.String()
	if err := h.store.Put(host, key, entry, respBody); err != nil {
		log.Printf("resemble: cache write error: %v", err)
	} else {
		log.Printf("resemble: RECORD %s %s [%s] -> %d", r.Method, r.URL.Path, key, entry.Response.Status)
	}

	writeResponse(w, entry, respBody)
}

func (h *Handler) handleAuto(w http.ResponseWriter, r *http.Request, host, key string, reqBody []byte) {
	// Try cache first
	entry, body, err := h.store.Get(host, key)
	if err != nil {
		log.Printf("resemble: cache error: %v", err)
	}

	if entry != nil && !entry.IsStale(h.ttl) {
		log.Printf("resemble: HIT %s %s [%s]", r.Method, r.URL.Path, key)
		writeResponse(w, entry, body)
		return
	}

	if entry != nil {
		log.Printf("resemble: STALE %s %s [%s], refreshing", r.Method, r.URL.Path, key)
	} else {
		log.Printf("resemble: MISS %s %s [%s], recording", r.Method, r.URL.Path, key)
	}

	// Fetch from upstream
	newEntry, respBody, err := h.fetchUpstream(r, reqBody)
	if err != nil {
		// If upstream fails but we have stale cache, serve it
		if entry != nil {
			log.Printf("resemble: upstream error, serving stale cache: %v", err)
			writeResponse(w, entry, body)
			return
		}
		log.Printf("resemble: upstream error: %v", err)
		http.Error(w, fmt.Sprintf("resemble: upstream error: %v", err), http.StatusBadGateway)
		return
	}

	newEntry.TTL = h.ttl.String()
	if entry != nil {
		newEntry.CachedAt = entry.CachedAt
		newEntry.RefreshCount = entry.RefreshCount + 1
	}

	if err := h.store.Put(host, key, newEntry, respBody); err != nil {
		log.Printf("resemble: cache write error: %v", err)
	}

	writeResponse(w, newEntry, respBody)
}

func (h *Handler) fetchUpstream(r *http.Request, reqBody []byte) (*cache.Entry, []byte, error) {
	upstreamURL := h.upstreamURL + r.URL.Path
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	var bodyReader io.Reader
	if len(reqBody) > 0 {
		bodyReader = bytes.NewReader(reqBody)
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bodyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("creating upstream request: %w", err)
	}

	// Copy headers
	for k, vals := range r.Header {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	// Remove hop-by-hop headers
	req.Header.Del("Connection")
	req.Header.Del("Keep-Alive")
	req.Header.Del("Transfer-Encoding")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("reading upstream response: %w", err)
	}

	// Build cache entry
	_, bodyHash := cache.HashBodyFromReader(bytes.NewReader(reqBody))
	headers := make(map[string]string)
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}

	entry := &cache.Entry{
		Request: cache.RequestMeta{
			Method:   r.Method,
			Path:     r.URL.Path,
			Query:    r.URL.RawQuery,
			BodyHash: bodyHash,
		},
		Response: cache.ResponseMeta{
			Status:  resp.StatusCode,
			Headers: headers,
		},
	}

	return entry, respBody, nil
}

func writeResponse(w http.ResponseWriter, entry *cache.Entry, body []byte) {
	for k, v := range entry.Response.Headers {
		w.Header().Set(k, v)
	}
	w.Header().Set("X-Resemble-Cache", "hit")
	w.WriteHeader(entry.Response.Status)
	w.Write(body)
}

func upstreamHost(upstreamURL string) string {
	// Extract host from URL like "https://api.example.com"
	host := upstreamURL
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	return host
}
