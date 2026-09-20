package groups

import (
	"encoding/json"
	"errors"
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

func (h *Handler) CourseIDForGroup(r *http.Request, groupID string) (string, error) {
	return h.service.CourseIDForGroup(r.Context(), groupID)
}

func (h *Handler) CourseIDForGrouping(r *http.Request, groupingID string) (string, error) {
	return h.service.CourseIDForGrouping(r.Context(), groupingID)
}

func decodeGroup(w http.ResponseWriter, r *http.Request) (GroupInput, bool) {
	var in GroupInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return in, false
	}
	in.Name = strings.TrimSpace(in.Name)
	return in, true
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	in, ok := decodeGroup(w, r)
	if !ok {
		return
	}
	g, err := h.service.Create(r.Context(), r.PathValue("id"), userID, in)
	if writeServiceError(w, err, "could not create group") {
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeGroup(w, r)
	if !ok {
		return
	}
	g, err := h.service.Update(r.Context(), r.PathValue("groupId"), in)
	if writeServiceError(w, err, "could not update group") {
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if writeServiceError(w, h.service.Delete(r.Context(), r.PathValue("groupId")), "could not delete group") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.List(r.Context(), r.PathValue("id"))
	if writeServiceError(w, err, "could not list groups") {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) AutoCreate(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var in AutoCreateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	list, err := h.service.AutoCreate(r.Context(), r.PathValue("id"), userID, in)
	if writeServiceError(w, err, "could not create groups") {
		return
	}
	writeJSON(w, http.StatusCreated, list)
}

type memberRequest struct {
	UserID string `json:"user_id"`
}

func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	addedBy, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req memberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.UserID) == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if writeServiceError(w, h.service.AddMember(r.Context(), r.PathValue("groupId"), addedBy, req.UserID), "could not add member") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	removedBy, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if writeServiceError(w, h.service.RemoveMember(r.Context(), r.PathValue("groupId"), removedBy, r.PathValue("userId")), "could not remove member") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CreateGrouping(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeGroup(w, r)
	if !ok {
		return
	}
	g, err := h.service.CreateGrouping(r.Context(), r.PathValue("id"), in)
	if writeServiceError(w, err, "could not create grouping") {
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (h *Handler) ListGroupings(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.ListGroupings(r.Context(), r.PathValue("id"))
	if writeServiceError(w, err, "could not list groupings") {
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) UpdateGrouping(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeGroup(w, r)
	if !ok {
		return
	}
	if writeServiceError(w, h.service.UpdateGrouping(r.Context(), r.PathValue("groupingId"), in), "could not update grouping") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteGrouping(w http.ResponseWriter, r *http.Request) {
	if writeServiceError(w, h.service.DeleteGrouping(r.Context(), r.PathValue("groupingId")), "could not delete grouping") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type groupingGroupRequest struct {
	GroupID string `json:"group_id"`
}

func (h *Handler) AddGroupToGrouping(w http.ResponseWriter, r *http.Request) {
	var req groupingGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.GroupID == "" {
		writeError(w, http.StatusBadRequest, "group_id is required")
		return
	}
	if writeServiceError(w, h.service.AddGroupToGrouping(r.Context(), r.PathValue("groupingId"), req.GroupID), "could not add group to grouping") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) RemoveGroupFromGrouping(w http.ResponseWriter, r *http.Request) {
	if writeServiceError(w, h.service.RemoveGroupFromGrouping(r.Context(), r.PathValue("groupingId"), r.PathValue("groupId")), "could not remove group from grouping") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeServiceError(w http.ResponseWriter, err error, fallback string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrNameTaken):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNotEnrolled), errors.Is(err, ErrNameRequired), errors.Is(err, ErrInvalidAuto),
		errors.Is(err, ErrCrossCourse), errors.Is(err, ErrNoStudentsToUse):
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
