package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"optipay/internal/middleware"
	"optipay/internal/model"
	"optipay/internal/repository"
	"optipay/internal/service"

	"github.com/go-chi/chi/v5"
)

type PagesHandler struct {
	dashboardSvc *service.DashboardService
	transSvc     *service.TransaccionService
	cfSvc        *service.CostoFijoService
	mesSvc       *service.MesService
	deudaSvc     *service.DeudaService
	catRepo      *repository.CategoriaRepo
	authSvc      *service.AuthService
}

func NewPagesHandler(ds *service.DashboardService, ts *service.TransaccionService, cfs *service.CostoFijoService, ms *service.MesService, ds2 *service.DeudaService, cr *repository.CategoriaRepo, as *service.AuthService) *PagesHandler {
	return &PagesHandler{
		dashboardSvc: ds, transSvc: ts, cfSvc: cfs, mesSvc: ms, deudaSvc: ds2, catRepo: cr, authSvc: as,
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

func (h *PagesHandler) TransaccionesPage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	periodo := r.URL.Query().Get("periodo")
	if periodo == "" {
		periodo = "all"
	}
	var transacciones []model.Transaccion
	var err error
	if periodo == "all" {
		transacciones, err = h.transSvc.List(r.Context(), uid, 100, 0)
	} else {
		transacciones, err = h.transSvc.ListByPeriodo(r.Context(), uid, periodo)
	}
	if err != nil {
		renderTemplate(w, "transacciones", map[string]interface{}{"error": err.Error(), "userName": h.userName(r)})
		return
	}
	cats, _ := h.catRepo.FindAll(r.Context(), uid)
	meses, _ := h.mesSvc.List(r.Context(), uid)
	renderTemplate(w, "transacciones", map[string]interface{}{
		"transacciones": transacciones,
		"categorias":    cats,
		"meses":         meses,
		"periodo":       periodo,
		"userName":      h.userName(r),
	})
}

func (h *PagesHandler) CostosFijosPage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	list, err := h.cfSvc.List(r.Context(), uid)
	if err != nil {
		renderTemplate(w, "costos_fijos", map[string]interface{}{"error": err.Error(), "userName": h.userName(r)})
		return
	}
	cats, _ := h.catRepo.FindAll(r.Context(), uid)
	renderTemplate(w, "costos_fijos", map[string]interface{}{
		"costos_fijos": list,
		"categorias":   cats,
		"userName":     h.userName(r),
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
	deudas, err := h.deudaSvc.List(r.Context(), uid)
	if err != nil {
		renderTemplate(w, "balance", map[string]interface{}{"error": err.Error(), "userName": h.userName(r)})
		return
	}
	var deudasPendientes []model.Deuda
	for _, d := range deudas {
		if d.Estado != "pagada" {
			deudasPendientes = append(deudasPendientes, d)
		}
	}
	renderTemplate(w, "balance", map[string]interface{}{
		"mes":           mes,
		"transacciones": transacciones,
		"deudas":        deudasPendientes,
		"userName":      h.userName(r),
	})
}

func (h *PagesHandler) DeudasPage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	deudas, err := h.deudaSvc.List(r.Context(), uid)
	if err != nil {
		renderTemplate(w, "deudas", map[string]interface{}{"error": err.Error(), "userName": h.userName(r)})
		return
	}
	renderTemplate(w, "deudas", map[string]interface{}{
		"deudas":     deudas,
		"categorias": h.categoriasEgreso(r),
		"userName":   h.userName(r),
	})
}

// categoriasEgreso devuelve las categorías de tipo egreso (de sistema y del
// usuario), usadas por el formulario de deuda y el de pago.
func (h *PagesHandler) categoriasEgreso(r *http.Request) []model.Categoria {
	uid := middleware.UserIDFromContext(r.Context())
	cats, err := h.catRepo.FindAll(r.Context(), uid)
	if err != nil {
		return nil
	}
	var egresos []model.Categoria
	for _, c := range cats {
		if c.Tipo == "egreso" {
			egresos = append(egresos, c)
		}
	}
	return egresos
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

func (h *PagesHandler) DeudaForm(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	data := map[string]interface{}{"categorias": h.categoriasEgreso(r)}
	if editID, err := strconv.ParseInt(r.URL.Query().Get("edit_id"), 10, 64); err == nil && editID > 0 {
		d, err := h.deudaSvc.GetByID(r.Context(), editID, uid)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		data["deuda"] = d
	}
	renderTemplateFragment(w, "deuda_form", data)
}

// DeudaPagoForm renderiza el partial de confirmación de pago: muestra el monto
// a registrar como egreso, deja elegir la categoría y la fecha. Si el mes
// actual está cerrado, precarga una fecha en el primer mes abierto (el que
// deja Cerrar) y muestra un aviso, para que el pago no quede bloqueado.
func (h *PagesHandler) DeudaPagoForm(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	d, err := h.deudaSvc.GetByID(r.Context(), id, uid)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	now := time.Now()
	periodoActual := now.Format("2006-01")
	fechaDefault := now.Format("2006-01-02")
	mesCerrado := false
	var periodoCerrado, mesSugerido string
	if meses, _ := h.mesSvc.List(r.Context(), uid); len(meses) > 0 {
		if m := mesByPeriodo(meses, periodoActual); m != nil && m.Estado == "cerrado" {
			mesCerrado = true
			periodoCerrado = periodoActual
			if sugerido := primerMesAbierto(meses, periodoActual); sugerido != "" {
				mesSugerido = sugerido
				fechaDefault = fechaEnMes(sugerido, now)
			}
		}
	}
	renderTemplateFragment(w, "deuda_pago_form", map[string]interface{}{
		"deuda":           d,
		"categorias":      h.categoriasEgreso(r),
		"fecha_default":   fechaDefault,
		"mes_cerrado":     mesCerrado,
		"periodo_cerrado": periodoCerrado,
		"mes_sugerido":    mesSugerido,
	})
}

func mesByPeriodo(meses []model.Mes, periodo string) *model.Mes {
	for i := range meses {
		if meses[i].Periodo == periodo {
			return &meses[i]
		}
	}
	return nil
}

// primerMesAbierto devuelve el primer período abierto estrictamente mayor al
// dado (meses viene ordenado por período desc). Vacío si no existe.
func primerMesAbierto(meses []model.Mes, periodo string) string {
	var best string
	for _, m := range meses {
		if m.Periodo > periodo && m.Estado == "abierto" {
			if best == "" || m.Periodo < best {
				best = m.Periodo
			}
		}
	}
	return best
}

// fechaEnMes devuelve una fecha dentro del mes indicado usando el día de hoy
// (recortado si el mes es más corto). Fallback al primer día.
func fechaEnMes(periodo string, hoy time.Time) string {
	t, err := time.Parse("2006-01", periodo)
	if err != nil {
		return periodo + "-01"
	}
	ultimo := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	dia := hoy.Day()
	if dia > ultimo {
		dia = ultimo
	}
	return fmt.Sprintf("%04d-%02d-%02d", t.Year(), t.Month(), dia)
}
