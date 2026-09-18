package gradebook

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

func (h *Handler) CourseIDForItem(r *http.Request, itemID string) (string, error) {
	return h.service.CourseIDForItem(r.Context(), itemID)
}

type categoryRequest struct {
	Name        string `json:"name"`
	Aggregation string `json:"aggregation"`
}

func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req categoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Aggregation != "" && req.Aggregation != "mean" && req.Aggregation != "weighted" {
		writeError(w, http.StatusBadRequest, "aggregation must be 'mean' or 'weighted'")
		return
	}

	c, err := h.service.CreateCategory(r.Context(), r.PathValue("id"), req.Name, req.Aggregation)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create grade category")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.service.ListCategories(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list grade categories")
		return
	}
	writeJSON(w, http.StatusOK, categories)
}

type itemRequest struct {
	CategoryID string   `json:"category_id"`
	Name       string   `json:"name"`
	MaxGrade   *float64 `json:"max_grade"`
	Weight     *float64 `json:"weight"`
}

func (h *Handler) CreateItem(w http.ResponseWriter, r *http.Request) {
	var req itemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	maxGrade := 100.0
	if req.MaxGrade != nil {
		maxGrade = *req.MaxGrade
	}

	item, err := h.service.CreateItem(r.Context(), r.PathValue("id"), req.CategoryID, nil, req.Name, maxGrade, req.Weight)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create grade item")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListItems(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list grade items")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

type gradeRequest struct {
	Grade    *float64 `json:"grade"`
	Feedback string   `json:"feedback"`
}

func (h *Handler) SetGrade(w http.ResponseWriter, r *http.Request) {
	graderID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req gradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	g, err := h.service.SetGrade(r.Context(), r.PathValue("itemId"), r.PathValue("userId"), graderID, req.Grade, req.Feedback)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not set grade")
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Handler) ListGrades(w http.ResponseWriter, r *http.Request) {
	grades, err := h.service.ListGrades(r.Context(), r.PathValue("itemId"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list grades")
		return
	}
	writeJSON(w, http.StatusOK, grades)
}

func (h *Handler) ListMyGrades(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	grades, err := h.service.ListMyGrades(r.Context(), r.PathValue("id"), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list your grades")
		return
	}
	writeJSON(w, http.StatusOK, grades)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
