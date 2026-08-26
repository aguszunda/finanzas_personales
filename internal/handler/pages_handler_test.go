package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"optipay/internal/model"
)

// newTestTemplateManager crea un TemplateManager mínimo a partir de un
// MapFS con los templates necesarios para renderTemplate/renderTemplateFragment.
func newTestTemplateManager(files fstest.MapFS) *TemplateManager {
	return NewTemplateManager(files)
}

func TestPrimerMesAbierto(t *testing.T) {
	tests := []struct {
		name    string
		meses   []model.Mes
		periodo string
		want    string
	}{
		{
			name:    "elige el menor periodo abierto mayor al dado",
			meses:   []model.Mes{{Periodo: "2026-04", Estado: "abierto"}, {Periodo: "2026-03", Estado: "cerrado"}, {Periodo: "2026-02", Estado: "abierto"}},
			periodo: "2026-01",
			want:    "2026-02",
		},
		{
			name:    "no toma meses cerrados",
			meses:   []model.Mes{{Periodo: "2026-04", Estado: "cerrado"}, {Periodo: "2026-03", Estado: "abierto"}},
			periodo: "2026-01",
			want:    "2026-03",
		},
		{
			name:    "no toma periodos menores o iguales",
			meses:   []model.Mes{{Periodo: "2026-01", Estado: "abierto"}, {Periodo: "2025-12", Estado: "abierto"}},
			periodo: "2026-01",
			want:    "",
		},
		{
			name:    "vacio si no hay mes abierto posterior",
			meses:   []model.Mes{{Periodo: "2026-03", Estado: "cerrado"}},
			periodo: "2026-01",
			want:    "",
		},
		{
			name:    "vacio si no hay meses",
			periodo: "2026-01",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := primerMesAbierto(tt.meses, tt.periodo); got != tt.want {
				t.Errorf("primerMesAbierto() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFechaEnMes(t *testing.T) {
	tests := []struct {
		name    string
		periodo string
		hoy     time.Time
		want    string
	}{
		{
			name:    "dia de hoy dentro del mes",
			periodo: "2026-08",
			hoy:     time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC),
			want:    "2026-08-14",
		},
		{
			name:    "recorta el dia si el mes es mas corto",
			periodo: "2026-02",
			hoy:     time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
			want:    "2026-02-28",
		},
		{
			name:    "mes febrero bisiesto",
			periodo: "2024-02",
			hoy:     time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
			want:    "2024-02-29",
		},
		{
			name:    "fallback al primer dia si el periodo es invalido",
			periodo: "invalido",
			hoy:     time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC),
			want:    "invalido-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fechaEnMes(tt.periodo, tt.hoy); got != tt.want {
				t.Errorf("fechaEnMes(%q) = %q, want %q", tt.periodo, got, tt.want)
			}
		})
	}
}

func TestMesByPeriodo(t *testing.T) {
	tests := []struct {
		name    string
		meses   []model.Mes
		periodo string
		wantNil bool
		wantP   string
	}{
		{
			name:    "encuentra el mes",
			meses:   []model.Mes{{Periodo: "2026-08", Estado: "abierto"}, {Periodo: "2026-07", Estado: "cerrado"}},
			periodo: "2026-07",
			wantNil: false,
			wantP:   "2026-07",
		},
		{
			name:    "devuelve nil si no hay match",
			meses:   []model.Mes{{Periodo: "2026-08"}, {Periodo: "2026-07"}},
			periodo: "2026-01",
			wantNil: true,
		},
		{
			name:    "devuelve nil con slice vacío",
			periodo: "2026-01",
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mesByPeriodo(tt.meses, tt.periodo)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil result")
			}
			if got.Periodo != tt.wantP {
				t.Errorf("Periodo = %q, want %q", got.Periodo, tt.wantP)
			}
		})
	}
}

func TestVerificacionPage(t *testing.T) {
	old := tmpl
	defer func() { tmpl = old }()

	fs := fstest.MapFS{
		"layout.html":       {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"verificacion.html": {Data: []byte(`{{define "content"}}<div>verificado</div>{{end}}`)},
		"reenvio_form.html": {Data: []byte(`{{define "reenvio_form"}}<form></form>{{end}}`)},
	}
	tmpl = newTestTemplateManager(fs)

	h := &PagesHandler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/verificacion?estado=ok", nil)
	h.VerificacionPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "verificado") {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestReenvioPage(t *testing.T) {
	old := tmpl
	defer func() { tmpl = old }()

	fs := fstest.MapFS{
		"layout.html":  {Data: []byte(`<html>{{template "content" .}}</html>`)},
		"reenvio.html": {Data: []byte(`{{define "content"}}<div>reenvio</div>{{end}}`)},
	}
	tmpl = newTestTemplateManager(fs)

	h := &PagesHandler{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/reenvio", nil)
	h.ReenvioPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
