package service

import (
	"context"

	"finanzas_personales/internal/model"
	"finanzas_personales/internal/repository"
)

type DeudaService struct {
	repo *repository.DeudaRepo
}

func NewDeudaService(r *repository.DeudaRepo) *DeudaService {
	return &DeudaService{repo: r}
}

type CreateDeudaInput struct {
	Tipo               string  `json:"tipo"`
	Entidad            string  `json:"entidad"`
	Descripcion        string  `json:"descripcion"`
	MontoTotal         float64 `json:"monto_total"`
	SaldoPendiente     float64 `json:"saldo_pendiente"`
	TasaInteres        float64 `json:"tasa_interes"`
	ProximoVencimiento string  `json:"proximo_vencimiento"`
}

var tiposDeudaValidos = map[string]bool{
	"tarjeta_credito": true,
	"prestamo":        true,
	"hipoteca":        true,
	"personal":        true,
	"otro":            true,
}

func (s *DeudaService) Create(ctx context.Context, usuarioID int64, input CreateDeudaInput) (*model.Deuda, error) {
	if input.Entidad == "" || input.MontoTotal <= 0 || input.SaldoPendiente < 0 {
		return nil, model.ErrInvalidInput
	}
	if input.SaldoPendiente > input.MontoTotal {
		return nil, model.ErrInvalidInput
	}
	if input.Tipo == "" {
		input.Tipo = "otro"
	}
	if !tiposDeudaValidos[input.Tipo] {
		return nil, model.ErrInvalidInput
	}
	d := &model.Deuda{
		UsuarioID:          usuarioID,
		Tipo:               input.Tipo,
		Entidad:            input.Entidad,
		Descripcion:        input.Descripcion,
		MontoTotal:         input.MontoTotal,
		SaldoPendiente:     input.SaldoPendiente,
		TasaInteres:        input.TasaInteres,
		ProximoVencimiento: input.ProximoVencimiento,
	}
	if err := s.repo.Create(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DeudaService) List(ctx context.Context, usuarioID int64) ([]model.Deuda, error) {
	return s.repo.FindByUsuarioID(ctx, usuarioID)
}

func (s *DeudaService) GetByID(ctx context.Context, id, usuarioID int64) (*model.Deuda, error) {
	return s.repo.FindByID(ctx, id, usuarioID)
}

func (s *DeudaService) Update(ctx context.Context, usuarioID, id int64, input CreateDeudaInput) (*model.Deuda, error) {
	if input.Entidad == "" || input.MontoTotal <= 0 || input.SaldoPendiente < 0 {
		return nil, model.ErrInvalidInput
	}
	if input.SaldoPendiente > input.MontoTotal {
		return nil, model.ErrInvalidInput
	}
	if input.Tipo == "" {
		input.Tipo = "otro"
	}
	if !tiposDeudaValidos[input.Tipo] {
		return nil, model.ErrInvalidInput
	}
	d, err := s.repo.FindByID(ctx, id, usuarioID)
	if err != nil {
		return nil, err
	}
	d.Tipo = input.Tipo
	d.Entidad = input.Entidad
	d.Descripcion = input.Descripcion
	d.MontoTotal = input.MontoTotal
	d.SaldoPendiente = input.SaldoPendiente
	d.TasaInteres = input.TasaInteres
	d.ProximoVencimiento = input.ProximoVencimiento
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DeudaService) Delete(ctx context.Context, usuarioID, id int64) error {
	return s.repo.Delete(ctx, id, usuarioID)
}
