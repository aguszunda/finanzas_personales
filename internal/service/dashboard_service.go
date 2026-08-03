package service

import (
	"context"
	"time"

	"administracion-financiera/internal/model"
	"administracion-financiera/internal/repository"
)

type DashboardService struct {
	mesRepo         *repository.MesRepo
	transaccionRepo *repository.TransaccionRepo
	categoriaRepo   *repository.CategoriaRepo
}

func NewDashboardService(mr *repository.MesRepo, tr *repository.TransaccionRepo, cr *repository.CategoriaRepo) *DashboardService {
	return &DashboardService{mesRepo: mr, transaccionRepo: tr, categoriaRepo: cr}
}

type DashboardData struct {
	MesActual          *model.Mes          `json:"mes_actual"`
	MesAnterior        *model.Mes          `json:"mes_anterior,omitempty"`
	GastosPorCategoria []CategoriaGasto    `json:"gastos_por_categoria"`
	UltimosMovimientos []model.Transaccion `json:"ultimos_movimientos"`
}

type CategoriaGasto struct {
	CategoriaID int64   `json:"categoria_id"`
	Categoria   string  `json:"categoria"`
	Monto       float64 `json:"monto"`
	Porcentaje  float64 `json:"porcentaje"`
	Icono       string  `json:"icono"`
}

func (s *DashboardService) GetDashboard(ctx context.Context, usuarioID int64) (*DashboardData, error) {
	periodoActual := time.Now().Format("2006-01")
	mesActual, err := s.mesRepo.FindOrCreate(ctx, usuarioID, periodoActual)
	if err != nil {
		return nil, err
	}
	transacciones, err := s.transaccionRepo.FindByPeriodo(ctx, usuarioID, periodoActual)
	if err != nil {
		return nil, err
	}
	var totalIngresos, totalEgresos float64
	for _, t := range transacciones {
		switch t.Tipo {
		case "ingreso":
			totalIngresos += t.Monto
		case "egreso":
			totalEgresos += t.Monto
		}
	}
	mesActual.IngresosTotal = totalIngresos
	mesActual.EgresosTotal = totalEgresos
	mesActual.Superavit = totalIngresos - totalEgresos
	if totalIngresos > 0 {
		tasa := (mesActual.Superavit / totalIngresos) * 100
		mesActual.TasaAhorro = &tasa
	}
	t, err := time.Parse("2006-01", periodoActual)
	if err != nil {
		return nil, err
	}
	periodoAnterior := t.AddDate(0, -1, 0).Format("2006-01")
	mesAnterior, _ := s.mesRepo.FindByPeriodo(ctx, usuarioID, periodoAnterior)
	categorias, err := s.categoriaRepo.FindAll(ctx, usuarioID)
	if err != nil {
		return nil, err
	}
	catMap := make(map[int64]model.Categoria)
	for _, c := range categorias {
		catMap[c.ID] = c
	}
	egresos := make(map[int64]float64)
	for _, t := range transacciones {
		if t.Tipo == "egreso" {
			egresos[t.CategoriaID] += t.Monto
		}
	}
	var gastosPorCat []CategoriaGasto
	for catID, monto := range egresos {
		pct := 0.0
		if totalEgresos > 0 {
			pct = (monto / totalEgresos) * 100
		}
		cat := CategoriaGasto{
			CategoriaID: catID,
			Monto:       monto,
			Porcentaje:  pct,
		}
		if c, ok := catMap[catID]; ok {
			cat.Categoria = c.Nombre
			cat.Icono = c.Icono
		}
		gastosPorCat = append(gastosPorCat, cat)
	}
	var ultimos []model.Transaccion
	if len(transacciones) > 5 {
		ultimos = transacciones[:5]
	} else {
		ultimos = transacciones
	}
	return &DashboardData{
		MesActual:          mesActual,
		MesAnterior:        mesAnterior,
		GastosPorCategoria: gastosPorCat,
		UltimosMovimientos: ultimos,
	}, nil
}
