package drift

import (
	"testing"

	"github.com/splusq/resemble/internal/cache"
)

func TestCompare_NoDrift(t *testing.T) {
	old := &cache.Entry{
		Response: cache.ResponseMeta{
			Status:  200,
			Headers: map[string]string{"Content-Type": "application/json"},
		},
	}
	new := &cache.Entry{
		Response: cache.ResponseMeta{
			Status:  200,
			Headers: map[string]string{"Content-Type": "application/json"},
		},
	}

	result := Compare(old, new, []byte(`{"key":"value"}`), []byte(`{"key":"value"}`), nil)

	if result.HasDrift {
		t.Error("expected no drift")
	}
	if result.Summary != "no drift" {
		t.Errorf("summary = %q", result.Summary)
	}
}

func TestCompare_StatusCodeDrift(t *testing.T) {
	old := &cache.Entry{
		Response: cache.ResponseMeta{Status: 200, Headers: map[string]string{}},
	}
	new := &cache.Entry{
		Response: cache.ResponseMeta{Status: 500, Headers: map[string]string{}},
	}

	result := Compare(old, new, nil, nil, nil)

	if !result.HasDrift {
		t.Error("expected drift")
	}
	if !result.StatusChanged {
		t.Error("expected status change")
	}
}

func TestCompare_BodyDrift(t *testing.T) {
	old := &cache.Entry{
		Response: cache.ResponseMeta{Status: 200, Headers: map[string]string{"Content-Type": "text/plain"}},
	}
	new := &cache.Entry{
		Response: cache.ResponseMeta{Status: 200, Headers: map[string]string{"Content-Type": "text/plain"}},
	}

	result := Compare(old, new, []byte("old body"), []byte("new body"), nil)

	if !result.HasDrift {
		t.Error("expected drift")
	}
	if !result.BodyChanged {
		t.Error("expected body change")
	}
}

func TestCompare_JSONKeyOrderNotDrift(t *testing.T) {
	old := &cache.Entry{
		Response: cache.ResponseMeta{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}},
	}
	new := &cache.Entry{
		Response: cache.ResponseMeta{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}},
	}

	oldBody := []byte(`{"a":1,"b":2}`)
	newBody := []byte(`{"b":2,"a":1}`)

	result := Compare(old, new, oldBody, newBody, nil)

	if result.HasDrift {
		t.Error("JSON key order should not cause drift")
	}
	if result.BodyChanged {
		t.Error("JSON key order should not be body change")
	}
}

func TestCompare_IgnoredHeaders(t *testing.T) {
	old := &cache.Entry{
		Response: cache.ResponseMeta{
			Status:  200,
			Headers: map[string]string{"Content-Type": "text/plain", "Date": "Mon, 01 Jan 2024"},
		},
	}
	new := &cache.Entry{
		Response: cache.ResponseMeta{
			Status:  200,
			Headers: map[string]string{"Content-Type": "text/plain", "Date": "Tue, 02 Jan 2024"},
		},
	}

	result := Compare(old, new, nil, nil, []string{"Date"})

	if result.HasDrift {
		t.Error("ignored header change should not cause drift")
	}
}

func TestCompare_HeaderAdded(t *testing.T) {
	old := &cache.Entry{
		Response: cache.ResponseMeta{
			Status:  200,
			Headers: map[string]string{"Content-Type": "text/plain"},
		},
	}
	new := &cache.Entry{
		Response: cache.ResponseMeta{
			Status:  200,
			Headers: map[string]string{"Content-Type": "text/plain", "X-New": "value"},
		},
	}

	result := Compare(old, new, nil, nil, nil)

	if !result.HasDrift {
		t.Error("added header should cause drift")
	}
}

func TestCompare_HeaderRemoved(t *testing.T) {
	old := &cache.Entry{
		Response: cache.ResponseMeta{
			Status:  200,
			Headers: map[string]string{"Content-Type": "text/plain", "X-Old": "value"},
		},
	}
	new := &cache.Entry{
		Response: cache.ResponseMeta{
			Status:  200,
			Headers: map[string]string{"Content-Type": "text/plain"},
		},
	}

	result := Compare(old, new, nil, nil, nil)

	if !result.HasDrift {
		t.Error("removed header should cause drift")
	}
}

func TestCompare_JSONFieldAdded(t *testing.T) {
	old := &cache.Entry{
		Response: cache.ResponseMeta{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}},
	}
	new := &cache.Entry{
		Response: cache.ResponseMeta{Status: 200, Headers: map[string]string{"Content-Type": "application/json"}},
	}

	oldBody := []byte(`{"name":"test"}`)
	newBody := []byte(`{"name":"test","age":30}`)

	result := Compare(old, new, oldBody, newBody, nil)

	if !result.HasDrift {
		t.Error("added JSON field should cause drift")
	}
}
