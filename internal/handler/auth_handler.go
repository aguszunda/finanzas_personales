package handler

import (
	"errors"
	"net/http"
	"strings"

	"optipay/internal/middleware"
	"optipay/internal/model"
	"optipay/internal/service"
)

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

// Register crea la cuenta pero NO inicia sesión: el alta queda pendiente de
// verificar el email. La confirmación se muestra como pop-up sobre la propia
// pantalla de registro (nunca en el login):
//   - HTMX -> fragmento register_exito que reemplaza #auth-panel.
//   - form plano -> render de la página de registro con el pop-up (200).
//   - API JSON -> payload con el mensaje (201).
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
	data := map[string]interface{}{
		"hideNav":       true,
		"registroExito": true,
		"Email":         result.Usuario.Email,
	}
	switch {
	case middleware.IsHTMXRequest(r.Context()):
		renderTemplateFragment(w, "register_exito", data)
	case strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"):
		renderTemplate(w, "register", data)
	default:
		respondJSON(w, http.StatusCreated, result)
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var input service.LoginInput
	if err := decodeBody(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "formulario inválido")
		return
	}
	result, err := h.svc.Login(r.Context(), input)
	if err != nil {
		if errors.Is(err, model.ErrEmailNoVerificado) {
			h.pendienteVerificacion(w, r, input.Email)
			return
		}
		handleServiceError(w, err)
		return
	}
	setAuthCookie(w, result.Token)
	respondMutation(w, r, http.StatusOK, result, "/api/dashboard/page")
}

// pendienteVerificacion responde al intento de login de una cuenta sin
// verificar: en la web muestra el pop-up de confirmación con el form de
// reenvío precargado con el email intentado; los clientes API conservan
// el 403 JSON de handleServiceError.
func (h *AuthHandler) pendienteVerificacion(w http.ResponseWriter, r *http.Request, email string) {
	data := map[string]interface{}{
		"hideNav":               true,
		"pendienteVerificacion": true,
		"Email":                 email,
	}
	switch {
	case middleware.IsHTMXRequest(r.Context()):
		renderTemplateFragment(w, "login_verificar", data)
	case strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"):
		renderTemplate(w, "login", data)
	default:
		handleServiceError(w, model.ErrEmailNoVerificado)
	}
}

// Verificar consume el enlace del mail (GET desde el cliente de correo, sin
// HTMX) y renderiza la página de resultado: éxito, expirado o inválido.
func (h *AuthHandler) Verificar(w http.ResponseWriter, r *http.Request) {
	err := h.svc.VerificarEmail(r.Context(), r.URL.Query().Get("token"))
	switch {
	case err == nil:
		renderTemplate(w, "verificacion", map[string]interface{}{"hideNav": true, "Exito": true})
	case errors.Is(err, model.ErrTokenExpirado):
		renderTemplate(w, "verificacion", map[string]interface{}{"hideNav": true, "Expirado": true})
	case errors.Is(err, model.ErrTokenInvalido):
		renderTemplate(w, "verificacion", map[string]interface{}{"hideNav": true, "Invalido": true})
	default:
		handleServiceError(w, err)
	}
}

// ReenviarVerificacion regenera y reenvía el enlace. La respuesta es genérica:
// no revela si el email existe o ya está verificado. En HTMX reemplaza
// #auth-panel por la confirmación; un form plano aterriza en /verificacion.
func (h *AuthHandler) ReenviarVerificacion(w http.ResponseWriter, r *http.Request) {
	var input service.ReenvioInput
	if err := decodeBody(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "formulario inválido")
		return
	}
	if err := h.svc.ReenviarVerificacion(r.Context(), input); err != nil {
		handleServiceError(w, err)
		return
	}
	mensaje := "Si el email está registrado y sin verificar, te enviamos un nuevo enlace."
	switch {
	case middleware.IsHTMXRequest(r.Context()):
		renderTemplateFragment(w, "reenvio_ok", map[string]interface{}{"Mensaje": mensaje})
	case strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"):
		http.Redirect(w, r, "/verificacion?estado=reenviado", http.StatusSeeOther)
	default:
		respondJSON(w, http.StatusOK, service.RegisterResponse{Mensaje: mensaje})
	}
}

// ForgotPassword procesa el pedido de reseteo de contraseña. La respuesta es
// genérica: no revela si el email existe o no.
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var input service.ForgotPasswordInput
	if err := decodeBody(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "formulario inválido")
		return
	}
	if err := h.svc.ForgotPassword(r.Context(), input); err != nil {
		handleServiceError(w, err)
		return
	}
	mensaje := "Si el email está registrado, te enviamos un enlace para restablecer tu contraseña."
	switch {
	case middleware.IsHTMXRequest(r.Context()):
		renderTemplateFragment(w, "forgot_ok", map[string]interface{}{"Mensaje": mensaje})
	case strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"):
		http.Redirect(w, r, "/forgot-password?estado=ok", http.StatusSeeOther)
	default:
		respondJSON(w, http.StatusOK, service.ResetPasswordResponse{Mensaje: mensaje})
	}
}

// ResetPasswordPage renderiza el formulario de nueva contraseña. El token viene
// como query param (?token=xxx) y se pasa al template.
func (h *AuthHandler) ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	renderTemplate(w, "reset_password", map[string]interface{}{
		"hideNav": true,
		"Token":   token,
	})
}

// ResetPassword valida el token y cambia la contraseña.
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var input service.ResetPasswordInput
	if err := decodeBody(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "formulario inválido")
		return
	}
	result, err := h.svc.ResetPassword(r.Context(), input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	switch {
	case middleware.IsHTMXRequest(r.Context()):
		renderTemplateFragment(w, "reset_ok", result)
	case strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded"):
		http.Redirect(w, r, "/login?reset=ok", http.StatusSeeOther)
	default:
		respondJSON(w, http.StatusOK, result)
	}
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
