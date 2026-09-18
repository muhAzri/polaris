package httpserver

import (
	"encoding/json"
	"net/http"

	"polaris-api/internal/auth"
	"polaris-api/internal/course"
	"polaris-api/internal/rbac"
)

type Deps struct {
	AuthHandler   *auth.Handler
	CourseHandler *course.Handler
	RBACService   *rbac.Service
}

func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", healthCheck)

	mux.HandleFunc("POST /api/v1/auth/register", deps.AuthHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", deps.AuthHandler.Login)
	mux.Handle("GET /api/v1/auth/me", deps.AuthHandler.RequireAuth(http.HandlerFunc(deps.AuthHandler.Me)))

	mux.HandleFunc("GET /api/v1/courses", deps.CourseHandler.List)
	mux.HandleFunc("GET /api/v1/courses/{id}/sections", deps.CourseHandler.ListSections)

	// course:manage / course:enrol replace what used to be ad-hoc role
	// checks (ROADMAP.md section 2.1) — RequireAuth resolves the user,
	// RequireCapability resolves the context and checks the capability.
	mux.Handle("POST /api/v1/courses", deps.AuthHandler.RequireAuth(
		deps.RBACService.RequireCapability("course:manage", func(r *http.Request) (string, error) {
			return deps.RBACService.SystemContextID(r.Context())
		})(http.HandlerFunc(deps.CourseHandler.Create)),
	))
	mux.Handle("POST /api/v1/courses/{id}/enroll", deps.AuthHandler.RequireAuth(
		deps.RBACService.RequireCapability("course:enrol", func(r *http.Request) (string, error) {
			return deps.RBACService.ContextID(r.Context(), rbac.ContextLevelCourse, r.PathValue("id"))
		})(http.HandlerFunc(deps.CourseHandler.Enroll)),
	))

	return withCORS(mux)
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
