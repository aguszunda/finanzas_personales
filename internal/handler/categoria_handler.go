package handler

import (
	"net/http"

	"administracion-financiera/internal/middleware"
	"administracion-financiera/internal/repository"
)

type CategoriaHandler struct {
	repo *repository.CategoriaRepo
}

func NewCategoriaHandler(repo *repository.CategoriaRepo) *CategoriaHandler {
	return &CategoriaHandler{repo: repo}
}

func (h *CategoriaHandler) List(w http.ResponseWriter, r *http.Request) {
	uid := middleware.UserIDFromContext(r.Context())
	cats, err := h.repo.FindAll(r.Context(), uid)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, cats)
}
