package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"finanzas_personales/internal/model"
)

type CostoFijoRepo struct {
	db *sql.DB
}

func NewCostoFijoRepo(db *sql.DB) *CostoFijoRepo {
	return &CostoFijoRepo{db: db}
}

func (r *CostoFijoRepo) Create(ctx context.Context, c *model.CostoFijo) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO costos_fijos (usuario_id, categoria_id, descripcion, monto_estimado, dia_vencimiento, activo, tipo_periodo)
		 VALUES (?,?,?,?,?,?,?)`,
		c.UsuarioID, c.CategoriaID, c.Descripcion, c.MontoEstimado, c.DiaVencimiento, c.Activo, c.TipoPeriodo,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	c.ID = id
	c.CreatedAt = time.Now()
	return nil
}

func (r *CostoFijoRepo) FindByUsuarioID(ctx context.Context, usuarioID int64) ([]model.CostoFijo, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT cf.id, cf.usuario_id, cf.categoria_id, c.nombre, cf.descripcion, cf.monto_estimado, cf.dia_vencimiento, cf.activo, cf.tipo_periodo, cf.created_at
		 FROM costos_fijos cf JOIN categorias c ON c.id = cf.categoria_id
		 WHERE cf.usuario_id = ?
		 ORDER BY cf.dia_vencimiento, cf.descripcion`, usuarioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCostosFijos(rows)
}

func (r *CostoFijoRepo) FindActivos(ctx context.Context, usuarioID int64) ([]model.CostoFijo, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT cf.id, cf.usuario_id, cf.categoria_id, c.nombre, cf.descripcion, cf.monto_estimado, cf.dia_vencimiento, cf.activo, cf.tipo_periodo, cf.created_at
		 FROM costos_fijos cf JOIN categorias c ON c.id = cf.categoria_id
		 WHERE cf.usuario_id = ? AND cf.activo = TRUE
		 ORDER BY cf.dia_vencimiento, cf.descripcion`, usuarioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCostosFijos(rows)
}

func (r *CostoFijoRepo) FindByID(ctx context.Context, id, usuarioID int64) (*model.CostoFijo, error) {
	c := &model.CostoFijo{}
	err := r.db.QueryRowContext(ctx,
		`SELECT cf.id, cf.usuario_id, cf.categoria_id, c.nombre, cf.descripcion, cf.monto_estimado, cf.dia_vencimiento, cf.activo, cf.tipo_periodo, cf.created_at
		 FROM costos_fijos cf JOIN categorias c ON c.id = cf.categoria_id
		 WHERE cf.id = ? AND cf.usuario_id = ?`, id, usuarioID,
	).Scan(&c.ID, &c.UsuarioID, &c.CategoriaID, &c.Categoria, &c.Descripcion, &c.MontoEstimado, &c.DiaVencimiento, &c.Activo, &c.TipoPeriodo, &c.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return c, nil
}

func (r *CostoFijoRepo) Update(ctx context.Context, c *model.CostoFijo) error {
	tag, err := r.db.ExecContext(ctx,
		`UPDATE costos_fijos SET categoria_id=?, descripcion=?, monto_estimado=?, dia_vencimiento=?, activo=?, tipo_periodo=?
		 WHERE id=? AND usuario_id=?`,
		c.CategoriaID, c.Descripcion, c.MontoEstimado, c.DiaVencimiento, c.Activo, c.TipoPeriodo, c.ID, c.UsuarioID)
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

func (r *CostoFijoRepo) Delete(ctx context.Context, id, usuarioID int64) error {
	tag, err := r.db.ExecContext(ctx,
		`DELETE FROM costos_fijos WHERE id=? AND usuario_id=?`, id, usuarioID)
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

// PrecargarEnPeriodo materializa un costo fijo activo como transacción
// "pendiente" en el período indicado. Es idempotente: si ya existe una
// transacción pendiente del mismo costo en ese período, no inserta duplicados.
func (r *CostoFijoRepo) PrecargarEnPeriodo(ctx context.Context, usuarioID int64, periodo string, f model.CostoFijo) error {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM transacciones
		 WHERE usuario_id = ? AND es_fijo = TRUE AND estado = 'pendiente'
		   AND categoria_id = ? AND descripcion = ?
		   AND fecha >= ? AND fecha <= ?`,
		usuarioID, f.CategoriaID, f.Descripcion, periodo+"-01", periodo+"-31").Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO transacciones (usuario_id, tipo, monto, fecha, categoria_id, descripcion, medio_pago, es_fijo, estado)
		 VALUES (?, 'egreso', ?, ?, ?, ?, 'debito', TRUE, 'pendiente')`,
		usuarioID, f.MontoEstimado, periodo+"-01", f.CategoriaID, f.Descripcion)
	return err
}

func (r *CostoFijoRepo) CreateTransaccionesFromFijos(ctx context.Context, usuarioID int64, periodo string, fijos []model.CostoFijo) error {
	for _, f := range fijos {
		if err := r.PrecargarEnPeriodo(ctx, usuarioID, periodo, f); err != nil {
			return err
		}
	}
	return nil
}

func scanCostosFijos(rows *sql.Rows) ([]model.CostoFijo, error) {
	var cs []model.CostoFijo
	for rows.Next() {
		var c model.CostoFijo
		if err := rows.Scan(&c.ID, &c.UsuarioID, &c.CategoriaID, &c.Categoria, &c.Descripcion, &c.MontoEstimado, &c.DiaVencimiento, &c.Activo, &c.TipoPeriodo, &c.CreatedAt); err != nil {
			return nil, err
		}
		cs = append(cs, c)
	}
	return cs, rows.Err()
}
