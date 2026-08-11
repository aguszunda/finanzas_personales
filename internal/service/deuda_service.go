package service

import (
	"context"
	"time"

	"finanzas_personales/internal/model"
	"finanzas_personales/internal/repository"
)

type DeudaService struct {
	repo     *repository.DeudaRepo
	catRepo  *repository.CategoriaRepo
	transSvc *TransaccionService
}

func NewDeudaService(r *repository.DeudaRepo, cr *repository.CategoriaRepo, ts *TransaccionService) *DeudaService {
	return &DeudaService{repo: r, catRepo: cr, transSvc: ts}
}

type CreateDeudaInput struct {
	Tipo               string  `json:"tipo"`
	Entidad            string  `json:"entidad"`
	Descripcion        string  `json:"descripcion"`
	MontoTotal         float64 `json:"monto_total"`
	CategoriaID        int64   `json:"categoria_id"` // categoría (egreso) default del pago
	MedioPago          string  `json:"medio_pago"`
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
	if input.Entidad == "" || input.MontoTotal <= 0 {
		return nil, model.ErrInvalidInput
	}
	if input.Tipo == "" {
		input.Tipo = "otro"
	}
	if !tiposDeudaValidos[input.Tipo] {
		return nil, model.ErrInvalidInput
	}
	if input.CategoriaID != 0 {
		if err := s.validarCategoriaEgreso(ctx, usuarioID, input.CategoriaID); err != nil {
			return nil, err
		}
	}
	d := &model.Deuda{
		UsuarioID:          usuarioID,
		Tipo:               input.Tipo,
		Entidad:            input.Entidad,
		Descripcion:        input.Descripcion,
		MontoTotal:         input.MontoTotal,
		CategoriaID:        input.CategoriaID,
		MedioPago:          input.MedioPago,
		ProximoVencimiento: input.ProximoVencimiento,
		Estado:             "pendiente",
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
	if input.Entidad == "" || input.MontoTotal <= 0 {
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
	if input.CategoriaID != 0 {
		if err := s.validarCategoriaEgreso(ctx, usuarioID, input.CategoriaID); err != nil {
			return nil, err
		}
	}
	d.CategoriaID = input.CategoriaID
	d.MedioPago = input.MedioPago
	d.ProximoVencimiento = input.ProximoVencimiento
	if err := s.repo.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *DeudaService) Delete(ctx context.Context, usuarioID, id int64) error {
	return s.repo.Delete(ctx, id, usuarioID)
}

// MarcarPagada convierte una deuda en egreso: registra una transacción de
// egreso por el monto total y luego marca la deuda como "pagada". Así deja de
// contar como pasivo. Si no se manda categoría, se usa la elegida al crear la
// deuda (input.CategoriaID == 0 → d.CategoriaID); si tampoco tiene, falla.
// Igual con la forma de pago: si medioPago está vacío se usa el de la deuda,
// y si tampoco tiene, el egreso queda sin forma de pago. La fecha es opcional:
// vacía → hoy, respetando el guard de mes cerrado de TransaccionService.Create
// (si el mes elegido está cerrado devuelve ErrMesCerrado). Rechaza categorías
// que no existan o no sean de tipo egreso, y deudas ya pagadas.
func (s *DeudaService) MarcarPagada(ctx context.Context, usuarioID, id, categoriaID int64, fecha, medioPago string) (*model.Deuda, error) {
	d, err := s.repo.FindByID(ctx, id, usuarioID)
	if err != nil {
		return nil, err
	}
	if d.Estado == "pagada" {
		return nil, model.ErrInvalidInput
	}
	if fecha != "" {
		if _, err := time.Parse("2006-01-02", fecha); err != nil {
			return nil, model.ErrInvalidInput
		}
	}
	if categoriaID == 0 {
		categoriaID = d.CategoriaID
	}
	if err := s.validarCategoriaEgreso(ctx, usuarioID, categoriaID); err != nil {
		return nil, err
	}
	if medioPago == "" {
		medioPago = d.MedioPago
	}
	if _, err := s.transSvc.Create(ctx, usuarioID, CreateTransaccionInput{
		Tipo:        "egreso",
		Monto:       d.MontoTotal,
		Fecha:       fecha, // vacía → hoy (default de Create)
		CategoriaID: categoriaID,
		MedioPago:   medioPago,
		Descripcion: "Pago deuda: " + d.Entidad,
	}); err != nil {
		return nil, err
	}
	if err := s.repo.MarcarPagada(ctx, id, usuarioID); err != nil {
		return nil, err
	}
	d.Estado = "pagada"
	return d, nil
}

func (s *DeudaService) validarCategoriaEgreso(ctx context.Context, usuarioID, categoriaID int64) error {
	cats, err := s.catRepo.FindAll(ctx, usuarioID)
	if err != nil {
		return err
	}
	for _, c := range cats {
		if c.ID == categoriaID && c.Tipo == "egreso" {
			return nil
		}
	}
	return model.ErrInvalidInput
}
