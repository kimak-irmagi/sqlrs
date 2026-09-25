package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCacheStatusRequiresAuth(t *testing.T) {
	opts, cleanup := newRouteTestOptions(t)
	defer cleanup()

	handler := NewHandler(opts)
	req := httptest.NewRequest(http.MethodGet, "/v1/cache/status", nil)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
}

func TestCacheStatusReturnsPayload(t *testing.T) {
	opts, cleanup := newRouteTestOptions(t)
	defer cleanup()

	handler := NewHandler(opts)
	req := httptest.NewRequest(http.MethodGet, "/v1/cache/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	for _, key := range []string{"usage_bytes", "effective_max_bytes", "reserve_bytes", "state_count", "pressure_reasons"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected %q in payload, got %+v", key, payload)
		}
	}
}

func TestCacheStatusRejectsWrongMethodAndMissingService(t *testing.T) {
	t.Run("method", func(t *testing.T) {
		opts, cleanup := newRouteTestOptions(t)
		defer cleanup()
		req := httptest.NewRequest(http.MethodPost, "/v1/cache/status", nil)
		req.Header.Set("Authorization", "Bearer secret")
		resp := httptest.NewRecorder()
		NewHandler(opts).ServeHTTP(resp, req)
		if resp.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", resp.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("missing service", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/cache/status", nil)
		req.Header.Set("Authorization", "Bearer secret")
		resp := httptest.NewRecorder()
		NewHandler(Options{AuthToken: "secret"}).ServeHTTP(resp, req)
		if resp.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", resp.Code, http.StatusInternalServerError)
		}
	})
}

func TestCacheStatusMapsServiceFailure(t *testing.T) {
	opts, cleanup := newRouteTestOptions(t)
	cleanup()
	req := httptest.NewRequest(http.MethodGet, "/v1/cache/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp := httptest.NewRecorder()
	NewHandler(opts).ServeHTTP(resp, req)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusInternalServerError)
	}
}

func TestStatesListIncludesCacheMetadataFields(t *testing.T) {
	server, cleanup := newTestServer(t)
	defer cleanup()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/states", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("states request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var entries []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		t.Fatalf("decode states: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected states in payload")
	}
	var found map[string]any
	for _, entry := range entries {
		if entry["state_id"] == "state-1" {
			found = entry
			break
		}
	}
	if found == nil {
		t.Fatalf("expected state-1 in payload, got %+v", entries)
	}
	for _, key := range []string{"size_bytes", "last_used_at", "use_count", "min_retention_until"} {
		if _, ok := found[key]; !ok {
			t.Fatalf("expected %q in state payload, got %+v", key, found)
		}
	}
	if found["size_bytes"] != float64(10) {
		t.Fatalf("expected size_bytes=10, got %+v", found["size_bytes"])
	}
}
