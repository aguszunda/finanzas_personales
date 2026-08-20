package repository

import (
	"context"
	"database/sql"
	"errors"

	"optipay/internal/model"
)

type MesRepo struct {
	db *sql.DB
}

func NewMesRepo(db *sql.DB) *MesRepo {
	return &MesRepo{db: db}
}

func (r *MesRepo) FindByUsuarioID(ctx context.Context, usuarioID int64) ([]model.Mes, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? ORDER BY periodo DESC`, usuarioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMeses(rows)
}

func (r *MesRepo) FindByID(ctx context.Context, id, usuarioID int64) (*model.Mes, error) {
	m := &model.Mes{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE id = ? AND usuario_id = ?`, id, usuarioID,
	).Scan(&m.ID, &m.UsuarioID, &m.Periodo, &m.Estado, &m.IngresosTotal, &m.EgresosTotal, &m.Superavit, &m.TasaAhorro, &m.AhorroAcumulado, &m.PasivosTotal, &m.Patrimonio, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return m, nil
}

func (r *MesRepo) FindByPeriodo(ctx context.Context, usuarioID int64, periodo string) (*model.Mes, error) {
	m := &model.Mes{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, usuario_id, periodo, estado, ingresos_total, egresos_total, superavit, tasa_ahorro, ahorro_acumulado, pasivos_total, patrimonio, created_at
		 FROM meses WHERE usuario_id = ? AND periodo = ?`, usuarioID, periodo,
	).Scan(&m.ID, &m.UsuarioID, &m.Periodo, &m.Estado, &m.IngresosTotal, &m.EgresosTotal, &m.Superavit, &m.TasaAhorro, &m.AhorroAcumulado, &m.PasivosTotal, &m.Patrimonio, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return m, nil
}

func (r *MesRepo) FindOrCreate(ctx context.Context, usuarioID int64, periodo string) (*model.Mes, error) {
	m, err := r.FindByPeriodo(ctx, usuarioID, periodo)
	if err == nil {
		return m, nil
	}
	if !errors.Is(err, model.ErrNotFound) {
		return nil, err
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO meses (usuario_id, periodo, estado)
		 VALUES (?, ?, 'abierto')
		 ON DUPLICATE KEY UPDATE estado = VALUES(estado)`,
		usuarioID, periodo,
	)
	if err != nil {
		return nil, err
	}
	return r.FindByPeriodo(ctx, usuarioID, periodo)
}

func (r *MesRepo) Update(ctx context.Context, m *model.Mes) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE meses SET estado=?, ingresos_total=?, egresos_total=?, superavit=?, tasa_ahorro=?, ahorro_acumulado=?, pasivos_total=?, patrimonio=?
		 WHERE id=? AND usuario_id=?`,
		m.Estado, m.IngresosTotal, m.EgresosTotal, m.Superavit, m.TasaAhorro, m.AhorroAcumulado, m.PasivosTotal, m.Patrimonio, m.ID, m.UsuarioID)
	return err
}

func (r *MesRepo) Cerrar(ctx context.Context, id, usuarioID int64) error {
	tag, err := r.db.ExecContext(ctx,
		`UPDATE meses SET estado='cerrado' WHERE id=? AND usuario_id=? AND estado='abierto'`, id, usuarioID)
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

// SumSuperavitAnterior devuelve el superávit acumulado de todos los meses
// cerrados estrictamente anteriores al período indicado. Es la base del
// "ahorro acumulado": se le suma el superávit del período actual en memoria.
func (r *MesRepo) SumSuperavitAnterior(ctx context.Context, usuarioID int64, periodo string) (float64, error) {
	var sum float64
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(superavit), 0) FROM meses
		 WHERE usuario_id = ? AND estado = 'cerrado' AND periodo < ?`,
		usuarioID, periodo,
	).Scan(&sum)
	if err != nil {
		return 0, err
	}
	return sum, nil
}

func scanMeses(rows *sql.Rows) ([]model.Mes, error) {
	var ms []model.Mes
	for rows.Next() {
		var m model.Mes
		if err := rows.Scan(&m.ID, &m.UsuarioID, &m.Periodo, &m.Estado, &m.IngresosTotal, &m.EgresosTotal, &m.Superavit, &m.TasaAhorro, &m.AhorroAcumulado, &m.PasivosTotal, &m.Patrimonio, &m.CreatedAt); err != nil {
			return nil, err
		}
		ms = append(ms, m)
	}
	return ms, rows.Err()
}
