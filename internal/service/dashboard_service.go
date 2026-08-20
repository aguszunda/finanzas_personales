package service

import (
	"context"
	"sort"
	"time"

	"optipay/internal/model"
	"optipay/internal/repository"
)

type DashboardService struct {
	mesRepo         *repository.MesRepo
	transaccionRepo *repository.TransaccionRepo
	categoriaRepo   *repository.CategoriaRepo
	deudaRepo       *repository.DeudaRepo
}

func NewDashboardService(mr *repository.MesRepo, tr *repository.TransaccionRepo, cr *repository.CategoriaRepo, dr *repository.DeudaRepo) *DashboardService {
	return &DashboardService{mesRepo: mr, transaccionRepo: tr, categoriaRepo: cr, deudaRepo: dr}
}

type DashboardData struct {
	MesActual          *model.Mes         `json:"mes_actual"`
	MesAnterior        *model.Mes         `json:"mes_anterior,omitempty"`
	GastosPorCategoria []CategoriaGasto   `json:"gastos_por_categoria"`
	UltimosMovimientos []model.Movimiento `json:"ultimos_movimientos"`
}

type CategoriaGasto struct {
	CategoriaID int64   `json:"categoria_id"`
	Categoria   string  `json:"categoria"`
	Monto       float64 `json:"monto"`
	Porcentaje  float64 `json:"porcentaje"`
	Icono       string  `json:"icono"`
}

func (s *DashboardService) GetDashboard(ctx context.Context, usuarioID int64, periodo string) (*DashboardData, error) {
	periodoActual := time.Now().Format("2006-01")
	mesActual, err := s.mesRepo.FindOrCreate(ctx, usuarioID, periodoActual)
	if err != nil {
		return nil, err
	}
	if mesActual.Estado == "cerrado" {
		meses, err := s.mesRepo.FindByUsuarioID(ctx, usuarioID)
		if err != nil {
			return nil, err
		}
		if abierto := primerMesAbierto(meses, periodoActual); abierto != "" {
			periodoActual = abierto
			mesActual, err = s.mesRepo.FindOrCreate(ctx, usuarioID, abierto)
			if err != nil {
				return nil, err
			}
		}
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
	// Feed unificado de "Últimos Movimientos": transacciones + deudas.
	// Por defecto se muestran los últimos 10 días; si se filtra por mes, la
	// ventana reemplaza los 10 días por el período completo.
	desde, hasta := rango10Dias()
	if periodo != "" {
		desde, hasta = periodo+"-01", periodo+"-31"
	}
	transaccionesUltimos, err := s.transaccionRepo.FindByRango(ctx, usuarioID, desde, hasta)
	if err != nil {
		return nil, err
	}
	deudasUltimos, err := s.deudaRepo.FindByRango(ctx, usuarioID, desde, hasta)
	if err != nil {
		return nil, err
	}
	movimientos, err := s.unirMovimientos(transaccionesUltimos, deudasUltimos)
	if err != nil {
		return nil, err
	}
	return &DashboardData{
		MesActual:          mesActual,
		MesAnterior:        mesAnterior,
		GastosPorCategoria: gastosPorCat,
		UltimosMovimientos: movimientos,
	}, nil
}

func primerMesAbierto(meses []model.Mes, periodo string) string {
	var best string
	for _, m := range meses {
		if m.Periodo > periodo && m.Estado == "abierto" {
			if best == "" || m.Periodo < best {
				best = m.Periodo
			}
		}
	}
	return best
}

// rango10Dias devuelve el rango (desde, hasta) de los últimos 10 días.
func rango10Dias() (string, string) {
	hasta := time.Now()
	desde := hasta.AddDate(0, 0, -9)
	return desde.Format("2006-01-02"), hasta.Format("2006-01-02")
}

// unirMovimientos combina transacciones y deudas en un único feed ordenado
// por fecha desc. Las deudas se muestran como movimientos con su monto total
// y fecha de alta (created_at).
func (s *DashboardService) unirMovimientos(transacciones []model.Transaccion, deudas []model.Deuda) ([]model.Movimiento, error) {
	movimientos := make([]model.Movimiento, 0, len(transacciones)+len(deudas))
	for _, t := range transacciones {
		movimientos = append(movimientos, model.Movimiento{
			ID:          t.ID,
			Origen:      "transaccion",
			Tipo:        t.Tipo,
			Monto:       t.Monto,
			Fecha:       t.Fecha,
			Categoria:   t.Categoria,
			Descripcion: t.Descripcion,
			CreatedAt:   t.CreatedAt,
		})
	}
	for _, d := range deudas {
		movimientos = append(movimientos, model.Movimiento{
			ID:          d.ID,
			Origen:      "deuda",
			Tipo:        "deuda",
			Monto:       d.MontoTotal,
			Fecha:       d.CreatedAt.Format("2006-01-02"),
			Categoria:   d.Entidad,
			Descripcion: d.Descripcion,
			CreatedAt:   d.CreatedAt,
		})
	}
	sort.SliceStable(movimientos, func(i, j int) bool {
		if movimientos[i].Fecha != movimientos[j].Fecha {
			return movimientos[i].Fecha > movimientos[j].Fecha
		}
		if movimientos[i].CreatedAt.Equal(movimientos[j].CreatedAt) {
			return movimientos[i].ID > movimientos[j].ID
		}
		return movimientos[i].CreatedAt.After(movimientos[j].CreatedAt)
	})
	return movimientos, nil
}
