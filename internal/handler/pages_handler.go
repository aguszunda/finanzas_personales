package handler

import (
	"net/http"
	"strconv"

	"optipay/internal/middleware"
	"optipay/internal/repository"
	"optipay/internal/service"

	"github.com/go-chi/chi/v5"
)

type PagesHandler struct {
	dashboardSvc *service.DashboardService
	transSvc     *service.TransaccionService
	mesSvc       *service.MesService
	catRepo      *repository.CategoriaRepo
	authSvc      *service.AuthService
}

func NewPagesHandler(ds *service.DashboardService, ts *service.TransaccionService, ms *service.MesService, cr *repository.CategoriaRepo, as *service.AuthService) *PagesHandler {
	return &PagesHandler{
		dashboardSvc: ds, transSvc: ts, mesSvc: ms, catRepo: cr, authSvc: as,
	}
}

// userName devuelve el nombre del usuario autenticado para el nav.
func (h *PagesHandler) userName(r *http.Request) string {
	uid := middleware.UserIDFromContext(r.Context())
	if uid == 0 {
		return ""
	}
	u, err := h.authSvc.GetUsuario(r.Context(), uid)
	if err != nil {
		return ""
	}
	return u.Nombre
}

func (h *PagesHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "login", map[string]interface{}{"hideNav": true})
}

func (h *PagesHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "register", map[string]interface{}{"hideNav": true})
}

// VerificacionPage muestra el resultado de confirmar el email (o de pedir un
// reenvío desde un form plano). El login queda intacto: los estados viven acá.
func (h *PagesHandler) VerificacionPage(w http.ResponseWriter, r *http.Request) {
	estado := r.URL.Query().Get("estado")
	data := map[string]interface{}{
		"hideNav":   true,
		"Exito":     estado == "ok",
		"Reenviado": estado == "reenviado",
		"Expirado":  estado == "expirado",
		"Invalido":  estado == "invalido",
	}
	renderTemplate(w, "verificacion", data)
}

func (h *PagesHandler) ReenvioPage(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "reenvio", map[string]interface{}{"hideNav": true})
}

// ForgotPasswordPage renderiza el formulario de solicitud de reseteo de contraseña.
func (h *PagesHandler) ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	estado := r.URL.Query().Get("estado")
	email := r.URL.Query().Get("email")
	data := map[string]interface{}{
		"hideNav": true,
		"Exito":   estado == "ok",
		"Email":   email,
	}
	renderTemplate(w, "forgot_password", data)
}

func (h *PagesHandler) DashboardPage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	periodo := r.URL.Query().Get("periodo")
	data, err := h.dashboardSvc.GetDashboard(r.Context(), uid, periodo)
	if err != nil {
		renderTemplate(w, "dashboard", map[string]interface{}{"error": err.Error(), "userName": h.userName(r)})
		return
	}
	meses, _ := h.mesSvc.List(r.Context(), uid)
	renderTemplate(w, "dashboard", map[string]interface{}{
		"error":    "",
		"d":        data,
		"meses":    meses,
		"periodo":  periodo,
		"userName": h.userName(r),
	})
}

func (h *PagesHandler) MesesPage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	meses, err := h.mesSvc.List(r.Context(), uid)
	if err != nil {
		renderTemplate(w, "meses", map[string]interface{}{"error": err.Error(), "userName": h.userName(r)})
		return
	}
	renderTemplate(w, "meses", map[string]interface{}{"meses": meses, "userName": h.userName(r)})
}

func (h *PagesHandler) BalancePage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	mes, transacciones, err := h.mesSvc.Balance(r.Context(), uid, id)
	if err != nil {
		renderTemplate(w, "balance", map[string]interface{}{"error": err.Error(), "userName": h.userName(r)})
		return
	}
	renderTemplate(w, "balance", map[string]interface{}{
		"mes":           mes,
		"transacciones": transacciones,
		"userName":      h.userName(r),
	})
}

// TransaccionForm renderiza el partial del formulario de transacción. Sin
// edit_id devuelve el modo "nueva" (hx-post); con edit_id precarga la
// transacción y responde con hx-put. El verbo y los valores vienen del
// servidor para que HTMX procese los atributos al recibir el fragmento.
func (h *PagesHandler) TransaccionForm(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	cats, _ := h.catRepo.FindAll(r.Context(), uid)
	data := map[string]interface{}{"categorias": cats}
	if editID, err := strconv.ParseInt(r.URL.Query().Get("edit_id"), 10, 64); err == nil && editID > 0 {
		t, err := h.transSvc.GetByID(r.Context(), editID, uid)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		data["transaccion"] = t
	}
	renderTemplateFragment(w, "transaccion_form", data)
}
