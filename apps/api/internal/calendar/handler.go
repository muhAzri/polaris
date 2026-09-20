package calendar

import (
	"encoding/json"
	"errors"
	"net/http"
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

func decodeEvent(w http.ResponseWriter, r *http.Request) (EventInput, bool) {
	var in EventInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return in, false
	}
	return in, true
}

func (h *Handler) CreateCourseEvent(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeEvent(w, r)
	if !ok {
		return
	}
	e, err := h.service.CreateCourseEvent(r.Context(), r.PathValue("id"), in)
	if writeServiceError(w, err, "could not create event") {
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (h *Handler) ListCourseEvents(w http.ResponseWriter, r *http.Request) {
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
	events, err := h.service.ListCourseEvents(r.Context(), r.PathValue("id"), userID, from, to)
	if writeServiceError(w, err, "could not list events") {
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
	in, ok := decodeEvent(w, r)
	if !ok {
		return
	}
	e, err := h.service.CreateUserEvent(r.Context(), userID, in)
	if writeServiceError(w, err, "could not create event") {
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
	if writeServiceError(w, err, "could not list events") {
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *Handler) CreateSiteEvent(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeEvent(w, r)
	if !ok {
		return
	}
	e, err := h.service.CreateSiteEvent(r.Context(), in)
	if writeServiceError(w, err, "could not create event") {
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

// authorizeEdit checks the caller may change the event in the path and
// reports whether the request may continue.
func (h *Handler) authorizeEdit(w http.ResponseWriter, r *http.Request) bool {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return false
	}
	allowed, err := h.service.CanEdit(r.Context(), userID, r.PathValue("eventId"))
	if writeServiceError(w, err, "could not check permissions") {
		return false
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "you cannot change this event")
		return false
	}
	return true
}

func (h *Handler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeEdit(w, r) {
		return
	}
	in, ok := decodeEvent(w, r)
	if !ok {
		return
	}
	e, err := h.service.Update(r.Context(), r.PathValue("eventId"), in)
	if writeServiceError(w, err, "could not update event") {
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *Handler) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeEdit(w, r) {
		return
	}
	if writeServiceError(w, h.service.Delete(r.Context(), r.PathValue("eventId")), "could not delete event") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ExportICS downloads the caller's calendar as an .ics file.
func (h *Handler) ExportICS(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	body, err := h.service.ExportICS(r.Context(), userID)
	if writeServiceError(w, err, "could not export calendar") {
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="polaris.ics"`)
	w.Write([]byte(body))
}

func writeServiceError(w http.ResponseWriter, err error, fallback string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrManagedByModule):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNameRequired), errors.Is(err, ErrInvalidRepeat), errors.Is(err, ErrInvalidRange):
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
