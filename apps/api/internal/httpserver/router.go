package httpserver

import (
	"encoding/json"
	"net/http"

	"polaris-api/internal/auth"
	"polaris-api/internal/calendar"
	"polaris-api/internal/content"
	"polaris-api/internal/course"
	"polaris-api/internal/gradebook"
	"polaris-api/internal/groups"
	"polaris-api/internal/rbac"
)

type Deps struct {
	AuthHandler      *auth.Handler
	CourseHandler    *course.Handler
	ContentHandler   *content.Handler
	GradebookHandler *gradebook.Handler
	GroupsHandler    *groups.Handler
	CalendarHandler  *calendar.Handler
	RBACService      *rbac.Service
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

	// Gradebook. Category/item structure is gated by grade:view (both
	// roles hold it); entering grades and seeing the whole class's grades
	// needs grade:manage (teacher/admin only). "My grades" only needs
	// grade:view since the handler scopes the query to the requesting user
	// itself.
	mux.Handle("POST /api/v1/courses/{id}/gradebook/categories", requireCapability(deps, "grade:manage", courseContext(deps, "id"))(http.HandlerFunc(deps.GradebookHandler.CreateCategory)))
	mux.Handle("GET /api/v1/courses/{id}/gradebook/categories", requireCapability(deps, "grade:view", courseContext(deps, "id"))(http.HandlerFunc(deps.GradebookHandler.ListCategories)))
	mux.Handle("POST /api/v1/courses/{id}/gradebook/items", requireCapability(deps, "grade:manage", courseContext(deps, "id"))(http.HandlerFunc(deps.GradebookHandler.CreateItem)))
	mux.Handle("GET /api/v1/courses/{id}/gradebook/items", requireCapability(deps, "grade:view", courseContext(deps, "id"))(http.HandlerFunc(deps.GradebookHandler.ListItems)))
	mux.Handle("GET /api/v1/courses/{id}/gradebook/mine", requireCapability(deps, "grade:view", courseContext(deps, "id"))(http.HandlerFunc(deps.GradebookHandler.ListMyGrades)))
	mux.Handle("POST /api/v1/gradebook/items/{itemId}/grades/{userId}", requireCapability(deps, "grade:manage", itemCourseContext(deps))(http.HandlerFunc(deps.GradebookHandler.SetGrade)))
	mux.Handle("GET /api/v1/gradebook/items/{itemId}/grades", requireCapability(deps, "grade:manage", itemCourseContext(deps))(http.HandlerFunc(deps.GradebookHandler.ListGrades)))

	// Groups. Reads public like course/section listing; writes need
	// group:manage at the owning course's context.
	mux.Handle("POST /api/v1/courses/{id}/groups", requireCapability(deps, "group:manage", courseContext(deps, "id"))(http.HandlerFunc(deps.GroupsHandler.Create)))
	mux.HandleFunc("GET /api/v1/courses/{id}/groups", deps.GroupsHandler.List)
	mux.Handle("POST /api/v1/groups/{groupId}/members", requireCapability(deps, "group:manage", groupCourseContext(deps))(http.HandlerFunc(deps.GroupsHandler.AddMember)))

	// Calendar. Course events reuse course:manage; personal events only
	// need auth (self-service, no capability check); site events are
	// gated by calendar:manage, which only admin holds.
	mux.Handle("POST /api/v1/courses/{id}/events", requireCapability(deps, "course:manage", courseContext(deps, "id"))(http.HandlerFunc(deps.CalendarHandler.CreateCourseEvent)))
	mux.HandleFunc("GET /api/v1/courses/{id}/events", deps.CalendarHandler.ListCourseEvents)
	mux.Handle("POST /api/v1/users/me/events", deps.AuthHandler.RequireAuth(http.HandlerFunc(deps.CalendarHandler.CreateMyEvent)))
	mux.Handle("GET /api/v1/users/me/events", deps.AuthHandler.RequireAuth(http.HandlerFunc(deps.CalendarHandler.ListMyEvents)))
	mux.Handle("POST /api/v1/admin/events", requireCapability(deps, "calendar:manage", func(r *http.Request) (string, error) {
		return deps.RBACService.SystemContextID(r.Context())
	})(http.HandlerFunc(deps.CalendarHandler.CreateSiteEvent)))

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

// requireCapability composes RequireAuth + RequireCapability the way every
// route above already does inline — factored out here once enough routes
// needed it that repeating the two-level closure per route stopped being
// readable.
func requireCapability(deps Deps, capability string, resolve rbac.ContextResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return deps.AuthHandler.RequireAuth(
			deps.RBACService.RequireCapability(capability, resolve)(next),
		)
	}
}

// courseContext resolves the RBAC context straight from a course id already
// in the path (the {id} route param), same as course:manage/course:enrol
// above.
func courseContext(deps Deps, pathParam string) rbac.ContextResolver {
	return func(r *http.Request) (string, error) {
		return deps.RBACService.ContextID(r.Context(), rbac.ContextLevelCourse, r.PathValue(pathParam))
	}
}

func itemCourseContext(deps Deps) rbac.ContextResolver {
	return func(r *http.Request) (string, error) {
		courseID, err := deps.GradebookHandler.CourseIDForItem(r, r.PathValue("itemId"))
		if err != nil {
			return "", err
		}
		return deps.RBACService.ContextID(r.Context(), rbac.ContextLevelCourse, courseID)
	}
}

func groupCourseContext(deps Deps) rbac.ContextResolver {
	return func(r *http.Request) (string, error) {
		courseID, err := deps.GroupsHandler.CourseIDForGroup(r, r.PathValue("groupId"))
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
