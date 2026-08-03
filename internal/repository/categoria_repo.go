package repository

import (
	"context"
	"database/sql"

	"administracion-financiera/internal/model"
)

type CategoriaRepo struct {
	db *sql.DB
}

func NewCategoriaRepo(db *sql.DB) *CategoriaRepo {
	return &CategoriaRepo{db: db}
}

func (r *CategoriaRepo) FindAll(ctx context.Context, usuarioID int64) ([]model.Categoria, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias
		 WHERE es_personalizada = FALSE OR usuario_id = ?
		 ORDER BY tipo, nombre`, usuarioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cats []model.Categoria
	for rows.Next() {
		var c model.Categoria
		if err := rows.Scan(&c.ID, &c.Nombre, &c.Tipo, &c.Icono, &c.EsPersonalizada, &c.UsuarioID, &c.CreatedAt); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

func (r *CategoriaRepo) FindByID(ctx context.Context, id int64) (*model.Categoria, error) {
	c := &model.Categoria{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, nombre, tipo, icono, es_personalizada, usuario_id, created_at
		 FROM categorias WHERE id = ?`, id,
	).Scan(&c.ID, &c.Nombre, &c.Tipo, &c.Icono, &c.EsPersonalizada, &c.UsuarioID, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}
