package model

import "errors"

var (
	ErrNotFound     = errors.New("recurso no encontrado")
	ErrUnauthorized = errors.New("no autorizado")
	ErrInvalidInput = errors.New("entrada inválida")
	ErrDuplicate    = errors.New("el recurso ya existe")
	ErrMesCerrado   = errors.New("el mes está cerrado, no se puede modificar")
	ErrForbidden    = errors.New("acceso denegado")
	ErrEmailExiste  = errors.New("el email ya está registrado")
	ErrEmailNoExiste = errors.New("el email no está registrado")
)
