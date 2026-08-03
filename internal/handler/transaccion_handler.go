package handler

import (
	"net/http"
	"strconv"

	"finanzas_personales/internal/middleware"
	"finanzas_personales/internal/service"

	"github.com/go-chi/chi/v5"
)

type TransaccionHandler struct {
	svc *service.TransaccionService
}

func NewTransaccionHandler(svc *service.TransaccionService) *TransaccionHandler {
	return &TransaccionHandler{svc: svc}
}

func (h *TransaccionHandler) List(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	periodo := r.URL.Query().Get("periodo")
	var transacciones interface{}
	var err error
	if periodo != "" {
		transacciones, err = h.svc.ListByPeriodo(r.Context(), uid, periodo)
	} else {
		transacciones, err = h.svc.List(r.Context(), uid, limit, offset)
	}
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, transacciones)
}

func (h *TransaccionHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	t, err := h.svc.GetByID(r.Context(), id, uid)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, t)
}

func (h *TransaccionHandler) Create(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	var input service.CreateTransaccionInput
	if err := decodeBody(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "datos inválidos")
		return
	}
	t, err := h.svc.Create(r.Context(), uid, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondMutation(w, r, http.StatusCreated, t, "/api/transacciones/page")
}

func (h *TransaccionHandler) Update(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var input service.CreateTransaccionInput
	if err := decodeBody(r, &input); err != nil {
		respondError(w, http.StatusBadRequest, "datos inválidos")
		return
	}
	t, err := h.svc.Update(r.Context(), uid, id, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondMutation(w, r, http.StatusOK, t, "/api/transacciones/page")
}

func (h *TransaccionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := h.svc.Delete(r.Context(), uid, id); err != nil {
		handleServiceError(w, err)
		return
	}
	if middleware.IsHTMXRequest(r.Context()) {
		respondHTMXRedirect(w, http.StatusOK, "/api/transacciones/page")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
