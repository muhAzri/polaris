package gradebook

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"polaris-api/internal/auth"
	"polaris-api/internal/rbac"
)

type Handler struct {
	service *Service
	rbac    *rbac.Service
}

func NewHandler(service *Service, rbacService *rbac.Service) *Handler {
	return &Handler{service: service, rbac: rbacService}
}

func (h *Handler) CourseIDForItem(r *http.Request, itemID string) (string, error) {
	return h.service.CourseIDForItem(r.Context(), itemID)
}

func (h *Handler) CourseIDForCategory(r *http.Request, categoryID string) (string, error) {
	return h.service.CourseIDForCategory(r.Context(), categoryID)
}

// canSeeHidden reports whether the caller may see hidden grade items and
// grades in the course.
func (h *Handler) canSeeHidden(r *http.Request, userID, courseID string) bool {
	contextID, err := h.rbac.ContextID(r.Context(), rbac.ContextLevelCourse, courseID)
	if err != nil {
		return false
	}
	ok, err := h.rbac.Can(r.Context(), userID, "grade:viewhidden", contextID)
	return err == nil && ok
}

func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var in CategoryInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	c, err := h.service.CreateCategory(r.Context(), r.PathValue("id"), in)
	if writeServiceError(w, err, "could not create grade category") {
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	var in CategoryInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	c, err := h.service.UpdateCategory(r.Context(), r.PathValue("categoryId"), in)
	if writeServiceError(w, err, "could not update grade category") {
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	if writeServiceError(w, h.service.DeleteCategory(r.Context(), r.PathValue("categoryId")), "could not delete grade category") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.service.ListCategories(r.Context(), r.PathValue("id"))
	if writeServiceError(w, err, "could not list grade categories") {
		return
	}
	writeJSON(w, http.StatusOK, categories)
}

func (h *Handler) CreateItem(w http.ResponseWriter, r *http.Request) {
	var in ItemInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	item, err := h.service.CreateItem(r.Context(), r.PathValue("id"), nil, in)
	if writeServiceError(w, err, "could not create grade item") {
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	var in ItemInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	item, err := h.service.UpdateItem(r.Context(), r.PathValue("itemId"), in)
	if writeServiceError(w, err, "could not update grade item") {
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	if writeServiceError(w, h.service.DeleteItem(r.Context(), r.PathValue("itemId")), "could not delete grade item") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListItems(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	courseID := r.PathValue("id")
	items, err := h.service.ListItems(r.Context(), courseID, h.canSeeHidden(r, userID, courseID))
	if writeServiceError(w, err, "could not list grade items") {
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) SetGrade(w http.ResponseWriter, r *http.Request) {
	graderID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var in GradeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	g, err := h.service.SetGrade(r.Context(), r.PathValue("itemId"), r.PathValue("userId"), graderID, in)
	if writeServiceError(w, err, "could not set grade") {
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Handler) ListGrades(w http.ResponseWriter, r *http.Request) {
	grades, err := h.service.ListGrades(r.Context(), r.PathValue("itemId"))
	if writeServiceError(w, err, "could not list grades") {
		return
	}
	writeJSON(w, http.StatusOK, grades)
}

func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	entries, err := h.service.History(r.Context(), r.PathValue("itemId"), r.PathValue("userId"))
	if writeServiceError(w, err, "could not load grade history") {
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// MyReport is the caller's own gradebook: hidden items and grades are
// left out unless they may see hidden grades.
func (h *Handler) MyReport(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	courseID := r.PathValue("id")
	report, err := h.service.UserReport(r.Context(), courseID, userID, h.canSeeHidden(r, userID, courseID))
	if writeServiceError(w, err, "could not load your grades") {
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// UserReport is a teacher's view of one student's gradebook.
func (h *Handler) UserReport(w http.ResponseWriter, r *http.Request) {
	report, err := h.service.UserReport(r.Context(), r.PathValue("id"), r.PathValue("userId"), true)
	if writeServiceError(w, err, "could not load the user report") {
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *Handler) GraderReport(w http.ResponseWriter, r *http.Request) {
	report, err := h.service.GraderReport(r.Context(), r.PathValue("id"))
	if writeServiceError(w, err, "could not load the grader report") {
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// writeServiceError maps gradebook errors to HTTP statuses and reports
// whether it wrote a response.
func writeServiceError(w http.ResponseWriter, err error, fallback string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrGradeLocked), errors.Is(err, ErrRootCategory), errors.Is(err, ErrModuleItem):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidGrade), errors.Is(err, ErrInvalidMethod), errors.Is(err, ErrInvalidRange),
		errors.Is(err, ErrCategoryCycle), errors.Is(err, ErrInvalidParent), errors.Is(err, ErrNameRequired),
		errors.Is(err, ErrInvalidDropCount):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, fallback)
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
