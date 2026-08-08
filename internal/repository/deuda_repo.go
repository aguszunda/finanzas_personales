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
		`INSERT INTO deudas (usuario_id, tipo, entidad, descripcion, monto_total, proximo_vencimiento)
		 VALUES (?,?,?,?,?,?)`,
		d.UsuarioID, d.Tipo, d.Entidad, d.Descripcion, d.MontoTotal, nullString(d.ProximoVencimiento),
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
	var venc sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT id, usuario_id, tipo, entidad, descripcion, monto_total, proximo_vencimiento, created_at
		 FROM deudas WHERE id = ? AND usuario_id = ?`, id, usuarioID,
	).Scan(&d.ID, &d.UsuarioID, &d.Tipo, &d.Entidad, &d.Descripcion, &d.MontoTotal, &venc, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	d.ProximoVencimiento = venc.String
	return d, nil
}

func (r *DeudaRepo) FindByUsuarioID(ctx context.Context, usuarioID int64) ([]model.Deuda, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, usuario_id, tipo, entidad, descripcion, monto_total, proximo_vencimiento, created_at
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
		`UPDATE deudas SET tipo=?, entidad=?, descripcion=?, monto_total=?, proximo_vencimiento=?
		 WHERE id=? AND usuario_id=?`,
		d.Tipo, d.Entidad, d.Descripcion, d.MontoTotal, nullString(d.ProximoVencimiento), d.ID, d.UsuarioID)
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

// SumMontoTotal devuelve el total de pasivos del usuario: la suma de los
// montos totales de todas sus deudas.
func (r *DeudaRepo) SumMontoTotal(ctx context.Context, usuarioID int64) (float64, error) {
	var sum float64
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(monto_total), 0) FROM deudas WHERE usuario_id = ?`,
		usuarioID,
	).Scan(&sum)
	if err != nil {
		return 0, err
	}
	return sum, nil
}

func scanDeudas(rows *sql.Rows) ([]model.Deuda, error) {
	var ds []model.Deuda
	for rows.Next() {
		var d model.Deuda
		var venc sql.NullString
		if err := rows.Scan(&d.ID, &d.UsuarioID, &d.Tipo, &d.Entidad, &d.Descripcion, &d.MontoTotal, &venc, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.ProximoVencimiento = venc.String
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
