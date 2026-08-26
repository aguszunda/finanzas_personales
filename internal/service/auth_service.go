package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"optipay/internal/model"
	"optipay/internal/repository"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// emailRe exige estructura RFC 5322 básica y dominio con TLD (ej: nombre@dominio.com).
var emailRe = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_\x60{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

// passwordRe restringe la contraseña al alfabeto alfanumérico ASCII:
// rechaza espacios, símbolos y letras fuera de a-z / A-Z / 0-9.
var passwordRe = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

const maxEmailLen = 254

const (
	minPasswordLen = 8
	maxPasswordLen = 72

	// verificationTokenBytes produce un token de 64 caracteres hex; el hash
	// SHA-256 almacenado tiene la misma longitud (columna CHAR(64)).
	verificationTokenBytes = 32

	// verificationTokenTTL es la vigencia del enlace enviado por mail.
	verificationTokenTTL = 48 * time.Hour
)

// validatePassword exige longitud entre 8 y 72 bytes (el límite que bcrypt
// acepta sin truncar silenciosamente), solo caracteres alfanuméricos y al
// menos una letra y un número, para descartar contraseñas triviales como
// "12345678" o "aaaaaaaa".
func validatePassword(pw string) error {
	if pw == "" || len(pw) < minPasswordLen || len(pw) > maxPasswordLen {
		return model.ErrPasswordInvalido
	}
	if !passwordRe.MatchString(pw) {
		return model.ErrPasswordInvalido
	}
	var hasLetra, hasDigito bool
	for _, c := range pw {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			hasLetra = true
		case c >= '0' && c <= '9':
			hasDigito = true
		}
	}
	if !hasLetra || !hasDigito {
		return model.ErrPasswordInvalido
	}
	return nil
}

// normalizeEmail valida el formato y normaliza (trim + lowercase) antes de
// guardar o buscar, evitando duplicados por mayúsculas o espacios.
func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(email) > maxEmailLen || !emailRe.MatchString(email) {
		return "", model.ErrEmailInvalido
	}
	return email, nil
}

type AuthService struct {
	usuarioRepo   *repository.UsuarioRepo
	jwtSecret     []byte
	jwtExpiration time.Duration
	mailer        Mailer
	baseURL       string
}

func NewAuthService(ur *repository.UsuarioRepo, secret []byte, exp time.Duration, mailer Mailer, baseURL string) *AuthService {
	return &AuthService{
		usuarioRepo:   ur,
		jwtSecret:     secret,
		jwtExpiration: exp,
		mailer:        mailer,
		baseURL:       strings.TrimRight(baseURL, "/"),
	}
}

type RegisterInput struct {
	Nombre        string `json:"nombre"`
	Email         string `json:"email"`
	Password      string `json:"password"`
	MonedaDefault string `json:"moneda_default"`
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse es la respuesta del login (única vía que emite sesión).
type AuthResponse struct {
	Token   string         `json:"token"`
	Usuario *model.Usuario `json:"usuario"`
}

// RegisterResponse confirma el alta pendiente de verificación; no expone
// sesión hasta que el usuario confirme su email.
type RegisterResponse struct {
	Mensaje string         `json:"mensaje"`
	Usuario *model.Usuario `json:"usuario"`
}

type ReenvioInput struct {
	Email string `json:"email"`
}

// generateVerificationToken devuelve el token crudo (va en el link) y su hash
// SHA-256 (es lo único que se persiste, igual criterio que password_hash).
func generateVerificationToken() (raw, hash string, err error) {
	b := make([]byte, verificationTokenBytes)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(raw))
	return raw, hex.EncodeToString(sum[:]), nil
}

func hashVerificationToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// sendVerificacionEmail arma el link y lo envía. Un fallo de envío NO invalida
// el registro: el usuario ya existe y puede pedir un reenvío, así que se
// registra el error y se continúa.
func (s *AuthService) sendVerificacionEmail(ctx context.Context, u *model.Usuario, rawToken string) {
	link := s.baseURL + "/api/auth/verificar?token=" + rawToken
	if err := s.mailer.SendVerificacion(ctx, u.Email, u.Nombre, link); err != nil {
		slog.Error("envío de email de verificación falló", "email", u.Email, "error", err)
	}
}

func (s *AuthService) Register(ctx context.Context, input RegisterInput) (*RegisterResponse, error) {
	if input.Nombre == "" {
		return nil, model.ErrInvalidInput
	}
	if err := validatePassword(input.Password); err != nil {
		return nil, err
	}
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	if input.MonedaDefault == "" {
		input.MonedaDefault = "ARS"
	}
	u := &model.Usuario{
		Nombre:        input.Nombre,
		Email:         email,
		PasswordHash:  string(hash),
		MonedaDefault: input.MonedaDefault,
	}
	if err := s.usuarioRepo.Create(ctx, u); err != nil {
		return nil, err
	}
	rawToken, tokenHash, err := generateVerificationToken()
	if err != nil {
		return nil, err
	}
	if err := s.usuarioRepo.GuardarTokenVerificacion(ctx, u.ID, tokenHash, time.Now().Add(verificationTokenTTL)); err != nil {
		return nil, err
	}
	s.sendVerificacionEmail(ctx, u, rawToken)
	return &RegisterResponse{
		Mensaje: "Cuenta creada. Te enviamos un email para confirmar tu dirección; podés ingresar una vez verificado.",
		Usuario: u,
	}, nil
}

func (s *AuthService) Login(ctx context.Context, input LoginInput) (*AuthResponse, error) {
	if input.Password == "" {
		return nil, model.ErrInvalidInput
	}
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return nil, err
	}
	u, err := s.usuarioRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, model.ErrEmailNoExiste
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(input.Password)); err != nil {
		return nil, model.ErrUnauthorized
	}
	// El chequeo va después de validar la contraseña para no revelar el estado
	// de verificación a quien no conoce las credenciales.
	if !u.EmailVerificado {
		return nil, model.ErrEmailNoVerificado
	}
	token, err := s.generateToken(u.ID)
	if err != nil {
		return nil, err
	}
	return &AuthResponse{Token: token, Usuario: u}, nil
}

// VerificarEmail consume un token de verificación: marca el email como
// confirmado. Es idempotente (un enlace ya usado responde éxito) pero distingue
// token inválido de expirado para que la UI pueda sugerir el reenvío.
func (s *AuthService) VerificarEmail(ctx context.Context, rawToken string) error {
	if strings.TrimSpace(rawToken) == "" || len(rawToken) > 512 {
		return model.ErrTokenInvalido
	}
	u, err := s.usuarioRepo.FindByTokenVerificacion(ctx, hashVerificationToken(rawToken))
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return model.ErrTokenInvalido
		}
		return err
	}
	if u.EmailVerificado {
		return nil
	}
	if u.TokenExpiracion != nil && time.Now().After(*u.TokenExpiracion) {
		return model.ErrTokenExpirado
	}
	return s.usuarioRepo.MarcarVerificado(ctx, u.ID)
}

// ReenviarVerificacion regenera el token y reenvía el mail. Responde éxito
// genérico aunque el email no exista o ya esté verificado, para no permitir
// enumerar cuentas registradas.
func (s *AuthService) ReenviarVerificacion(ctx context.Context, input ReenvioInput) error {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return err
	}
	u, err := s.usuarioRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return nil
		}
		return err
	}
	if u.EmailVerificado {
		return nil
	}
	rawToken, tokenHash, err := generateVerificationToken()
	if err != nil {
		return err
	}
	if err := s.usuarioRepo.GuardarTokenVerificacion(ctx, u.ID, tokenHash, time.Now().Add(verificationTokenTTL)); err != nil {
		return err
	}
	s.sendVerificacionEmail(ctx, u, rawToken)
	return nil
}

func (s *AuthService) generateToken(userID int64) (string, error) {
	claims := jwt.MapClaims{
		"sub": float64(userID),
		"exp": time.Now().Add(s.jwtExpiration).Unix(),
		"iat": time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}
