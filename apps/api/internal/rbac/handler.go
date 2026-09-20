package rbac

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"polaris-api/internal/auth"
)

// Handler exposes role administration and the per-context role assignment
// and override endpoints. The context-scoped handlers take a resolver so
// the same code serves both course and category routes.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.service.ListRoles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list roles")
		return
	}
	writeJSON(w, http.StatusOK, roles)
}

func (h *Handler) ListCapabilities(w http.ResponseWriter, r *http.Request) {
	caps, err := h.service.ListCapabilities(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list capabilities")
		return
	}
	writeJSON(w, http.StatusOK, caps)
}

func (h *Handler) GetRole(w http.ResponseWriter, r *http.Request) {
	role, err := h.service.GetRole(r.Context(), r.PathValue("roleId"))
	if h.writeRoleError(w, err, "could not load role") {
		return
	}
	writeJSON(w, http.StatusOK, role)
}

type createRoleRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	CloneFrom   string `json:"clone_from"`
}

func (h *Handler) CreateRole(w http.ResponseWriter, r *http.Request) {
	var req createRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	role, err := h.service.CreateRole(r.Context(), req.Name, req.DisplayName, req.Description, req.CloneFrom)
	if h.writeRoleError(w, err, "could not create role") {
		return
	}
	writeJSON(w, http.StatusCreated, role)
}

func (h *Handler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	if h.writeRoleError(w, h.service.DeleteRole(r.Context(), r.PathValue("roleId")), "could not delete role") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type permissionRequest struct {
	Permission string `json:"permission"`
}

func (h *Handler) SetRoleCapability(w http.ResponseWriter, r *http.Request) {
	var req permissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	err := h.service.SetRoleCapability(r.Context(), r.PathValue("roleId"), r.PathValue("capability"), req.Permission)
	if h.writeRoleError(w, err, "could not update role") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MyCapabilities lists what the caller can do at the resolved context.
func (h *Handler) MyCapabilities(resolve ContextResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		contextID, err := resolve(r)
		if err != nil {
			writeError(w, http.StatusNotFound, "context not found")
			return
		}
		caps, err := h.service.Capabilities(r.Context(), userID, contextID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not resolve capabilities")
			return
		}
		writeJSON(w, http.StatusOK, caps)
	}
}

func (h *Handler) ListAssignments(resolve ContextResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		contextID, err := resolve(r)
		if err != nil {
			writeError(w, http.StatusNotFound, "context not found")
			return
		}
		list, err := h.service.ListAssignments(r.Context(), contextID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not list role assignments")
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

type assignRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

func (h *Handler) Assign(resolve ContextResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req assignRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.UserID == "" || req.Role == "" {
			writeError(w, http.StatusBadRequest, "user_id and role are required")
			return
		}
		contextID, err := resolve(r)
		if err != nil {
			writeError(w, http.StatusNotFound, "context not found")
			return
		}
		if h.writeRoleError(w, h.service.AssignRole(r.Context(), req.UserID, req.Role, contextID), "could not assign role") {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) Unassign(resolve ContextResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		contextID, err := resolve(r)
		if err != nil {
			writeError(w, http.StatusNotFound, "context not found")
			return
		}
		err = h.service.UnassignRole(r.Context(), r.PathValue("userId"), r.PathValue("role"), contextID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not remove role")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) ListOverrides(resolve ContextResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		contextID, err := resolve(r)
		if err != nil {
			writeError(w, http.StatusNotFound, "context not found")
			return
		}
		list, err := h.service.ListOverrides(r.Context(), contextID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not list overrides")
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

func (h *Handler) SetOverride(resolve ContextResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Override
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		contextID, err := resolve(r)
		if err != nil {
			writeError(w, http.StatusNotFound, "context not found")
			return
		}
		if h.writeRoleError(w, h.service.SetOverride(r.Context(), req.Role, req.Capability, req.Permission, contextID), "could not save override") {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// writeRoleError maps the package's sentinel errors to HTTP statuses and
// reports whether it wrote a response.
func (h *Handler) writeRoleError(w http.ResponseWriter, err error, fallback string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrRoleNotFound), errors.Is(err, ErrCapabilityNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrRoleNameTaken):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrSystemRole), errors.Is(err, ErrRoleNotAssignable):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrInvalidPermission):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, fallback)
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
