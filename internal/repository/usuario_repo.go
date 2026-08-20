package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"optipay/internal/model"

	"github.com/go-sql-driver/mysql"
)

type UsuarioRepo struct {
	db *sql.DB
}

func NewUsuarioRepo(db *sql.DB) *UsuarioRepo {
	return &UsuarioRepo{db: db}
}

func (r *UsuarioRepo) Create(ctx context.Context, u *model.Usuario) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO usuarios (nombre, email, password_hash, moneda_default)
		 VALUES (?, ?, ?, ?)`,
		u.Nombre, u.Email, u.PasswordHash, u.MonedaDefault,
	)
	if err != nil {
		var myErr *mysql.MySQLError
		if errors.As(err, &myErr) && myErr.Number == 1062 {
			return model.ErrEmailExiste
		}
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	u.ID = id
	u.CreatedAt = time.Now()
	return nil
}

func (r *UsuarioRepo) FindByEmail(ctx context.Context, email string) (*model.Usuario, error) {
	u := &model.Usuario{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, nombre, email, password_hash, moneda_default, created_at
		 FROM usuarios WHERE email = ?`, email,
	).Scan(&u.ID, &u.Nombre, &u.Email, &u.PasswordHash, &u.MonedaDefault, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

func (r *UsuarioRepo) FindByID(ctx context.Context, id int64) (*model.Usuario, error) {
	u := &model.Usuario{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, nombre, email, password_hash, moneda_default, created_at
		 FROM usuarios WHERE id = ?`, id,
	).Scan(&u.ID, &u.Nombre, &u.Email, &u.PasswordHash, &u.MonedaDefault, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return u, nil
}
