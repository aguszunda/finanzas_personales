package model

import "errors"

var (
	ErrNotFound         = errors.New("recurso no encontrado")
	ErrUnauthorized     = errors.New("no autorizado")
	ErrInvalidInput     = errors.New("entrada inválida")
	ErrDuplicate        = errors.New("el recurso ya existe")
	ErrMesCerrado       = errors.New("el mes está cerrado, no se puede modificar")
	ErrForbidden        = errors.New("acceso denegado")
	ErrEmailExiste      = errors.New("el email ya está registrado")
	ErrEmailNoExiste    = errors.New("el email no está registrado")
	ErrEmailInvalido    = errors.New("el email no es válido")
	ErrPasswordInvalido = errors.New("la contraseña no es válida")

	ErrEmailNoVerificado = errors.New("el email no está verificado")
	ErrTokenInvalido     = errors.New("el token de verificación no es válido")
	ErrTokenExpirado     = errors.New("el token de verificación está expirado")

	ErrPasswordResetInvalido = errors.New("el token de reseteo no es válido")
	ErrPasswordResetExpirado = errors.New("el token de reseteo está expirado")
)
