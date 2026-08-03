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
	catRepo      *repository.CategoriaRepo
}

func NewPagesHandler(ds *service.DashboardService, ts *service.TransaccionService, cfs *service.CostoFijoService, ms *service.MesService, cr *repository.CategoriaRepo) *PagesHandler {
	return &PagesHandler{
		dashboardSvc: ds, transSvc: ts, cfSvc: cfs, mesSvc: ms, catRepo: cr,
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
	var mes *model.Mes
	var err error
	if id > 0 {
		mes, err = h.mesSvc.GetByID(r.Context(), id, uid)
	} else {
		mes, err = h.mesSvc.GetCurrent(r.Context(), uid)
	}
	if err != nil {
		renderTemplate(w, "balance", map[string]interface{}{"error": err.Error()})
		return
	}
	transacciones, err := h.transSvc.ListByPeriodo(r.Context(), uid, mes.Periodo)
	if err != nil {
		transacciones = []model.Transaccion{}
	}
	renderTemplate(w, "balance", map[string]interface{}{
		"mes":           mes,
		"transacciones": transacciones,
	})
}
