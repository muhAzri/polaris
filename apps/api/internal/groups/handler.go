package groups

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

func (h *Handler) CourseIDForGroup(r *http.Request, groupID string) (string, error) {
	return h.service.CourseIDForGroup(r.Context(), groupID)
}

type createRequest struct {
	Name        string `json:"name"`
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
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	g, err := h.service.Create(r.Context(), r.PathValue("id"), userID, req.Name, req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create group")
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.service.List(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list groups")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type addMemberRequest struct {
	UserID string `json:"user_id"`
}

func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	addedBy, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req addMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.UserID) == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	if err := h.service.AddMember(r.Context(), r.PathValue("groupId"), addedBy, req.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not add member")
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
