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
		`SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado
		 FROM usuarios WHERE email = ?`, email,
	).Scan(&u.ID, &u.Nombre, &u.Email, &u.PasswordHash, &u.MonedaDefault, &u.CreatedAt, &u.EmailVerificado)
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
		`SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado
		 FROM usuarios WHERE id = ?`, id,
	).Scan(&u.ID, &u.Nombre, &u.Email, &u.PasswordHash, &u.MonedaDefault, &u.CreatedAt, &u.EmailVerificado)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

// GuardarTokenVerificacion persiste el hash del token de verificación y su
// vencimiento. Regenerar el token invalida el enlace anterior (reenvío).
func (r *UsuarioRepo) GuardarTokenVerificacion(ctx context.Context, usuarioID int64, tokenHash string, expira time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE usuarios SET token_verificacion = ?, token_expiracion = ? WHERE id = ?`,
		tokenHash, expira, usuarioID,
	)
	return err
}

// FindByTokenVerificacion busca al usuario por el hash del token. Devuelve
// ErrNotFound si el token no existe (inválido o ya consumido).
func (r *UsuarioRepo) FindByTokenVerificacion(ctx context.Context, tokenHash string) (*model.Usuario, error) {
	u := &model.Usuario{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado, token_expiracion
		 FROM usuarios WHERE token_verificacion = ?`, tokenHash,
	).Scan(&u.ID, &u.Nombre, &u.Email, &u.PasswordHash, &u.MonedaDefault, &u.CreatedAt, &u.EmailVerificado, &u.TokenExpiracion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

// MarcarVerificado confirma el email. El hash del token se conserva a propósito:
// un segundo clic sobre el mismo enlace es idempotente (service lo detecta vía
// email_verificado), mientras que la expiración se limpia.
func (r *UsuarioRepo) MarcarVerificado(ctx context.Context, usuarioID int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE usuarios SET email_verificado = TRUE, token_expiracion = NULL WHERE id = ?`,
		usuarioID,
	)
	return err
}

// GuardarTokenPasswordReset persiste el hash del token de reseteo de contraseña
// y su vencimiento. Regenerar el token invalida el enlace anterior.
func (r *UsuarioRepo) GuardarTokenPasswordReset(ctx context.Context, usuarioID int64, tokenHash string, expira time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE usuarios SET password_reset_token = ?, password_reset_expiracion = ? WHERE id = ?`,
		tokenHash, expira, usuarioID,
	)
	return err
}

// FindByPasswordResetToken busca al usuario por el hash del token de reseteo.
// Devuelve ErrNotFound si el token no existe (inválido o ya consumido).
func (r *UsuarioRepo) FindByPasswordResetToken(ctx context.Context, tokenHash string) (*model.Usuario, error) {
	u := &model.Usuario{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado, password_reset_expiracion
		 FROM usuarios WHERE password_reset_token = ?`, tokenHash,
	).Scan(&u.ID, &u.Nombre, &u.Email, &u.PasswordHash, &u.MonedaDefault, &u.CreatedAt, &u.EmailVerificado, &u.PasswordResetExpiracion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

// ActualizarPassword cambia la contraseña del usuario y limpia el token de reseteo.
func (r *UsuarioRepo) ActualizarPassword(ctx context.Context, usuarioID int64, passwordHash string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE usuarios SET password_hash = ?, password_reset_token = NULL, password_reset_expiracion = NULL WHERE id = ?`,
		passwordHash, usuarioID,
	)
	return err
}

// ActualizarNombre cambia el nombre del usuario.
func (r *UsuarioRepo) ActualizarNombre(ctx context.Context, usuarioID int64, nombre string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE usuarios SET nombre = ? WHERE id = ?`,
		nombre, usuarioID,
	)
	return err
}
