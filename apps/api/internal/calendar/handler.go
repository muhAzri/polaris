package calendar

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"polaris-api/internal/auth"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// parseRange reads the ?from=&to= RFC3339 query params every list endpoint
// takes — a month view is just this called with the month's bounds.
func parseRange(r *http.Request) (from, to time.Time, err error) {
	from, err = time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	if err != nil {
		return
	}
	to, err = time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	return
}

type eventRequest struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	StartAt     time.Time  `json:"start_at"`
	EndAt       *time.Time `json:"end_at"`
}

func (h *Handler) CreateCourseEvent(w http.ResponseWriter, r *http.Request) {
	var req eventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || req.StartAt.IsZero() {
		writeError(w, http.StatusBadRequest, "name and start_at are required")
		return
	}

	e, err := h.service.CreateCourseEvent(r.Context(), r.PathValue("id"), req.Name, req.Description, req.StartAt, req.EndAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create event")
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (h *Handler) ListCourseEvents(w http.ResponseWriter, r *http.Request) {
	from, to, err := parseRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "from and to must be RFC3339 timestamps")
		return
	}

	events, err := h.service.ListCourseEvents(r.Context(), r.PathValue("id"), from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list events")
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *Handler) CreateMyEvent(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req eventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || req.StartAt.IsZero() {
		writeError(w, http.StatusBadRequest, "name and start_at are required")
		return
	}

	e, err := h.service.CreateUserEvent(r.Context(), userID, req.Name, req.Description, req.StartAt, req.EndAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create event")
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (h *Handler) ListMyEvents(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	from, to, err := parseRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "from and to must be RFC3339 timestamps")
		return
	}

	events, err := h.service.ListMyEvents(r.Context(), userID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list events")
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *Handler) CreateSiteEvent(w http.ResponseWriter, r *http.Request) {
	var req eventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || req.StartAt.IsZero() {
		writeError(w, http.StatusBadRequest, "name and start_at are required")
		return
	}

	e, err := h.service.CreateSiteEvent(r.Context(), req.Name, req.Description, req.StartAt, req.EndAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create event")
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
