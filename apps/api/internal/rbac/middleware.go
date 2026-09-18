package rbac

import (
	"encoding/json"
	"net/http"

	"polaris-api/internal/auth"
)

// ContextResolver derives the context id a capability check should run
// against for a given request, e.g. the system context for creating a
// course, or a specific course's context for enrolling into it.
type ContextResolver func(r *http.Request) (string, error)

// RequireCapability replaces ad-hoc "if role == ..." checks with a
// capability lookup. It must run after auth.RequireAuth so the user id is
// already in the request context.
func (s *Service) RequireCapability(capability string, resolveContext ContextResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := auth.UserIDFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "not authenticated")
				return
			}

			contextID, err := resolveContext(r)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid context")
				return
			}

			allowed, err := s.Can(r.Context(), userID, capability, contextID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "could not check permissions")
				return
			}
			if !allowed {
				writeError(w, http.StatusForbidden, "missing capability: "+capability)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
