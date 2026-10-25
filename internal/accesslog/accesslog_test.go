package accesslog

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareLogsJSONLine(t *testing.T) {
	var buf bytes.Buffer
	h := Middleware(&buf, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "abc123")
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	line := strings.TrimSpace(buf.String())
	var e entry
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		t.Fatal(err)
	}
	if e.Method != "GET" || e.Path != "/v1/status" || e.Status != 201 || e.RequestID != "abc123" {
		t.Fatalf("unexpected entry: %+v", e)
	}
	if e.LatencyMS < 0 {
		t.Fatalf("latency: %d", e.LatencyMS)
	}
}
