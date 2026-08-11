package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"finanzas_personales/internal/model"
)

type DeudaRepo struct {
	db *sql.DB
}

func NewDeudaRepo(db *sql.DB) *DeudaRepo {
	return &DeudaRepo{db: db}
}

func (r *DeudaRepo) Create(ctx context.Context, d *model.Deuda) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO deudas (usuario_id, tipo, entidad, descripcion, monto_total, categoria_id, medio_pago, proximo_vencimiento)
		 VALUES (?,?,?,?,?,?,?,?)`,
		d.UsuarioID, d.Tipo, d.Entidad, d.Descripcion, d.MontoTotal, nullInt64(d.CategoriaID), d.MedioPago, nullString(d.ProximoVencimiento),
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	d.ID = id
	d.CreatedAt = time.Now()
	return nil
}

func (r *DeudaRepo) FindByID(ctx context.Context, id, usuarioID int64) (*model.Deuda, error) {
	d := &model.Deuda{}
	var venc sql.NullTime
	var cat sql.NullInt64
	err := r.db.QueryRowContext(ctx,
		`SELECT id, usuario_id, tipo, entidad, descripcion, monto_total, categoria_id, medio_pago, proximo_vencimiento, estado, created_at
		 FROM deudas WHERE id = ? AND usuario_id = ?`, id, usuarioID,
	).Scan(&d.ID, &d.UsuarioID, &d.Tipo, &d.Entidad, &d.Descripcion, &d.MontoTotal, &cat, &d.MedioPago, &venc, &d.Estado, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	d.CategoriaID = cat.Int64
	d.ProximoVencimiento = formatFecha(venc)
	return d, nil
}

func (r *DeudaRepo) FindByUsuarioID(ctx context.Context, usuarioID int64) ([]model.Deuda, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, usuario_id, tipo, entidad, descripcion, monto_total, categoria_id, medio_pago, proximo_vencimiento, estado, created_at
		 FROM deudas WHERE usuario_id = ?
		 ORDER BY created_at DESC`, usuarioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDeudas(rows)
}

func (r *DeudaRepo) Update(ctx context.Context, d *model.Deuda) error {
	tag, err := r.db.ExecContext(ctx,
		`UPDATE deudas SET tipo=?, entidad=?, descripcion=?, monto_total=?, categoria_id=?, medio_pago=?, proximo_vencimiento=?
		 WHERE id=? AND usuario_id=?`,
		d.Tipo, d.Entidad, d.Descripcion, d.MontoTotal, nullInt64(d.CategoriaID), d.MedioPago, nullString(d.ProximoVencimiento), d.ID, d.UsuarioID)
	if err != nil {
		return err
	}
	affected, err := tag.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return model.ErrNotFound
	}
	return nil
}

func (r *DeudaRepo) Delete(ctx context.Context, id, usuarioID int64) error {
	tag, err := r.db.ExecContext(ctx,
		`DELETE FROM deudas WHERE id=? AND usuario_id=?`, id, usuarioID)
	if err != nil {
		return err
	}
	affected, err := tag.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return model.ErrNotFound
	}
	return nil
}

// FindByRango devuelve deudas registradas (created_at) entre dos fechas
// (inclusive), para la ventana de "últimos movimientos" del balance general.
// Las deudas pagadas se excluyen: al pagarlas pasan a existir como egreso.
func (r *DeudaRepo) FindByRango(ctx context.Context, usuarioID int64, desde, hasta string) ([]model.Deuda, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, usuario_id, tipo, entidad, descripcion, monto_total, categoria_id, medio_pago, proximo_vencimiento, estado, created_at
		 FROM deudas WHERE usuario_id = ? AND estado != 'pagada' AND DATE(created_at) BETWEEN ? AND ?
		 ORDER BY created_at DESC`, usuarioID, desde, hasta)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDeudas(rows)
}

// SumMontoTotal devuelve el total de pasivos del usuario: la suma de los
// montos totales de sus deudas pendientes. Las pagadas dejaron de ser pasivos
// porque fueron registradas como egreso.
func (r *DeudaRepo) SumMontoTotal(ctx context.Context, usuarioID int64) (float64, error) {
	var sum float64
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(monto_total), 0) FROM deudas WHERE usuario_id = ? AND estado != 'pagada'`,
		usuarioID,
	).Scan(&sum)
	if err != nil {
		return 0, err
	}
	return sum, nil
}

// MarcarPagada marca una deuda como pagada, siempre que su estado actual sea
// "pendiente". No encontrada o ya pagada -> ErrNotFound.
func (r *DeudaRepo) MarcarPagada(ctx context.Context, id, usuarioID int64) error {
	tag, err := r.db.ExecContext(ctx,
		`UPDATE deudas SET estado = 'pagada'
		 WHERE id=? AND usuario_id=? AND estado='pendiente'`, id, usuarioID)
	if err != nil {
		return err
	}
	affected, err := tag.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return model.ErrNotFound
	}
	return nil
}

func scanDeudas(rows *sql.Rows) ([]model.Deuda, error) {
	var ds []model.Deuda
	for rows.Next() {
		var d model.Deuda
		var venc sql.NullTime
		var cat sql.NullInt64
		if err := rows.Scan(&d.ID, &d.UsuarioID, &d.Tipo, &d.Entidad, &d.Descripcion, &d.MontoTotal, &cat, &d.MedioPago, &venc, &d.Estado, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.CategoriaID = cat.Int64
		d.ProximoVencimiento = formatFecha(venc)
		ds = append(ds, d)
	}
	return ds, rows.Err()
}

// nullString convierte una cadena vacía en NULL para columnas opcionales.
func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// formatFecha devuelve una fecha DATE en formato YYYY-MM-DD, o "" si es nula.
func formatFecha(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format("2006-01-02")
}

// nullInt64 convierte 0 en NULL para columnas opcionales de tipo id.
func nullInt64(id int64) interface{} {
	if id == 0 {
		return nil
	}
	return id
}
