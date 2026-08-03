package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"administracion-financiera/internal/middleware"
	"administracion-financiera/internal/service"

	"github.com/go-chi/chi/v5"
)

type MesHandler struct {
	svc *service.MesService
}

func NewMesHandler(svc *service.MesService) *MesHandler {
	return &MesHandler{svc: svc}
}

func (h *MesHandler) List(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	list, err := h.svc.List(r.Context(), uid)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, list)
}

func (h *MesHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	m, err := h.svc.GetByID(r.Context(), id, uid)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, m)
}

func (h *MesHandler) Current(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	m, err := h.svc.GetCurrent(r.Context(), uid)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, m)
}

func (h *MesHandler) Cerrar(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	m, err := h.svc.Cerrar(r.Context(), uid, id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondMutation(w, r, http.StatusOK, m, fmt.Sprintf("/api/balance/%d/page", id))
}

func (h *MesHandler) Recalcular(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	m, err := h.svc.Recalcular(r.Context(), uid, id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondMutation(w, r, http.StatusOK, m, fmt.Sprintf("/api/balance/%d/page", id))
}
