package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDetectHTMX_WithHeader(t *testing.T) {
	var got bool
	h := DetectHTMX(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IsHTMXRequest(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	h.ServeHTTP(rec, req)
	if !got {
		t.Error("expected IsHTMXRequest to return true")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestDetectHTMX_WithoutHeader(t *testing.T) {
	var got bool
	h := DetectHTMX(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IsHTMXRequest(r.Context())
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	if got {
		t.Error("expected IsHTMXRequest to return false")
	}
}

func TestIsHTMXRequest_EmptyContext(t *testing.T) {
	if IsHTMXRequest(context.Background()) {
		t.Error("expected false on empty context")
	}
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, status: http.StatusOK}
	rw.WriteHeader(http.StatusCreated)
	if rw.status != http.StatusCreated {
		t.Fatalf("expected stored status %d, got %d", http.StatusCreated, rw.status)
	}
	// Double WriteHeader — underlying recorder allows it; our wrapper just overwrites.
	rw.WriteHeader(http.StatusGone)
	if rw.status != http.StatusGone {
		t.Fatalf("expected stored status %d after second call, got %d", http.StatusGone, rw.status)
	}
}

func TestLogging_Middleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	h := Logging(inner)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("unexpected body: %q", rec.Body.String())
	}
}

func TestLogging_Middleware_CustomStatus(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	})
	h := Logging(inner)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestLogging_Middleware_NoExplicitWriteHeader(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("implicit 200"))
	})
	h := Logging(inner)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (implicit), got %d", rec.Code)
	}
	if rec.Body.String() != "implicit 200" {
		t.Errorf("unexpected body: %q", rec.Body.String())
	}
}

func TestResponseWriter_DefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, status: http.StatusOK}
	if rw.status != http.StatusOK {
		t.Fatalf("expected default status 200, got %d", rw.status)
	}
}

func TestDetectHTMX_WrongHeaderValue(t *testing.T) {
	var got bool
	h := DetectHTMX(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IsHTMXRequest(r.Context())
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "yes")
	h.ServeHTTP(rec, req)
	if got {
		t.Error("expected false when HX-Request header is not exactly 'true'")
	}
}

func TestLogging_RecordsCorrectStatus(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	h := Logging(inner)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/create", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}
