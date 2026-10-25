package accesslog

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type entry struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	LatencyMS  int64  `json:"latency_ms"`
	RequestID  string `json:"request_id"`
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Middleware logs one JSON line per request to w.
func Middleware(w io.Writer, next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: rw, status: http.StatusOK}
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = rw.Header().Get("X-Request-Id")
		}
		next.ServeHTTP(rec, r)
		if reqID == "" {
			reqID = rec.Header().Get("X-Request-Id")
		}
		e := entry{
			Method:    r.Method,
			Path:      r.URL.Path,
			Status:    rec.status,
			LatencyMS: time.Since(start).Milliseconds(),
			RequestID: reqID,
		}
		b, _ := json.Marshal(e)
		b = append(b, '\n')
		_, _ = w.Write(b)
	})
}
