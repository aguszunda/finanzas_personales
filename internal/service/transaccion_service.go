package service

import (
	"context"
	"time"

	"finanzas_personales/internal/model"
	"finanzas_personales/internal/repository"
)

type TransaccionService struct {
	transaccionRepo *repository.TransaccionRepo
	mesRepo         *repository.MesRepo
}

func NewTransaccionService(tr *repository.TransaccionRepo, mr *repository.MesRepo) *TransaccionService {
	return &TransaccionService{transaccionRepo: tr, mesRepo: mr}
}

type CreateTransaccionInput struct {
	Tipo        string  `json:"tipo"`
	Monto       float64 `json:"monto"`
	Fecha       string  `json:"fecha"`
	CategoriaID int64   `json:"categoria_id"`
	Descripcion string  `json:"descripcion"`
	MedioPago   string  `json:"medio_pago"`
	EsFijo      bool    `json:"es_fijo"`
	CuotasTotal *int    `json:"cuotas_total"`
	CuotaActual *int    `json:"cuota_actual"`
}

func (s *TransaccionService) Create(ctx context.Context, usuarioID int64, input CreateTransaccionInput) (*model.Transaccion, error) {
	if input.Monto <= 0 {
		return nil, model.ErrInvalidInput
	}
	if input.Tipo != "ingreso" && input.Tipo != "egreso" {
		return nil, model.ErrInvalidInput
	}
	if input.Fecha == "" {
		input.Fecha = time.Now().Format("2006-01-02")
	}
	periodo := input.Fecha[:7]
	mes, err := s.mesRepo.FindOrCreate(ctx, usuarioID, periodo)
	if err != nil {
		return nil, err
	}
	if mes.Estado == "cerrado" {
		return nil, model.ErrMesCerrado
	}
	t := &model.Transaccion{
		UsuarioID:   usuarioID,
		Tipo:        input.Tipo,
		Monto:       input.Monto,
		Fecha:       input.Fecha,
		CategoriaID: input.CategoriaID,
		Descripcion: input.Descripcion,
		MedioPago:   input.MedioPago,
		EsFijo:      input.EsFijo,
		CuotasTotal: input.CuotasTotal,
		CuotaActual: input.CuotaActual,
		Estado:      "confirmado",
		MesID:       &mes.ID,
	}
	if err := s.transaccionRepo.Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TransaccionService) GetByID(ctx context.Context, id, usuarioID int64) (*model.Transaccion, error) {
	return s.transaccionRepo.FindByID(ctx, id, usuarioID)
}

func (s *TransaccionService) List(ctx context.Context, usuarioID int64, limit, offset int) ([]model.Transaccion, error) {
	return s.transaccionRepo.FindByUsuarioID(ctx, usuarioID, limit, offset)
}

func (s *TransaccionService) ListByPeriodo(ctx context.Context, usuarioID int64, periodo string) ([]model.Transaccion, error) {
	return s.transaccionRepo.FindByPeriodo(ctx, usuarioID, periodo)
}

func (s *TransaccionService) Update(ctx context.Context, usuarioID int64, id int64, input CreateTransaccionInput) (*model.Transaccion, error) {
	if input.Monto <= 0 {
		return nil, model.ErrInvalidInput
	}
	if input.Tipo != "ingreso" && input.Tipo != "egreso" {
		return nil, model.ErrInvalidInput
	}
	existing, err := s.transaccionRepo.FindByID(ctx, id, usuarioID)
	if err != nil {
		return nil, err
	}
	if existing.MesID != nil {
		mes, err := s.mesRepo.FindByID(ctx, *existing.MesID, usuarioID)
		if err == nil && mes.Estado == "cerrado" {
			return nil, model.ErrMesCerrado
		}
	}
	existing.Tipo = input.Tipo
	existing.Monto = input.Monto
	if input.Fecha != "" {
		existing.Fecha = input.Fecha
	}
	existing.CategoriaID = input.CategoriaID
	existing.Descripcion = input.Descripcion
	existing.MedioPago = input.MedioPago
	existing.EsFijo = input.EsFijo
	existing.CuotasTotal = input.CuotasTotal
	existing.CuotaActual = input.CuotaActual
	if err := s.transaccionRepo.Update(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *TransaccionService) Delete(ctx context.Context, usuarioID, id int64) error {
	existing, err := s.transaccionRepo.FindByID(ctx, id, usuarioID)
	if err != nil {
		return err
	}
	if existing.MesID != nil {
		mes, err := s.mesRepo.FindByID(ctx, *existing.MesID, usuarioID)
		if err == nil && mes.Estado == "cerrado" {
			return model.ErrMesCerrado
		}
	}
	return s.transaccionRepo.Delete(ctx, id, usuarioID)
}
