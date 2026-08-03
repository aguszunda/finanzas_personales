package handler

import (
	"net/http"
	"strconv"

	"finanzas_personales/internal/middleware"
	"finanzas_personales/internal/service"

	"github.com/go-chi/chi/v5"
)

type CostoFijoHandler struct {
	svc *service.CostoFijoService
}

func NewCostoFijoHandler(svc *service.CostoFijoService) *CostoFijoHandler {
	return &CostoFijoHandler{svc: svc}
}

func (h *CostoFijoHandler) List(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	list, err := h.svc.List(r.Context(), uid)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, list)
}

func (h *CostoFijoHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	c, err := h.svc.GetByID(r.Context(), id, uid)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, c)
}

func (h *CostoFijoHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	var input service.CreateCostoFijoInput
	if err := decodeBody(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "datos inválidos")
		return
	}
	c, err := h.svc.Create(r.Context(), uid, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondMutation(w, r, http.StatusCreated, c, "/api/costos-fijos/page")
}

func (h *CostoFijoHandler) Update(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var input service.CreateCostoFijoInput
	if err := decodeBody(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "datos inválidos")
		return
	}
	c, err := h.svc.Update(r.Context(), uid, id, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondMutation(w, r, http.StatusOK, c, "/api/costos-fijos/page")
}

func (h *CostoFijoHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	c, err := h.svc.ToggleActivo(r.Context(), uid, id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondMutation(w, r, http.StatusOK, c, "/api/costos-fijos/page")
}

func (h *CostoFijoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := h.svc.Delete(r.Context(), uid, id); err != nil {
		handleServiceError(w, err)
		return
	}
	if middleware.IsHTMXRequest(r.Context()) {
		respondHTMXRedirect(w, http.StatusOK, "/api/costos-fijos/page")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
