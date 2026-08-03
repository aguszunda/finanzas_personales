package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"administracion-financiera/internal/model"
)

type TransaccionRepo struct {
	db *sql.DB
}

func NewTransaccionRepo(db *sql.DB) *TransaccionRepo {
	return &TransaccionRepo{db: db}
}

func (r *TransaccionRepo) Create(ctx context.Context, t *model.Transaccion) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO transacciones (usuario_id, tipo, monto, fecha, categoria_id, descripcion, medio_pago, es_fijo, cuotas_total, cuota_actual, estado, mes_id)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.UsuarioID, t.Tipo, t.Monto, t.Fecha, t.CategoriaID,
		t.Descripcion, t.MedioPago, t.EsFijo, t.CuotasTotal, t.CuotaActual,
		t.Estado, t.MesID,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	t.ID = id
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	return nil
}

func (r *TransaccionRepo) FindByID(ctx context.Context, id, usuarioID int64) (*model.Transaccion, error) {
	t := &model.Transaccion{}
	var fecha time.Time
	err := r.db.QueryRowContext(ctx,
		`SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.es_fijo, t.cuotas_total, t.cuota_actual, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.id = ? AND t.usuario_id = ?`, id, usuarioID,
	).Scan(&t.ID, &t.UsuarioID, &t.Tipo, &t.Monto, &fecha, &t.CategoriaID, &t.Categoria, &t.Descripcion, &t.MedioPago, &t.EsFijo, &t.CuotasTotal, &t.CuotaActual, &t.Estado, &t.MesID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	t.Fecha = fecha.Format("2006-01-02")
	return t, nil
}

func (r *TransaccionRepo) FindByUsuarioID(ctx context.Context, usuarioID int64, limit, offset int) ([]model.Transaccion, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.es_fijo, t.cuotas_total, t.cuota_actual, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ?
		 ORDER BY t.fecha DESC, t.created_at DESC
		 LIMIT ? OFFSET ?`, usuarioID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTransacciones(rows)
}

func (r *TransaccionRepo) FindByMesID(ctx context.Context, mesID, usuarioID int64) ([]model.Transaccion, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.es_fijo, t.cuotas_total, t.cuota_actual, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.mes_id = ? AND t.usuario_id = ?
		 ORDER BY t.fecha DESC, t.created_at DESC`, mesID, usuarioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTransacciones(rows)
}

func (r *TransaccionRepo) FindByPeriodo(ctx context.Context, usuarioID int64, periodo string) ([]model.Transaccion, error) {
	start := periodo + "-01"
	end := periodo + "-31"
	rows, err := r.db.QueryContext(ctx,
		`SELECT t.id, t.usuario_id, t.tipo, t.monto, t.fecha, t.categoria_id, c.nombre, t.descripcion, t.medio_pago, t.es_fijo, t.cuotas_total, t.cuota_actual, t.estado, t.mes_id, t.created_at, t.updated_at
		 FROM transacciones t JOIN categorias c ON c.id = t.categoria_id
		 WHERE t.usuario_id = ? AND t.fecha >= ? AND t.fecha <= ?
		 ORDER BY t.fecha DESC, t.created_at DESC`, usuarioID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTransacciones(rows)
}

func (r *TransaccionRepo) Update(ctx context.Context, t *model.Transaccion) error {
	tag, err := r.db.ExecContext(ctx,
		`UPDATE transacciones SET tipo=?, monto=?, fecha=?, categoria_id=?, descripcion=?, medio_pago=?, es_fijo=?, cuotas_total=?, cuota_actual=?, updated_at=NOW()
		 WHERE id=? AND usuario_id=?`,
		t.Tipo, t.Monto, t.Fecha, t.CategoriaID, t.Descripcion, t.MedioPago, t.EsFijo, t.CuotasTotal, t.CuotaActual, t.ID, t.UsuarioID)
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

func (r *TransaccionRepo) Delete(ctx context.Context, id, usuarioID int64) error {
	tag, err := r.db.ExecContext(ctx,
		`DELETE FROM transacciones WHERE id=? AND usuario_id=?`, id, usuarioID)
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

func (r *TransaccionRepo) SumByCategoria(ctx context.Context, usuarioID int64, periodo string) (map[int64]float64, error) {
	start := periodo + "-01"
	end := periodo + "-31"
	rows, err := r.db.QueryContext(ctx,
		`SELECT categoria_id, SUM(monto) FROM transacciones
		 WHERE usuario_id=? AND fecha>=? AND fecha<=? AND tipo='egreso'
		 GROUP BY categoria_id`, usuarioID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64]float64)
	for rows.Next() {
		var catID int64
		var sum float64
		if err := rows.Scan(&catID, &sum); err != nil {
			return nil, err
		}
		result[catID] = sum
	}
	return result, rows.Err()
}

func (r *TransaccionRepo) CreateAjuste(ctx context.Context, usuarioID int64, monto float64, categoriaID int64, descripcion, periodo string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO transacciones (usuario_id, tipo, monto, fecha, categoria_id, descripcion, estado)
		 VALUES (?, 'egreso', ?, ?, ?, ?, 'ajuste')`,
		usuarioID, monto, periodo+"-01", categoriaID, descripcion)
	return err
}

func scanTransacciones(rows *sql.Rows) ([]model.Transaccion, error) {
	var ts []model.Transaccion
	for rows.Next() {
		var t model.Transaccion
		var fecha time.Time
		if err := rows.Scan(&t.ID, &t.UsuarioID, &t.Tipo, &t.Monto, &fecha, &t.CategoriaID, &t.Categoria, &t.Descripcion, &t.MedioPago, &t.EsFijo, &t.CuotasTotal, &t.CuotaActual, &t.Estado, &t.MesID, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Fecha = fecha.Format("2006-01-02")
		ts = append(ts, t)
	}
	return ts, rows.Err()
}
