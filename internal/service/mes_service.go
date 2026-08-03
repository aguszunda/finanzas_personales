package service

import (
	"context"
	"fmt"
	"time"

	"finanzas_personales/internal/model"
	"finanzas_personales/internal/repository"
)

type MesService struct {
	mesRepo         *repository.MesRepo
	transaccionRepo *repository.TransaccionRepo
	costoFijoRepo   *repository.CostoFijoRepo
}

func NewMesService(mr *repository.MesRepo, tr *repository.TransaccionRepo, cfr *repository.CostoFijoRepo) *MesService {
	return &MesService{mesRepo: mr, transaccionRepo: tr, costoFijoRepo: cfr}
}

func (s *MesService) List(ctx context.Context, usuarioID int64) ([]model.Mes, error) {
	return s.mesRepo.FindByUsuarioID(ctx, usuarioID)
}

func (s *MesService) GetByID(ctx context.Context, id, usuarioID int64) (*model.Mes, error) {
	return s.mesRepo.FindByID(ctx, id, usuarioID)
}

func (s *MesService) GetCurrent(ctx context.Context, usuarioID int64) (*model.Mes, error) {
	periodo := time.Now().Format("2006-01")
	return s.mesRepo.FindOrCreate(ctx, usuarioID, periodo)
}

func (s *MesService) Cerrar(ctx context.Context, usuarioID, mesID int64) (*model.Mes, error) {
	mes, err := s.mesRepo.FindByID(ctx, mesID, usuarioID)
	if err != nil {
		return nil, err
	}
	if mes.Estado == "cerrado" {
		return nil, fmt.Errorf("el mes %s ya está cerrado", mes.Periodo)
	}
	transacciones, err := s.transaccionRepo.FindByPeriodo(ctx, usuarioID, mes.Periodo)
	if err != nil {
		return nil, err
	}
	totalIngresos, totalEgresos := calcularTotales(transacciones)
	mes.IngresosTotal = totalIngresos
	mes.EgresosTotal = totalEgresos
	mes.Superavit = totalIngresos - totalEgresos
	if totalIngresos > 0 {
		tasa := (mes.Superavit / totalIngresos) * 100
		mes.TasaAhorro = &tasa
	}
	mes.Estado = "cerrado"
	ultimo, err := s.mesRepo.GetUltimoCerrado(ctx, usuarioID)
	if err == nil {
		mes.AhorroAcumulado = ultimo.AhorroAcumulado + ultimo.Superavit
		mes.PasivosTotal = ultimo.PasivosTotal
	} else {
		mes.AhorroAcumulado = 0
	}
	mes.Patrimonio = mes.AhorroAcumulado - mes.PasivosTotal
	if err := s.mesRepo.Update(ctx, mes); err != nil {
		return nil, err
	}
	proximoPeriodo, err := nextPeriodo(mes.Periodo)
	if err != nil {
		return nil, err
	}
	proximoMes, err := s.mesRepo.FindOrCreate(ctx, usuarioID, proximoPeriodo)
	if err != nil {
		return nil, err
	}
	if err := s.SyncFijosPeriodo(ctx, usuarioID, proximoPeriodo); err != nil {
		return nil, err
	}
	proximoMes.Estado = "abierto"
	_ = s.mesRepo.Update(ctx, proximoMes)
	return mes, nil
}

func (s *MesService) Recalcular(ctx context.Context, usuarioID, mesID int64) (*model.Mes, error) {
	mes, err := s.mesRepo.FindByID(ctx, mesID, usuarioID)
	if err != nil {
		return nil, err
	}
	transacciones, err := s.transaccionRepo.FindByPeriodo(ctx, usuarioID, mes.Periodo)
	if err != nil {
		return nil, err
	}
	totalIngresos, totalEgresos := calcularTotales(transacciones)
	mes.IngresosTotal = totalIngresos
	mes.EgresosTotal = totalEgresos
	mes.Superavit = totalIngresos - totalEgresos
	if totalIngresos > 0 {
		tasa := (mes.Superavit / totalIngresos) * 100
		mes.TasaAhorro = &tasa
	}
	if err := s.mesRepo.Update(ctx, mes); err != nil {
		return nil, err
	}
	return mes, nil
}

func nextPeriodo(current string) (string, error) {
	t, err := time.Parse("2006-01", current)
	if err != nil {
		return "", err
	}
	return t.AddDate(0, 1, 0).Format("2006-01"), nil
}

func calcularTotales(transacciones []model.Transaccion) (ingresos, egresos float64) {
	for _, t := range transacciones {
		switch t.Tipo {
		case "ingreso":
			ingresos += t.Monto
		case "egreso":
			egresos += t.Monto
		}
	}
	return ingresos, egresos
}

// SyncFijosPeriodo materializa los costos fijos activos del usuario como
// transacciones "pendientes" en el período indicado (idempotente). No toca
// meses cerrados para preservar su inmutabilidad.
func (s *MesService) SyncFijosPeriodo(ctx context.Context, usuarioID int64, periodo string) error {
	mes, err := s.mesRepo.FindOrCreate(ctx, usuarioID, periodo)
	if err != nil {
		return err
	}
	if mes.Estado == "cerrado" {
		return nil
	}
	fijos, err := s.costoFijoRepo.FindActivos(ctx, usuarioID)
	if err != nil {
		return err
	}
	return s.costoFijoRepo.CreateTransaccionesFromFijos(ctx, usuarioID, periodo, fijos)
}

// Balance devuelve el mes (por ID o el actual) con sus transacciones y los
// totales recalculados sobre la marcha a partir de ellas, para que el balance
// siempre refleje la realidad sin depender de los valores persistidos (que solo
// se actualizan al cerrar o recalcular). Meses cerrados no se modifican.
func (s *MesService) Balance(ctx context.Context, usuarioID, mesID int64) (*model.Mes, []model.Transaccion, error) {
	var mes *model.Mes
	var err error
	if mesID > 0 {
		mes, err = s.mesRepo.FindByID(ctx, mesID, usuarioID)
	} else {
		mes, err = s.mesRepo.FindOrCreate(ctx, usuarioID, time.Now().Format("2006-01"))
	}
	if err != nil {
		return nil, nil, err
	}
	if err := s.SyncFijosPeriodo(ctx, usuarioID, mes.Periodo); err != nil {
		return nil, nil, err
	}
	transacciones, err := s.transaccionRepo.FindByPeriodo(ctx, usuarioID, mes.Periodo)
	if err != nil {
		return nil, nil, err
	}
	ingresos, egresos := calcularTotales(transacciones)
	mes.IngresosTotal = ingresos
	mes.EgresosTotal = egresos
	mes.Superavit = ingresos - egresos
	if ingresos > 0 {
		tasa := (mes.Superavit / ingresos) * 100
		mes.TasaAhorro = &tasa
	} else {
		mes.TasaAhorro = nil
	}
	return mes, transacciones, nil
}
