package service

import (
	"context"

	"finanzas_personales/internal/model"
	"finanzas_personales/internal/repository"
)

type CostoFijoService struct {
	repo *repository.CostoFijoRepo
}

func NewCostoFijoService(r *repository.CostoFijoRepo) *CostoFijoService {
	return &CostoFijoService{repo: r}
}

type CreateCostoFijoInput struct {
	CategoriaID    int64   `json:"categoria_id"`
	Descripcion    string  `json:"descripcion"`
	MontoEstimado  float64 `json:"monto_estimado"`
	DiaVencimiento int     `json:"dia_vencimiento"`
	TipoPeriodo    string  `json:"tipo_periodo"`
}

func (s *CostoFijoService) Create(ctx context.Context, usuarioID int64, input CreateCostoFijoInput) (*model.CostoFijo, error) {
	if input.Descripcion == "" || input.MontoEstimado <= 0 {
		return nil, model.ErrInvalidInput
	}
	if input.TipoPeriodo == "" {
		input.TipoPeriodo = "mensual"
	}
	if input.DiaVencimiento < 1 || input.DiaVencimiento > 31 {
		return nil, model.ErrInvalidInput
	}
	c := &model.CostoFijo{
		UsuarioID:      usuarioID,
		CategoriaID:    input.CategoriaID,
		Descripcion:    input.Descripcion,
		MontoEstimado:  input.MontoEstimado,
		DiaVencimiento: input.DiaVencimiento,
		Activo:         true,
		TipoPeriodo:    input.TipoPeriodo,
	}
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *CostoFijoService) List(ctx context.Context, usuarioID int64) ([]model.CostoFijo, error) {
	return s.repo.FindByUsuarioID(ctx, usuarioID)
}

func (s *CostoFijoService) ListActivos(ctx context.Context, usuarioID int64) ([]model.CostoFijo, error) {
	return s.repo.FindActivos(ctx, usuarioID)
}

func (s *CostoFijoService) GetByID(ctx context.Context, id, usuarioID int64) (*model.CostoFijo, error) {
	return s.repo.FindByID(ctx, id, usuarioID)
}

func (s *CostoFijoService) Update(ctx context.Context, usuarioID, id int64, input CreateCostoFijoInput) (*model.CostoFijo, error) {
	c, err := s.repo.FindByID(ctx, id, usuarioID)
	if err != nil {
		return nil, err
	}
	c.CategoriaID = input.CategoriaID
	c.Descripcion = input.Descripcion
	c.MontoEstimado = input.MontoEstimado
	c.DiaVencimiento = input.DiaVencimiento
	c.TipoPeriodo = input.TipoPeriodo
	if err := s.repo.Update(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *CostoFijoService) ToggleActivo(ctx context.Context, usuarioID, id int64) (*model.CostoFijo, error) {
	c, err := s.repo.FindByID(ctx, id, usuarioID)
	if err != nil {
		return nil, err
	}
	c.Activo = !c.Activo
	if err := s.repo.Update(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *CostoFijoService) Delete(ctx context.Context, usuarioID, id int64) error {
	return s.repo.Delete(ctx, id, usuarioID)
}
