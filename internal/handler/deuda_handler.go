package handler

import (
	"net/http"
	"strconv"

	"finanzas_personales/internal/middleware"
	"finanzas_personales/internal/service"

	"github.com/go-chi/chi/v5"
)

type DeudaHandler struct {
	svc *service.DeudaService
}

func NewDeudaHandler(svc *service.DeudaService) *DeudaHandler {
	return &DeudaHandler{svc: svc}
}

func (h *DeudaHandler) List(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	list, err := h.svc.List(r.Context(), uid)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, list)
}

func (h *DeudaHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	d, err := h.svc.GetByID(r.Context(), id, uid)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, d)
}

func (h *DeudaHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	var input service.CreateDeudaInput
	if err := decodeBody(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "datos inválidos")
		return
	}
	d, err := h.svc.Create(r.Context(), uid, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondMutation(w, r, http.StatusCreated, d, "/api/deudas/page")
}

func (h *DeudaHandler) Update(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var input service.CreateDeudaInput
	if err := decodeBody(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "datos inválidos")
		return
	}
	d, err := h.svc.Update(r.Context(), uid, id, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondMutation(w, r, http.StatusOK, d, "/api/deudas/page")
}

func (h *DeudaHandler) Delete(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := h.svc.Delete(r.Context(), uid, id); err != nil {
		handleServiceError(w, err)
		return
	}
	if middleware.IsHTMXRequest(r.Context()) {
		respondHTMXRedirect(w, http.StatusOK, "/api/deudas/page")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
