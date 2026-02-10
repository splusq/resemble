package cache

import "time"

type RequestMeta struct {
	Method   string `json:"method"`
	Path     string `json:"path"`
	Query    string `json:"query"`
	BodyHash string `json:"body_hash"`
}

type ResponseMeta struct {
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers"`
	BodyFile string            `json:"body_file"`
	BodySize int64             `json:"body_size"`
}

type Entry struct {
	Version       int          `json:"version"`
	Key           string       `json:"key"`
	Request       RequestMeta  `json:"request"`
	Response      ResponseMeta `json:"response"`
	CachedAt      time.Time    `json:"cached_at"`
	TTL           string       `json:"ttl"`
	LastRefreshed time.Time    `json:"last_refreshed"`
	RefreshCount  int          `json:"refresh_count"`
	DriftDetected bool         `json:"drift_detected"`
}

func (e *Entry) IsStale(ttl time.Duration) bool {
	if ttl == 0 {
		return false
	}
	return time.Since(e.LastRefreshed) > ttl
}
