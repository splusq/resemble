package drift

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/splusq/resemble/internal/cache"
)

type Result struct {
	HasDrift      bool
	StatusChanged bool
	HeaderDiffs   []string
	BodyChanged   bool
	Summary       string
}

func Compare(old, new *cache.Entry, oldBody, newBody []byte, ignoreHeaders []string) *Result {
	r := &Result{}

	// Status code comparison
	if old.Response.Status != new.Response.Status {
		r.HasDrift = true
		r.StatusChanged = true
	}

	// Header comparison
	ignoreSet := make(map[string]bool, len(ignoreHeaders))
	for _, h := range ignoreHeaders {
		ignoreSet[strings.ToLower(h)] = true
	}

	for k, v := range new.Response.Headers {
		if ignoreSet[strings.ToLower(k)] {
			continue
		}
		oldVal, exists := old.Response.Headers[k]
		if !exists {
			r.HasDrift = true
			r.HeaderDiffs = append(r.HeaderDiffs, fmt.Sprintf("+%s: %s", k, v))
		} else if oldVal != v {
			r.HasDrift = true
			r.HeaderDiffs = append(r.HeaderDiffs, fmt.Sprintf("~%s: %q -> %q", k, oldVal, v))
		}
	}
	for k := range old.Response.Headers {
		if ignoreSet[strings.ToLower(k)] {
			continue
		}
		if _, exists := new.Response.Headers[k]; !exists {
			r.HasDrift = true
			r.HeaderDiffs = append(r.HeaderDiffs, fmt.Sprintf("-%s", k))
		}
	}

	// Body comparison
	contentType := new.Response.Headers["Content-Type"]
	if strings.Contains(contentType, "application/json") {
		r.BodyChanged = !jsonEqual(oldBody, newBody)
	} else {
		r.BodyChanged = !bytes.Equal(oldBody, newBody)
	}
	if r.BodyChanged {
		r.HasDrift = true
	}

	// Build summary
	var parts []string
	if r.StatusChanged {
		parts = append(parts, fmt.Sprintf("status: %d -> %d", old.Response.Status, new.Response.Status))
	}
	if len(r.HeaderDiffs) > 0 {
		parts = append(parts, fmt.Sprintf("headers: %d changes", len(r.HeaderDiffs)))
	}
	if r.BodyChanged {
		parts = append(parts, "body changed")
	}
	if len(parts) == 0 {
		r.Summary = "no drift"
	} else {
		r.Summary = "DRIFT: " + strings.Join(parts, ", ")
	}

	return r
}

func jsonEqual(a, b []byte) bool {
	var objA, objB interface{}
	if json.Unmarshal(a, &objA) != nil || json.Unmarshal(b, &objB) != nil {
		return bytes.Equal(a, b)
	}
	normA, _ := json.Marshal(objA)
	normB, _ := json.Marshal(objB)
	return bytes.Equal(normA, normB)
}
