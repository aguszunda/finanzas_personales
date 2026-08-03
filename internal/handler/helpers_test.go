package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"administracion-financiera/internal/middleware"
	"administracion-financiera/internal/model"
)

type testPayload struct {
	Nombre  string  `json:"nombre"`
	Monto   float64 `json:"monto"`
	EsFijo  bool    `json:"es_fijo"`
	Cuantas int64   `json:"cuantas"`
	Ptr     *int    `json:"ptr"`
}

func formRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

func TestDecodeBody_Form(t *testing.T) {
	r := formRequest(t, "nombre=Comida&monto=45.5&es_fijo=true&cuantas=3&ptr=7")
	var p testPayload
	if err := decodeBody(r, &p); err != nil {
		t.Fatalf("decodeBody: %v", err)
	}
	if p.Nombre != "Comida" {
		t.Errorf("nombre = %q", p.Nombre)
	}
	if p.Monto != 45.5 {
		t.Errorf("monto = %v", p.Monto)
	}
	if !p.EsFijo {
		t.Error("es_fijo should be true")
	}
	if p.Cuantas != 3 {
		t.Errorf("cuantas = %d", p.Cuantas)
	}
	if p.Ptr == nil || *p.Ptr != 7 {
		t.Errorf("ptr = %v", p.Ptr)
	}
}

func TestDecodeBody_FormEmptyPointerStaysNil(t *testing.T) {
	r := formRequest(t, "nombre=Comida&monto=10")
	var p testPayload
	if err := decodeBody(r, &p); err != nil {
		t.Fatalf("decodeBody: %v", err)
	}
	if p.Ptr != nil {
		t.Errorf("ptr should stay nil, got %v", *p.Ptr)
	}
}

func TestDecodeBody_JSON(t *testing.T) {
	body := `{"nombre":"Comida","monto":45.5,"es_fijo":false,"cuantas":3,"ptr":null}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	var p testPayload
	if err := decodeBody(r, &p); err != nil {
		t.Fatalf("decodeBody: %v", err)
	}
	if p.Nombre != "Comida" || p.Monto != 45.5 || p.Cuantas != 3 {
		t.Errorf("unexpected payload: %+v", p)
	}
}

func TestDecodeBody_BadJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{not json"))
	r.Header.Set("Content-Type", "application/json")
	var p testPayload
	if err := decodeBody(r, &p); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDecodeBody_MonedaDefaultFromForm(t *testing.T) {
	// Regression test: moneda_default must be decoded from urlencoded forms
	// (the register screen). It depends on the json tag on the input field.
	var p struct {
		MonedaDefault string `json:"moneda_default"`
	}
	r := formRequest(t, "moneda_default=USD")
	if err := decodeBody(r, &p); err != nil {
		t.Fatalf("decodeBody: %v", err)
	}
	if p.MonedaDefault != "USD" {
		t.Errorf("expected USD, got %q", p.MonedaDefault)
	}
}

func TestRespondMutation_HTMX(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("HX-Request", "true")
	ctx := context.WithValue(r.Context(), middleware.IsHTMXKey, true)
	rec := httptest.NewRecorder()
	respondMutation(rec, r.WithContext(ctx), http.StatusCreated, map[string]string{"ok": "1"}, "/api/transacciones/page")

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
	if rec.Header().Get("HX-Redirect") != "/api/transacciones/page" {
		t.Errorf("missing HX-Redirect: %q", rec.Header().Get("HX-Redirect"))
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body for HTMX, got %q", rec.Body.String())
	}
}

func TestRespondMutation_Form(t *testing.T) {
	r := formRequest(t, "x=1")
	rec := httptest.NewRecorder()
	respondMutation(rec, r, http.StatusCreated, map[string]string{"ok": "1"}, "/api/transacciones/page")
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rec.Code)
	}
	if rec.Header().Get("Location") != "/api/transacciones/page" {
		t.Errorf("unexpected Location: %q", rec.Header().Get("Location"))
	}
}

func TestRespondMutation_JSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	respondMutation(rec, r, http.StatusCreated, map[string]string{"ok": "1"}, "/api/transacciones/page")
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["ok"] != "1" {
		t.Errorf("unexpected body: %v", body)
	}
}

func TestHandleServiceError_Mapping(t *testing.T) {
	tests := []struct {
		err      error
		wantCode int
	}{
		{model.ErrNotFound, http.StatusNotFound},
		{model.ErrUnauthorized, http.StatusUnauthorized},
		{model.ErrInvalidInput, http.StatusBadRequest},
		{model.ErrEmailExiste, http.StatusConflict},
		{model.ErrMesCerrado, http.StatusConflict},
		{errors.New("otro error"), http.StatusInternalServerError},
		{model.ErrForbidden, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		handleServiceError(rec, tt.err)
		if rec.Code != tt.wantCode {
			t.Errorf("error %v: expected %d, got %d", tt.err, tt.wantCode, rec.Code)
		}
	}
}
