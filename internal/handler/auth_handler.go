package handler

import (
	"net/http"

	"finanzas_personales/internal/service"
)

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var input service.RegisterInput
	if err := decodeBody(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "formulario inválido")
		return
	}
	result, err := h.svc.Register(r.Context(), input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	setAuthCookie(w, result.Token)
	respondMutation(w, r, http.StatusCreated, result, "/api/dashboard/page")
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var input service.LoginInput
	if err := decodeBody(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "formulario inválido")
		return
	}
	result, err := h.svc.Login(r.Context(), input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	setAuthCookie(w, result.Token)
	respondMutation(w, r, http.StatusOK, result, "/api/dashboard/page")
}

func setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   72 * 60 * 60,
	})
}
