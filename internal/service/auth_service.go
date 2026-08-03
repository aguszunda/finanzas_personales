package service

import (
	"context"
	"time"

	"administracion-financiera/internal/model"
	"administracion-financiera/internal/repository"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

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
	if input.Nombre == "" || input.Email == "" || input.Password == "" {
		return nil, model.ErrInvalidInput
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
		Email:         input.Email,
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
	if input.Email == "" || input.Password == "" {
		return nil, model.ErrInvalidInput
	}
	u, err := s.usuarioRepo.FindByEmail(ctx, input.Email)
	if err != nil {
		return nil, model.ErrUnauthorized
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
