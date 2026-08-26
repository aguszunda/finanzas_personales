package service

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"optipay/internal/model"
	"optipay/internal/repository"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var mysqlErrorDuplicate = mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}

func parseTestToken(tokenStr string, secret []byte) (bool, int64, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, model.ErrUnauthorized
		}
		return secret, nil
	})
	if err != nil || !token.Valid {
		return false, 0, err
	}
	claims := token.Claims.(jwt.MapClaims)
	sub, ok := claims["sub"].(float64)
	if !ok {
		return false, 0, errors.New("sub claim missing")
	}
	return true, int64(sub), nil
}

const (
	queryInsertUsuario    = "INSERT INTO usuarios (nombre, email, password_hash, moneda_default) VALUES (?, ?, ?, ?)"
	queryFindByEmail      = "SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado FROM usuarios WHERE email = ?"
	queryFindByToken      = "SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado, token_expiracion FROM usuarios WHERE token_verificacion = ?"
	queryGuardarToken     = "UPDATE usuarios SET token_verificacion = ?, token_expiracion = ? WHERE id = ?"
	queryMarcarVerificado = "UPDATE usuarios SET email_verificado = TRUE, token_expiracion = NULL WHERE id = ?"
)

// fakeMailer captura los links enviados para poder afirmar sobre ellos y
// permite simular fallos de SMTP.
type fakeMailer struct {
	links []string
	err   error
}

func (f *fakeMailer) SendVerificacion(_ context.Context, _, _, link string) error {
	if f.err != nil {
		return f.err
	}
	f.links = append(f.links, link)
	return nil
}

func (f *fakeMailer) SendPasswordReset(_ context.Context, _, _, link string) error {
	if f.err != nil {
		return f.err
	}
	f.links = append(f.links, link)
	return nil
}

// authServiceFixture agrupa el servicio con sus dobles de prueba.
type authServiceFixture struct {
	svc    *AuthService
	mock   sqlmock.Sqlmock
	mailer *fakeMailer
}

func newAuthServiceFixture(t *testing.T) *authServiceFixture {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	mailer := &fakeMailer{}
	repo := repository.NewUsuarioRepo(db)
	svc := NewAuthService(repo, []byte("test-secret"), 72*time.Hour, mailer, "http://localhost:8080")
	return &authServiceFixture{svc: svc, mock: mock, mailer: mailer}
}

func newAuthService(t *testing.T) (*AuthService, sqlmock.Sqlmock) {
	t.Helper()
	f := newAuthServiceFixture(t)
	return f.svc, f.mock
}

func TestAuthService_Register_Valid(t *testing.T) {
	f := newAuthServiceFixture(t)
	f.mock.ExpectExec(regexp.QuoteMeta(queryInsertUsuario)).
		WithArgs("Agustin", "a@test.com", sqlmock.AnyArg(), "USD").
		WillReturnResult(sqlmock.NewResult(7, 1))
	f.mock.ExpectExec(regexp.QuoteMeta(queryGuardarToken)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	resp, err := f.svc.Register(context.Background(), RegisterInput{
		Nombre:        "Agustin",
		Email:         "a@test.com",
		Password:      "secreto123",
		MonedaDefault: "USD",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if resp == nil || resp.Usuario == nil {
		t.Fatal("expected a user in response")
	}
	if resp.Usuario.ID != 7 {
		t.Errorf("expected user ID 7, got %d", resp.Usuario.ID)
	}
	if resp.Usuario.MonedaDefault != "USD" {
		t.Errorf("expected moneda USD, got %q", resp.Usuario.MonedaDefault)
	}
	if resp.Usuario.PasswordHash == "" || resp.Usuario.PasswordHash == "secreto123" {
		t.Error("password must be hashed")
	}
	if resp.Mensaje == "" {
		t.Error("expected a pending-verification message")
	}
	if len(f.mailer.links) != 1 {
		t.Fatalf("expected exactly one verification email, got %d", len(f.mailer.links))
	}
	if !strings.Contains(f.mailer.links[0], "/api/auth/verificar?token=") {
		t.Errorf("verification link malformed: %q", f.mailer.links[0])
	}
}

func TestAuthService_Register_MailerFalloNoInvalidaAlta(t *testing.T) {
	f := newAuthServiceFixture(t)
	f.mailer.err = errors.New("smtp caido")
	f.mock.ExpectExec(regexp.QuoteMeta(queryInsertUsuario)).
		WithArgs("Agustin", "a@test.com", sqlmock.AnyArg(), "ARS").
		WillReturnResult(sqlmock.NewResult(3, 1))
	f.mock.ExpectExec(regexp.QuoteMeta(queryGuardarToken)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	resp, err := f.svc.Register(context.Background(), RegisterInput{
		Nombre: "Agustin", Email: "a@test.com", Password: "secreto123",
	})
	if err != nil {
		t.Fatalf("Register should not fail when mailer errors, got: %v", err)
	}
	if resp.Usuario == nil {
		t.Fatal("expected the created user back")
	}
}

func TestAuthService_Register_DefaultsARSCurrency(t *testing.T) {
	svc, mock := newAuthService(t)
	mock.ExpectExec(regexp.QuoteMeta(queryInsertUsuario)).
		WithArgs("Agustin", "a@test.com", sqlmock.AnyArg(), "ARS").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(queryGuardarToken)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	resp, err := svc.Register(context.Background(), RegisterInput{
		Nombre:   "Agustin",
		Email:    "a@test.com",
		Password: "secreto123",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if resp.Usuario.MonedaDefault != "ARS" {
		t.Errorf("expected default ARS, got %q", resp.Usuario.MonedaDefault)
	}
}

func TestAuthService_Register_InvalidInput(t *testing.T) {
	svc, _ := newAuthService(t)
	_, err := svc.Register(context.Background(), RegisterInput{Nombre: "", Email: "", Password: ""})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestAuthService_Register_DuplicateEmail(t *testing.T) {
	svc, mock := newAuthService(t)
	mock.ExpectExec(regexp.QuoteMeta(queryInsertUsuario)).
		WithArgs("Agustin", "dup@test.com", sqlmock.AnyArg(), "ARS").
		WillReturnError(&mysqlErrorDuplicate)

	_, err := svc.Register(context.Background(), RegisterInput{
		Nombre:   "Agustin",
		Email:    "dup@test.com",
		Password: "secreto123",
	})
	if !errors.Is(err, model.ErrEmailExiste) {
		t.Fatalf("expected ErrEmailExiste, got %v", err)
	}
}

func TestAuthService_Register_InvalidEmail(t *testing.T) {
	svc, _ := newAuthService(t)
	invalid := []string{
		"xxx@xx",
		"sin-arroba",
		"a@",
		"@dominio.com",
		"a b@test.com",
		"a@test",
		"a@test..com",
		"a@.com",
		strings.Repeat("a", 246) + "@test.com",
	}
	for _, email := range invalid {
		_, err := svc.Register(context.Background(), RegisterInput{
			Nombre:   "Agustin",
			Email:    email,
			Password: "secreto123",
		})
		if !errors.Is(err, model.ErrEmailInvalido) {
			t.Errorf("Register con email %q: expected ErrEmailInvalido, got %v", email, err)
		}
	}
}

func TestAuthService_Register_InvalidPassword(t *testing.T) {
	svc, _ := newAuthService(t)
	invalid := []string{
		"",
		"corta",
		"12345678",    // solo números
		"abcdefgh",    // solo letras
		"secreto1!",   // carácter especial
		"clave uno1",  // espacio
		"contraseñá1", // fuera del alfabeto alfanumérico ASCII
		strings.Repeat("a", 73),
	}
	for _, pw := range invalid {
		_, err := svc.Register(context.Background(), RegisterInput{
			Nombre:   "Agustin",
			Email:    "pepe@test.com",
			Password: pw,
		})
		if !errors.Is(err, model.ErrPasswordInvalido) {
			t.Errorf("Register con password %q: expected ErrPasswordInvalido, got %v", pw, err)
		}
	}
}

func TestAuthService_Register_NormalizesEmail(t *testing.T) {
	svc, mock := newAuthService(t)
	mock.ExpectExec(regexp.QuoteMeta(queryInsertUsuario)).
		WithArgs("Agustin", "pepe@test.com", sqlmock.AnyArg(), "ARS").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(queryGuardarToken)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	resp, err := svc.Register(context.Background(), RegisterInput{
		Nombre:   "Agustin",
		Email:    "  Pepe@Test.COM  ",
		Password: "secreto123",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if resp.Usuario.Email != "pepe@test.com" {
		t.Errorf("expected normalized email %q, got %q", "pepe@test.com", resp.Usuario.Email)
	}
}

func TestAuthService_Login_InvalidEmail(t *testing.T) {
	svc, _ := newAuthService(t)
	_, err := svc.Login(context.Background(), LoginInput{Email: "xxx@xx", Password: "secreto123"})
	if !errors.Is(err, model.ErrEmailInvalido) {
		t.Fatalf("expected ErrEmailInvalido, got %v", err)
	}
}

func TestAuthService_Login_Valid(t *testing.T) {
	svc, mock := newAuthService(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("secreto123"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(5, "Agustin", "a@test.com", string(hash), "ARS", created, true)
	mock.ExpectQuery(regexp.QuoteMeta(queryFindByEmail)).
		WithArgs("a@test.com").
		WillReturnRows(rows)

	resp, err := svc.Login(context.Background(), LoginInput{Email: "a@test.com", Password: "secreto123"})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if resp.Usuario.ID != 5 {
		t.Errorf("expected user ID 5, got %d", resp.Usuario.ID)
	}
	if resp.Token == "" {
		t.Error("expected a JWT token")
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	svc, mock := newAuthService(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("secreto123"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(5, "Agustin", "a@test.com", string(hash), "ARS", created, true)
	mock.ExpectQuery(regexp.QuoteMeta(queryFindByEmail)).
		WithArgs("a@test.com").
		WillReturnRows(rows)

	_, err := svc.Login(context.Background(), LoginInput{Email: "a@test.com", Password: "incorrecta"})
	if !errors.Is(err, model.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestAuthService_Login_UnknownEmail(t *testing.T) {
	svc, mock := newAuthService(t)
	mock.ExpectQuery(regexp.QuoteMeta(queryFindByEmail)).
		WithArgs("nobody@test.com").
		WillReturnError(sql.ErrNoRows)

	_, err := svc.Login(context.Background(), LoginInput{Email: "nobody@test.com", Password: "x"})
	if !errors.Is(err, model.ErrEmailNoExiste) {
		t.Fatalf("expected ErrEmailNoExiste, got %v", err)
	}
}

func TestAuthService_Login_EmptyInput(t *testing.T) {
	svc, _ := newAuthService(t)
	_, err := svc.Login(context.Background(), LoginInput{Email: "", Password: ""})
	if !errors.Is(err, model.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestAuthService_Login_EmailNoVerificado(t *testing.T) {
	svc, mock := newAuthService(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("secreto123"), bcrypt.MinCost)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(9, "Agustin", "pendiente@test.com", string(hash), "ARS", created, false)
	mock.ExpectQuery(regexp.QuoteMeta(queryFindByEmail)).
		WithArgs("pendiente@test.com").
		WillReturnRows(rows)

	_, err := svc.Login(context.Background(), LoginInput{Email: "pendiente@test.com", Password: "secreto123"})
	if !errors.Is(err, model.ErrEmailNoVerificado) {
		t.Fatalf("expected ErrEmailNoVerificado, got %v", err)
	}
}

func TestAuthService_VerificarEmail_Valid(t *testing.T) {
	f := newAuthServiceFixture(t)
	rawToken, hash, err := generateVerificationToken()
	if err != nil {
		t.Fatalf("generateVerificationToken: %v", err)
	}
	expira := time.Now().Add(time.Hour)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado", "token_expiracion"}).
		AddRow(7, "Agustin", "a@test.com", "$2a$hash", "ARS", time.Now(), false, expira)
	f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByToken)).
		WithArgs(hash).
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta(queryMarcarVerificado)).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := f.svc.VerificarEmail(context.Background(), rawToken); err != nil {
		t.Fatalf("VerificarEmail returned error: %v", err)
	}
}

// Un segundo clic sobre el mismo enlace es idempotente: éxito sin re-marcar.
func TestAuthService_VerificarEmail_YaVerificadoEsIdempotente(t *testing.T) {
	f := newAuthServiceFixture(t)
	rawToken, hash, err := generateVerificationToken()
	if err != nil {
		t.Fatalf("generateVerificationToken: %v", err)
	}
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado", "token_expiracion"}).
		AddRow(7, "Agustin", "a@test.com", "$2a$hash", "ARS", time.Now(), true, nil)
	f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByToken)).
		WithArgs(hash).
		WillReturnRows(rows)

	if err := f.svc.VerificarEmail(context.Background(), rawToken); err != nil {
		t.Fatalf("expected idempotent success, got %v", err)
	}
	if err := f.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("no debe volver a marcar verificado: %v", err)
	}
}

func TestAuthService_VerificarEmail_TokenExpirado(t *testing.T) {
	f := newAuthServiceFixture(t)
	rawToken, hash, err := generateVerificationToken()
	if err != nil {
		t.Fatalf("generateVerificationToken: %v", err)
	}
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado", "token_expiracion"}).
		AddRow(7, "Agustin", "a@test.com", "$2a$hash", "ARS", time.Now(), false, time.Now().Add(-time.Minute))
	f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByToken)).
		WithArgs(hash).
		WillReturnRows(rows)

	err = f.svc.VerificarEmail(context.Background(), rawToken)
	if !errors.Is(err, model.ErrTokenExpirado) {
		t.Fatalf("expected ErrTokenExpirado, got %v", err)
	}
}

func TestAuthService_VerificarEmail_TokenInvalido(t *testing.T) {
	f := newAuthServiceFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByToken)).
		WithArgs(sqlmock.AnyArg()).
		WillReturnError(sql.ErrNoRows)

	if err := f.svc.VerificarEmail(context.Background(), "token-inexistente"); !errors.Is(err, model.ErrTokenInvalido) {
		t.Fatalf("expected ErrTokenInvalido, got %v", err)
	}
}

func TestAuthService_VerificarEmail_TokenVacio(t *testing.T) {
	f := newAuthServiceFixture(t)
	// Si el servicio consultara la base, esta expectativa quedaría consumida.
	f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByToken)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	if err := f.svc.VerificarEmail(context.Background(), "   "); !errors.Is(err, model.ErrTokenInvalido) {
		t.Fatalf("expected ErrTokenInvalido, got %v", err)
	}
	if err := f.mock.ExpectationsWereMet(); err == nil {
		t.Fatal("un token vacío no debe llegar a la base de datos")
	}
}

func TestAuthService_Reenviar_UsuarioPendiente(t *testing.T) {
	f := newAuthServiceFixture(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(7, "Agustin", "re@test.com", "$2a$hash", "ARS", created, false)
	f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByEmail)).
		WithArgs("re@test.com").
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta(queryGuardarToken)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := f.svc.ReenviarVerificacion(context.Background(), ReenvioInput{Email: "re@test.com"}); err != nil {
		t.Fatalf("ReenviarVerificacion returned error: %v", err)
	}
	if len(f.mailer.links) != 1 {
		t.Fatalf("expected one resent email, got %d", len(f.mailer.links))
	}
}

// Anti-enumeración: email desconocido y usuario ya verificado responden éxito
// genérico y no envían nada.
func TestAuthService_Reenviar_SinEfectoSoloGenerico(t *testing.T) {
	t.Run("email desconocido", func(t *testing.T) {
		f := newAuthServiceFixture(t)
		f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByEmail)).
			WithArgs("nadie@test.com").
			WillReturnError(sql.ErrNoRows)

		if err := f.svc.ReenviarVerificacion(context.Background(), ReenvioInput{Email: "nadie@test.com"}); err != nil {
			t.Fatalf("expected nil for unknown email, got %v", err)
		}
		if len(f.mailer.links) != 0 {
			t.Errorf("must not send emails for unknown addresses")
		}
	})

	t.Run("usuario ya verificado", func(t *testing.T) {
		f := newAuthServiceFixture(t)
		created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
			AddRow(7, "Agustin", "verif@test.com", "$2a$hash", "ARS", created, true)
		f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByEmail)).
			WithArgs("verif@test.com").
			WillReturnRows(rows)

		if err := f.svc.ReenviarVerificacion(context.Background(), ReenvioInput{Email: "verif@test.com"}); err != nil {
			t.Fatalf("expected nil for verified user, got %v", err)
		}
		if len(f.mailer.links) != 0 {
			t.Errorf("must not send emails for already verified users")
		}
	})
}

// El reenvío regenera el token: el enlace viejo deja de funcionar porque su
// hash ya no está en la base.
func TestAuthService_Reenviar_InvalidaEnlaceAnterior(t *testing.T) {
	f := newAuthServiceFixture(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(7, "Agustin", "re2@test.com", "$2a$hash", "ARS", created, false)
	f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByEmail)).
		WithArgs("re2@test.com").
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta(queryGuardarToken)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := f.svc.ReenviarVerificacion(context.Background(), ReenvioInput{Email: "re2@test.com"}); err != nil {
		t.Fatalf("ReenviarVerificacion returned error: %v", err)
	}

	// El enlace anterior apunta a un hash distinto del recién guardado:
	// simulo la búsqueda con el hash original y la DB responde sin filas.
	viejoRaw, _, _ := generateVerificationToken()
	f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByToken)).
		WithArgs(hashVerificationToken(viejoRaw)).
		WillReturnError(sql.ErrNoRows)
	if err := f.svc.VerificarEmail(context.Background(), viejoRaw); !errors.Is(err, model.ErrTokenInvalido) {
		t.Fatalf("old link should be invalid after resend, got %v", err)
	}
}

func TestAuthService_GenerateVerificationToken_Formato(t *testing.T) {
	raw, hash, err := generateVerificationToken()
	if err != nil {
		t.Fatalf("generateVerificationToken: %v", err)
	}
	if len(raw) != 64 || len(hash) != 64 {
		t.Fatalf("expected 64-char raw and hash, got %d/%d", len(raw), len(hash))
	}
	if hash != hashVerificationToken(raw) {
		t.Error("hash must be the SHA-256 of the raw token")
	}
}

func TestAuthService_GenerateTokenIsVerifiable(t *testing.T) {
	svc, _ := newAuthService(t)
	token, err := svc.generateToken(42)
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	valid, userID, err := parseTestToken(token, []byte("test-secret"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !valid {
		t.Error("token should be valid")
	}
	if userID != 42 {
		t.Errorf("expected sub 42, got %d", userID)
	}
}

// --- ForgotPassword ---

const queryGuardarPasswordReset = "UPDATE usuarios SET password_reset_token = ?, password_reset_expiracion = ? WHERE id = ?"
const queryFindByPasswordResetToken = "SELECT id, nombre, email, password_hash, moneda_default, created_at, email_verificado, password_reset_expiracion FROM usuarios WHERE password_reset_token = ?"

func TestAuthService_ForgotPassword_UsuarioExistente(t *testing.T) {
	f := newAuthServiceFixture(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado"}).
		AddRow(7, "Agustin", "a@test.com", "$2a$hash", "ARS", created, true)
	f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByEmail)).
		WithArgs("a@test.com").
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta(queryGuardarPasswordReset)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := f.svc.ForgotPassword(context.Background(), ForgotPasswordInput{Email: "a@test.com"}); err != nil {
		t.Fatalf("ForgotPassword returned error: %v", err)
	}
	if len(f.mailer.links) != 1 {
		t.Fatalf("expected one reset email, got %d", len(f.mailer.links))
	}
	if !strings.Contains(f.mailer.links[0], "/reset-password?token=") {
		t.Errorf("reset link malformed: %q", f.mailer.links[0])
	}
}

// Anti-enumeración: email desconocido responde éxito genérico y no envía nada.
func TestAuthService_ForgotPassword_EmailDesconocido(t *testing.T) {
	f := newAuthServiceFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByEmail)).
		WithArgs("nadie@test.com").
		WillReturnError(sql.ErrNoRows)

	if err := f.svc.ForgotPassword(context.Background(), ForgotPasswordInput{Email: "nadie@test.com"}); err != nil {
		t.Fatalf("expected nil for unknown email, got %v", err)
	}
	if len(f.mailer.links) != 0 {
		t.Errorf("must not send emails for unknown addresses")
	}
}

func TestAuthService_ForgotPassword_EmailInvalido(t *testing.T) {
	svc, _ := newAuthService(t)
	// Email inválido no debe fallar: retorna nil (respuesta genérica)
	if err := svc.ForgotPassword(context.Background(), ForgotPasswordInput{Email: "invalido"}); err != nil {
		t.Fatalf("expected nil for invalid email, got %v", err)
	}
}

// --- ResetPassword ---

func TestAuthService_ResetPassword_Exitoso(t *testing.T) {
	f := newAuthServiceFixture(t)
	rawToken, hash, err := generateVerificationToken()
	if err != nil {
		t.Fatalf("generateVerificationToken: %v", err)
	}
	expira := time.Now().Add(time.Hour)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado", "password_reset_expiracion"}).
		AddRow(7, "Agustin", "a@test.com", "$2a$oldhash", "ARS", time.Now(), true, expira)
	f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByPasswordResetToken)).
		WithArgs(hash).
		WillReturnRows(rows)
	f.mock.ExpectExec(regexp.QuoteMeta("UPDATE usuarios SET password_hash = ?, password_reset_token = NULL, password_reset_expiracion = NULL WHERE id = ?")).
		WithArgs(sqlmock.AnyArg(), int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	resp, err := f.svc.ResetPassword(context.Background(), ResetPasswordInput{
		Token:     rawToken,
		Password:  "nuevaclave1",
		Password2: "nuevaclave1",
	})
	if err != nil {
		t.Fatalf("ResetPassword returned error: %v", err)
	}
	if resp.Mensaje == "" {
		t.Error("expected success message")
	}
}

func TestAuthService_ResetPassword_ContrasenasNoCoinciden(t *testing.T) {
	svc, _ := newAuthService(t)
	_, err := svc.ResetPassword(context.Background(), ResetPasswordInput{
		Token:     "abc123",
		Password:  "nuevaclave1",
		Password2: "otraclave1",
	})
	if !errors.Is(err, model.ErrPasswordInvalido) {
		t.Fatalf("expected ErrPasswordInvalido, got %v", err)
	}
}

func TestAuthService_ResetPassword_ContrasenaInvalida(t *testing.T) {
	svc, _ := newAuthService(t)
	_, err := svc.ResetPassword(context.Background(), ResetPasswordInput{
		Token:     "abc123",
		Password:  "corta",
		Password2: "corta",
	})
	if !errors.Is(err, model.ErrPasswordInvalido) {
		t.Fatalf("expected ErrPasswordInvalido, got %v", err)
	}
}

func TestAuthService_ResetPassword_TokenInvalido(t *testing.T) {
	f := newAuthServiceFixture(t)
	f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByPasswordResetToken)).
		WithArgs(sqlmock.AnyArg()).
		WillReturnError(sql.ErrNoRows)

	_, err := f.svc.ResetPassword(context.Background(), ResetPasswordInput{
		Token:     "token-inexistente",
		Password:  "nuevaclave1",
		Password2: "nuevaclave1",
	})
	if !errors.Is(err, model.ErrPasswordResetInvalido) {
		t.Fatalf("expected ErrPasswordResetInvalido, got %v", err)
	}
}

func TestAuthService_ResetPassword_TokenExpirado(t *testing.T) {
	f := newAuthServiceFixture(t)
	rawToken, hash, err := generateVerificationToken()
	if err != nil {
		t.Fatalf("generateVerificationToken: %v", err)
	}
	expira := time.Now().Add(-time.Minute)
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at", "email_verificado", "password_reset_expiracion"}).
		AddRow(7, "Agustin", "a@test.com", "$2a$oldhash", "ARS", time.Now(), true, expira)
	f.mock.ExpectQuery(regexp.QuoteMeta(queryFindByPasswordResetToken)).
		WithArgs(hash).
		WillReturnRows(rows)

	_, err = f.svc.ResetPassword(context.Background(), ResetPasswordInput{
		Token:     rawToken,
		Password:  "nuevaclave1",
		Password2: "nuevaclave1",
	})
	if !errors.Is(err, model.ErrPasswordResetExpirado) {
		t.Fatalf("expected ErrPasswordResetExpirado, got %v", err)
	}
}

func TestAuthService_ResetPassword_TokenVacio(t *testing.T) {
	svc, _ := newAuthService(t)
	_, err := svc.ResetPassword(context.Background(), ResetPasswordInput{
		Token:     "   ",
		Password:  "nuevaclave1",
		Password2: "nuevaclave1",
	})
	if !errors.Is(err, model.ErrPasswordResetInvalido) {
		t.Fatalf("expected ErrPasswordResetInvalido, got %v", err)
	}
}
