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
	queryInsertUsuario = "INSERT INTO usuarios (nombre, email, password_hash, moneda_default) VALUES (?, ?, ?, ?)"
	queryFindByEmail   = "SELECT id, nombre, email, password_hash, moneda_default, created_at FROM usuarios WHERE email = ?"
)

func newAuthService(t *testing.T) (*AuthService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewAuthService(repository.NewUsuarioRepo(db), []byte("test-secret"), 72*time.Hour), mock
}

func TestAuthService_Register_Valid(t *testing.T) {
	svc, mock := newAuthService(t)
	mock.ExpectExec(regexp.QuoteMeta(queryInsertUsuario)).
		WithArgs("Agustin", "a@test.com", sqlmock.AnyArg(), "USD").
		WillReturnResult(sqlmock.NewResult(7, 1))

	resp, err := svc.Register(context.Background(), RegisterInput{
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
	if resp.Token == "" {
		t.Error("expected a JWT token")
	}
}

func TestAuthService_Register_DefaultsARSCurrency(t *testing.T) {
	svc, mock := newAuthService(t)
	mock.ExpectExec(regexp.QuoteMeta(queryInsertUsuario)).
		WithArgs("Agustin", "a@test.com", sqlmock.AnyArg(), "ARS").
		WillReturnResult(sqlmock.NewResult(1, 1))

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
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at"}).
		AddRow(5, "Agustin", "a@test.com", string(hash), "ARS", created)
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
	rows := sqlmock.NewRows([]string{"id", "nombre", "email", "password_hash", "moneda_default", "created_at"}).
		AddRow(5, "Agustin", "a@test.com", string(hash), "ARS", created)
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
