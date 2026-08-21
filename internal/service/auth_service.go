package service

import (
	"context"
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
}

func NewAuthService(ur *repository.UsuarioRepo, secret []byte, exp time.Duration) *AuthService {
	return &AuthService{usuarioRepo: ur, jwtSecret: secret, jwtExpiration: exp}
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

type AuthResponse struct {
	Token   string         `json:"token"`
	Usuario *model.Usuario `json:"usuario"`
}

func (s *AuthService) Register(ctx context.Context, input RegisterInput) (*AuthResponse, error) {
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
	token, err := s.generateToken(u.ID)
	if err != nil {
		return nil, err
	}
	return &AuthResponse{Token: token, Usuario: u}, nil
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
	token, err := s.generateToken(u.ID)
	if err != nil {
		return nil, err
	}
	return &AuthResponse{Token: token, Usuario: u}, nil
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
