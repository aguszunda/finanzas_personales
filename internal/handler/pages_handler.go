package handler

import (
	"net/http"
	"strconv"

	"finanzas_personales/internal/middleware"
	"finanzas_personales/internal/model"
	"finanzas_personales/internal/repository"
	"finanzas_personales/internal/service"

	"github.com/go-chi/chi/v5"
)

type PagesHandler struct {
	dashboardSvc *service.DashboardService
	transSvc     *service.TransaccionService
	cfSvc        *service.CostoFijoService
	mesSvc       *service.MesService
	deudaSvc     *service.DeudaService
	catRepo      *repository.CategoriaRepo
}

func NewPagesHandler(ds *service.DashboardService, ts *service.TransaccionService, cfs *service.CostoFijoService, ms *service.MesService, ds2 *service.DeudaService, cr *repository.CategoriaRepo) *PagesHandler {
	return &PagesHandler{
		dashboardSvc: ds, transSvc: ts, cfSvc: cfs, mesSvc: ms, deudaSvc: ds2, catRepo: cr,
	}
}

func (h *PagesHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "login", nil)
}

func (h *PagesHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "register", nil)
}

func (h *PagesHandler) DashboardPage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	data, err := h.dashboardSvc.GetDashboard(r.Context(), uid)
	if err != nil {
		renderTemplate(w, "dashboard", map[string]interface{}{"error": err.Error()})
		return
	}
	renderTemplate(w, "dashboard", map[string]interface{}{"error": "", "d": data})
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
		renderTemplate(w, "transacciones", map[string]interface{}{"error": err.Error()})
		return
	}
	cats, _ := h.catRepo.FindAll(r.Context(), uid)
	meses, _ := h.mesSvc.List(r.Context(), uid)
	renderTemplate(w, "transacciones", map[string]interface{}{
		"transacciones": transacciones,
		"categorias":    cats,
		"meses":         meses,
		"periodo":       periodo,
	})
}

func (h *PagesHandler) CostosFijosPage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	list, err := h.cfSvc.List(r.Context(), uid)
	if err != nil {
		renderTemplate(w, "costos_fijos", map[string]interface{}{"error": err.Error()})
		return
	}
	cats, _ := h.catRepo.FindAll(r.Context(), uid)
	renderTemplate(w, "costos_fijos", map[string]interface{}{
		"costos_fijos": list,
		"categorias":   cats,
	})
}

func (h *PagesHandler) MesesPage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	meses, err := h.mesSvc.List(r.Context(), uid)
	if err != nil {
		renderTemplate(w, "meses", map[string]interface{}{"error": err.Error()})
		return
	}
	renderTemplate(w, "meses", map[string]interface{}{"meses": meses})
}

func (h *PagesHandler) BalancePage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	mes, transacciones, err := h.mesSvc.Balance(r.Context(), uid, id)
	if err != nil {
		renderTemplate(w, "balance", map[string]interface{}{"error": err.Error()})
		return
	}
	deudas, err := h.deudaSvc.List(r.Context(), uid)
	if err != nil {
		renderTemplate(w, "balance", map[string]interface{}{"error": err.Error()})
		return
	}
	renderTemplate(w, "balance", map[string]interface{}{
		"mes":           mes,
		"transacciones": transacciones,
		"deudas":        deudas,
	})
}

func (h *PagesHandler) DeudasPage(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	deudas, err := h.deudaSvc.List(r.Context(), uid)
	if err != nil {
		renderTemplate(w, "deudas", map[string]interface{}{"error": err.Error()})
		return
	}
	renderTemplate(w, "deudas", map[string]interface{}{"deudas": deudas})
}
