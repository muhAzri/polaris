package course

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"polaris-api/internal/auth"
	"polaris-api/internal/coursemodule"
	"polaris-api/internal/rbac"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Create makes a course. The create permission is checked here rather than
// in route middleware because it applies at the target category, which is
// in the request body.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var in CourseInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	in.ShortName = strings.TrimSpace(in.ShortName)
	if in.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	allowed, err := h.service.CanCreateIn(r.Context(), userID, in.CategoryID)
	if writeServiceError(w, err, "could not check permissions") {
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "missing capability: course:create")
		return
	}

	c, err := h.service.Create(r.Context(), userID, in)
	if writeServiceError(w, err, "could not create course") {
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	c, err := h.service.Get(r.Context(), r.PathValue("id"))
	if writeServiceError(w, err, "could not load course") {
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var in CourseInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	in.ShortName = strings.TrimSpace(in.ShortName)

	// Moving a course into another category needs permission there too.
	if in.CategoryID != nil {
		allowed, err := h.service.CanCreateIn(r.Context(), userID, in.CategoryID)
		if writeServiceError(w, err, "could not check permissions") {
			return
		}
		if !allowed {
			writeError(w, http.StatusForbidden, "you cannot move courses into that category")
			return
		}
	}

	c, err := h.service.Update(r.Context(), r.PathValue("id"), userID, in)
	if writeServiceError(w, err, "could not update course") {
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if writeServiceError(w, h.service.Delete(r.Context(), r.PathValue("id"), userID), "could not delete course") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	courses, err := h.service.List(r.Context(), userID)
	if writeServiceError(w, err, "could not list courses") {
		return
	}
	writeJSON(w, http.StatusOK, courses)
}

// ListAvailable shows the courses a user may join on their own.
func (h *Handler) ListAvailable(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	courses, err := h.service.ListAvailable(r.Context(), userID)
	if writeServiceError(w, err, "could not list courses") {
		return
	}
	writeJSON(w, http.StatusOK, courses)
}

// RequireAccess gates a course-scoped route on being able to open the
// course. It must run after auth.RequireAuth.
func (h *Handler) RequireAccess(pathParam string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := auth.UserIDFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "not authenticated")
				return
			}
			allowed, err := h.service.CanAccess(r.Context(), userID, r.PathValue(pathParam))
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not check course access")
				return
			}
			if !allowed {
				writeError(w, http.StatusForbidden, "you cannot access this course")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// --- Sections ---

func (h *Handler) CourseIDForSection(r *http.Request, sectionID string) (string, error) {
	return h.service.CourseIDForSection(r.Context(), sectionID)
}

func (h *Handler) ListSections(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	sections, err := h.service.ListSectionsFor(r.Context(), r.PathValue("id"), userID)
	if writeServiceError(w, err, "could not list sections") {
		return
	}
	writeJSON(w, http.StatusOK, sections)
}

type sectionCreateRequest struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

func (h *Handler) CreateSection(w http.ResponseWriter, r *http.Request) {
	var req sectionCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	sec, err := h.service.CreateSection(r.Context(), r.PathValue("id"), strings.TrimSpace(req.Title), req.Summary)
	if writeServiceError(w, err, "could not create section") {
		return
	}
	writeJSON(w, http.StatusCreated, sec)
}

func (h *Handler) UpdateSection(w http.ResponseWriter, r *http.Request) {
	var in SectionInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	sec, err := h.service.UpdateSection(r.Context(), r.PathValue("sectionId"), in)
	if writeServiceError(w, err, "could not update section") {
		return
	}
	writeJSON(w, http.StatusOK, sec)
}

func (h *Handler) DeleteSection(w http.ResponseWriter, r *http.Request) {
	if writeServiceError(w, h.service.DeleteSection(r.Context(), r.PathValue("sectionId")), "could not delete section") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Categories ---

func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	includeHidden, err := h.service.CanManageIn(r.Context(), userID, nil)
	if writeServiceError(w, err, "could not check permissions") {
		return
	}
	list, err := h.service.ListCategories(r.Context(), includeHidden)
	if writeServiceError(w, err, "could not list categories") {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var in CategoryInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Name = strings.TrimSpace(in.Name)

	allowed, err := h.service.CanManageIn(r.Context(), userID, in.ParentID)
	if writeServiceError(w, err, "could not check permissions") {
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "missing capability: category:manage")
		return
	}
	c, err := h.service.CreateCategory(r.Context(), in)
	if writeServiceError(w, err, "could not create category") {
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var in CategoryInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Name = strings.TrimSpace(in.Name)

	if in.ParentID != nil {
		allowed, err := h.service.CanManageIn(r.Context(), userID, in.ParentID)
		if writeServiceError(w, err, "could not check permissions") {
			return
		}
		if !allowed {
			writeError(w, http.StatusForbidden, "you cannot move categories there")
			return
		}
	}
	c, err := h.service.UpdateCategory(r.Context(), r.PathValue("categoryId"), in)
	if writeServiceError(w, err, "could not update category") {
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// DeleteCategory removes a category, moving its courses and subcategories
// to ?move_to= when it has any.
func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var moveTo *string
	if v := r.URL.Query().Get("move_to"); v != "" {
		allowed, err := h.service.CanManageIn(r.Context(), userID, &v)
		if writeServiceError(w, err, "could not check permissions") {
			return
		}
		if !allowed {
			writeError(w, http.StatusForbidden, "you cannot move content into that category")
			return
		}
		moveTo = &v
	}
	if writeServiceError(w, h.service.DeleteCategory(r.Context(), r.PathValue("categoryId"), moveTo), "could not delete category") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Enrolment ---

type selfEnrolRequest struct {
	Key string `json:"key"`
}

// Enroll is self enrolment; it only succeeds when the course allows it.
func (h *Handler) Enroll(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req selfEnrolRequest
	// The body is optional; only courses with a key need it.
	_ = json.NewDecoder(r.Body).Decode(&req)

	err := h.service.EnrolSelf(r.Context(), r.PathValue("id"), userID, req.Key)
	if writeServiceError(w, err, "could not enroll") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// EnrolUser is manual enrolment by someone with enrol:manage.
func (h *Handler) EnrolUser(w http.ResponseWriter, r *http.Request) {
	var in EnrolInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	if in.Email == "" && in.UserID == "" {
		writeError(w, http.StatusBadRequest, "email or user_id is required")
		return
	}
	if writeServiceError(w, h.service.EnrolManual(r.Context(), r.PathValue("id"), in), "could not enrol user") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UpdateEnrolment(w http.ResponseWriter, r *http.Request) {
	var u EnrolmentUpdate
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if writeServiceError(w, h.service.UpdateEnrolment(r.Context(), r.PathValue("id"), r.PathValue("userId"), u), "could not update enrolment") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Unenrol(w http.ResponseWriter, r *http.Request) {
	if writeServiceError(w, h.service.Unenrol(r.Context(), r.PathValue("id"), r.PathValue("userId")), "could not unenrol user") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Participants(w http.ResponseWriter, r *http.Request) {
	participants, err := h.service.Participants(r.Context(), r.PathValue("id"))
	if writeServiceError(w, err, "could not list participants") {
		return
	}
	writeJSON(w, http.StatusOK, participants)
}

func (h *Handler) GetSelfEnrolment(w http.ResponseWriter, r *http.Request) {
	se, err := h.service.GetSelfEnrolment(r.Context(), r.PathValue("id"))
	if writeServiceError(w, err, "could not load self enrolment") {
		return
	}
	writeJSON(w, http.StatusOK, se)
}

func (h *Handler) SetSelfEnrolment(w http.ResponseWriter, r *http.Request) {
	var in SelfEnrolment
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if writeServiceError(w, h.service.SetSelfEnrolment(r.Context(), r.PathValue("id"), in), "could not update self enrolment") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeServiceError maps the package's errors to HTTP statuses and reports
// whether it wrote a response.
func writeServiceError(w http.ResponseWriter, err error, fallback string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrCourseNotFound), errors.Is(err, ErrCategoryNotFound), errors.Is(err, ErrSectionNotFound),
		errors.Is(err, ErrUserNotFound), errors.Is(err, ErrNotEnrolled), errors.Is(err, coursemodule.ErrModuleNotFound),
		errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrSelfEnrolDisabled), errors.Is(err, ErrSelfEnrolClosed), errors.Is(err, ErrEnrolKeyWrong):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrShortNameTaken), errors.Is(err, ErrIDNumberTaken), errors.Is(err, ErrOwnerEnrolment),
		errors.Is(err, ErrCourseFull), errors.Is(err, ErrDefaultCategory), errors.Is(err, ErrCategoryNotEmpty),
		errors.Is(err, ErrGeneralSection):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidFormat), errors.Is(err, ErrInvalidStatus), errors.Is(err, ErrCategoryCycle),
		errors.Is(err, ErrCategoryNameNeeded), errors.Is(err, ErrInvalidPosition), errors.Is(err, rbac.ErrRoleNotFound),
		errors.Is(err, rbac.ErrRoleNotAssignable):
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
