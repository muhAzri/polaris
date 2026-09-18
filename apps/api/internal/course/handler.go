package course

import (
	"encoding/json"
	"net/http"
	"strings"

	"polaris-api/internal/auth"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type createRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	c, err := h.service.Create(r.Context(), userID, req.Title, req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create course")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	courses, err := h.service.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list courses")
		return
	}
	writeJSON(w, http.StatusOK, courses)
}

func (h *Handler) ListSections(w http.ResponseWriter, r *http.Request) {
	courseID := r.PathValue("id")
	sections, err := h.service.ListSections(r.Context(), courseID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list sections")
		return
	}
	writeJSON(w, http.StatusOK, sections)
}

func (h *Handler) Enroll(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	courseID := r.PathValue("id")
	if err := h.service.Enroll(r.Context(), courseID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not enroll")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
