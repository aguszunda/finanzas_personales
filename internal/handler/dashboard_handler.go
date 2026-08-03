package handler

import (
	"net/http"

	"administracion-financiera/internal/middleware"
	"administracion-financiera/internal/service"
)

type DashboardHandler struct {
	svc *service.DashboardService
}

func NewDashboardHandler(svc *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

func (h *DashboardHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	data, err := h.svc.GetDashboard(r.Context(), uid)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, data)
}
