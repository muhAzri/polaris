package httpserver

import (
	"encoding/json"
	"net/http"

	"polaris-api/internal/auth"
	"polaris-api/internal/content"
	"polaris-api/internal/course"
	"polaris-api/internal/rbac"
)

type Deps struct {
	AuthHandler    *auth.Handler
	CourseHandler  *course.Handler
	ContentHandler *content.Handler
	RBACService    *rbac.Service
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
	// checks — RequireAuth resolves the user, RequireCapability resolves
	// the context and checks the capability.
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

	// The 6 static content modules. Reads are public like course/section
	// listing above; writes need mod:manage at the owning course's context
	// — resolved via the section/module id in the path since neither route
	// carries a course id directly.
	mux.HandleFunc("GET /api/v1/courses/{id}/content", deps.ContentHandler.CourseContent)

	requireModManageOnSection := func(next http.Handler) http.Handler {
		return deps.AuthHandler.RequireAuth(
			deps.RBACService.RequireCapability("mod:manage", sectionCourseContext(deps))(next),
		)
	}
	requireModManageOnModule := func(next http.Handler) http.Handler {
		return deps.AuthHandler.RequireAuth(
			deps.RBACService.RequireCapability("mod:manage", moduleCourseContext(deps))(next),
		)
	}

	mux.Handle("POST /api/v1/sections/{sectionId}/modules/label", requireModManageOnSection(http.HandlerFunc(deps.ContentHandler.CreateLabel)))
	mux.Handle("POST /api/v1/sections/{sectionId}/modules/page", requireModManageOnSection(http.HandlerFunc(deps.ContentHandler.CreatePage)))
	mux.Handle("POST /api/v1/sections/{sectionId}/modules/url", requireModManageOnSection(http.HandlerFunc(deps.ContentHandler.CreateURL)))
	mux.Handle("POST /api/v1/sections/{sectionId}/modules/resource", requireModManageOnSection(http.HandlerFunc(deps.ContentHandler.CreateResource)))
	mux.Handle("POST /api/v1/sections/{sectionId}/modules/folder", requireModManageOnSection(http.HandlerFunc(deps.ContentHandler.CreateFolder)))
	mux.Handle("POST /api/v1/sections/{sectionId}/modules/book", requireModManageOnSection(http.HandlerFunc(deps.ContentHandler.CreateBook)))

	mux.Handle("POST /api/v1/modules/{moduleId}/folder-files", requireModManageOnModule(http.HandlerFunc(deps.ContentHandler.AddFolderFile)))
	mux.Handle("POST /api/v1/modules/{moduleId}/book-chapters", requireModManageOnModule(http.HandlerFunc(deps.ContentHandler.AddBookChapter)))

	return withCORS(mux)
}

// sectionCourseContext/moduleCourseContext resolve the RBAC context for
// routes keyed by a section or module id rather than a course id directly —
// mod:manage still applies at the course context, so it just needs one
// extra lookup to find which course owns the section/module in the path.
func sectionCourseContext(deps Deps) rbac.ContextResolver {
	return func(r *http.Request) (string, error) {
		courseID, err := deps.ContentHandler.CourseIDForSection(r, r.PathValue("sectionId"))
		if err != nil {
			return "", err
		}
		return deps.RBACService.ContextID(r.Context(), rbac.ContextLevelCourse, courseID)
	}
}

func moduleCourseContext(deps Deps) rbac.ContextResolver {
	return func(r *http.Request) (string, error) {
		courseID, err := deps.ContentHandler.CourseIDForModule(r, r.PathValue("moduleId"))
		if err != nil {
			return "", err
		}
		return deps.RBACService.ContextID(r.Context(), rbac.ContextLevelCourse, courseID)
	}
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
