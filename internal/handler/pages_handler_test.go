package handler

import (
	"testing"
	"time"

	"optipay/internal/model"
)

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
