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
	RBACHandler      *rbac.Handler
	RBACService      *rbac.Service
}

func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()

	// route registers a handler behind auth and, when capability is set, a
	// capability check at the context the resolver returns. Every route
	// below goes through it, so no handler does its own role checks.
	route := func(pattern, capability string, resolve rbac.ContextResolver, h http.HandlerFunc) {
		var handler http.Handler = h
		if capability != "" {
			handler = deps.RBACService.RequireCapability(capability, resolve)(handler)
		}
		mux.Handle(pattern, deps.AuthHandler.RequireAuth(handler))
	}
	// member additionally requires being able to open the course in {id}.
	member := func(pattern, capability string, h http.HandlerFunc) {
		var handler http.Handler = h
		if capability != "" {
			handler = deps.RBACService.RequireCapability(capability, courseContext(deps, "id"))(handler)
		}
		mux.Handle(pattern, deps.AuthHandler.RequireAuth(deps.CourseHandler.RequireAccess("id")(handler)))
	}
	system := func(r *http.Request) (string, error) { return deps.RBACService.SystemContextID(r.Context()) }

	mux.HandleFunc("GET /api/v1/health", healthCheck)

	mux.HandleFunc("POST /api/v1/auth/register", deps.AuthHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", deps.AuthHandler.Login)
	mux.Handle("GET /api/v1/auth/me", deps.AuthHandler.RequireAuth(http.HandlerFunc(deps.AuthHandler.Me)))

	// --- Courses ---
	// Access is membership-based, like Moodle: a user opens the courses they
	// are actively enrolled in, plus any their site or category role
	// reaches. Creating a course is checked in the handler, against the
	// category it is created in.
	route("GET /api/v1/courses", "", nil, deps.CourseHandler.List)
	route("GET /api/v1/courses/available", "", nil, deps.CourseHandler.ListAvailable)
	route("POST /api/v1/courses", "", nil, deps.CourseHandler.Create)
	member("GET /api/v1/courses/{id}", "", deps.CourseHandler.Get)
	route("PUT /api/v1/courses/{id}", "course:update", courseContext(deps, "id"), deps.CourseHandler.Update)
	route("DELETE /api/v1/courses/{id}", "course:delete", courseContext(deps, "id"), deps.CourseHandler.Delete)
	member("GET /api/v1/courses/{id}/my-capabilities", "", deps.RBACHandler.MyCapabilities(courseContext(deps, "id")))

	// Sections.
	member("GET /api/v1/courses/{id}/sections", "", deps.CourseHandler.ListSections)
	route("POST /api/v1/courses/{id}/sections", "course:update", courseContext(deps, "id"), deps.CourseHandler.CreateSection)
	route("PATCH /api/v1/sections/{sectionId}", "course:update", sectionContext(deps), deps.CourseHandler.UpdateSection)
	route("DELETE /api/v1/sections/{sectionId}", "course:update", sectionContext(deps), deps.CourseHandler.DeleteSection)

	// Categories.
	route("GET /api/v1/categories", "", nil, deps.CourseHandler.ListCategories)
	route("POST /api/v1/categories", "", nil, deps.CourseHandler.CreateCategory)
	route("PUT /api/v1/categories/{categoryId}", "category:manage", categoryContext(deps), deps.CourseHandler.UpdateCategory)
	route("DELETE /api/v1/categories/{categoryId}", "category:manage", categoryContext(deps), deps.CourseHandler.DeleteCategory)

	// --- Enrolment ---
	route("GET /api/v1/courses/{id}/participants", "course:viewparticipants", courseContext(deps, "id"), deps.CourseHandler.Participants)
	route("POST /api/v1/courses/{id}/enrolments", "enrol:manage", courseContext(deps, "id"), deps.CourseHandler.EnrolUser)
	route("PATCH /api/v1/courses/{id}/enrolments/{userId}", "enrol:manage", courseContext(deps, "id"), deps.CourseHandler.UpdateEnrolment)
	route("DELETE /api/v1/courses/{id}/enrolments/{userId}", "enrol:manage", courseContext(deps, "id"), deps.CourseHandler.Unenrol)
	route("GET /api/v1/courses/{id}/self-enrolment", "enrol:manage", courseContext(deps, "id"), deps.CourseHandler.GetSelfEnrolment)
	route("PUT /api/v1/courses/{id}/self-enrolment", "enrol:manage", courseContext(deps, "id"), deps.CourseHandler.SetSelfEnrolment)
	route("POST /api/v1/courses/{id}/enroll", "course:enrol", courseContext(deps, "id"), deps.CourseHandler.Enroll)

	// --- Roles ---
	// Role definitions are site administration; assignments and overrides
	// are per context, so course and category routes share the handlers.
	route("GET /api/v1/roles", "", nil, deps.RBACHandler.ListRoles)
	route("GET /api/v1/admin/capabilities", "role:manage", system, deps.RBACHandler.ListCapabilities)
	route("GET /api/v1/admin/roles/{roleId}", "role:manage", system, deps.RBACHandler.GetRole)
	route("POST /api/v1/admin/roles", "role:manage", system, deps.RBACHandler.CreateRole)
	route("DELETE /api/v1/admin/roles/{roleId}", "role:manage", system, deps.RBACHandler.DeleteRole)
	route("PUT /api/v1/admin/roles/{roleId}/capabilities/{capability}", "role:manage", system, deps.RBACHandler.SetRoleCapability)
	route("GET /api/v1/admin/role-assignments", "role:manage", system, deps.RBACHandler.ListAssignments(system))
	route("POST /api/v1/admin/role-assignments", "role:manage", system, deps.RBACHandler.Assign(system))
	route("DELETE /api/v1/admin/role-assignments/{userId}/{role}", "role:manage", system, deps.RBACHandler.Unassign(system))

	for _, scope := range []struct {
		prefix  string
		context rbac.ContextResolver
	}{
		{"/api/v1/courses/{id}", courseContext(deps, "id")},
		{"/api/v1/categories/{categoryId}", categoryContext(deps)},
	} {
		route("GET "+scope.prefix+"/role-assignments", "role:assign", scope.context, deps.RBACHandler.ListAssignments(scope.context))
		route("POST "+scope.prefix+"/role-assignments", "role:assign", scope.context, deps.RBACHandler.Assign(scope.context))
		route("DELETE "+scope.prefix+"/role-assignments/{userId}/{role}", "role:assign", scope.context, deps.RBACHandler.Unassign(scope.context))
		route("GET "+scope.prefix+"/role-overrides", "role:override", scope.context, deps.RBACHandler.ListOverrides(scope.context))
		route("PUT "+scope.prefix+"/role-overrides", "role:override", scope.context, deps.RBACHandler.SetOverride(scope.context))
	}

	// --- Course content ---
	// Reads need course access; writes need mod:manage at the section's
	// course or, for module routes, at the module's own context (which
	// inherits from the course but can carry its own overrides).
	member("GET /api/v1/courses/{id}/content", "", deps.ContentHandler.CourseContent)

	for _, kind := range []struct {
		path    string
		handler http.HandlerFunc
	}{
		{"label", deps.ContentHandler.CreateLabel},
		{"page", deps.ContentHandler.CreatePage},
		{"url", deps.ContentHandler.CreateURL},
		{"resource", deps.ContentHandler.CreateResource},
		{"folder", deps.ContentHandler.CreateFolder},
		{"book", deps.ContentHandler.CreateBook},
	} {
		route("POST /api/v1/sections/{sectionId}/modules/"+kind.path, "mod:manage", sectionContext(deps), kind.handler)
	}

	module := moduleContext(deps)
	route("PATCH /api/v1/modules/{moduleId}", "mod:manage", module, deps.ContentHandler.UpdateModule)
	route("PUT /api/v1/modules/{moduleId}/content", "mod:manage", module, deps.ContentHandler.UpdateModuleContent)
	route("DELETE /api/v1/modules/{moduleId}", "mod:manage", module, deps.ContentHandler.DeleteModule)
	route("POST /api/v1/modules/{moduleId}/folder-files", "mod:manage", module, deps.ContentHandler.AddFolderFile)
	route("PATCH /api/v1/modules/{moduleId}/folder-files/{fileId}", "mod:manage", module, deps.ContentHandler.UpdateFolderFile)
	route("DELETE /api/v1/modules/{moduleId}/folder-files/{fileId}", "mod:manage", module, deps.ContentHandler.DeleteFolderFile)
	route("POST /api/v1/modules/{moduleId}/book-chapters", "mod:manage", module, deps.ContentHandler.AddBookChapter)
	route("PATCH /api/v1/modules/{moduleId}/book-chapters/{chapterId}", "mod:manage", module, deps.ContentHandler.UpdateBookChapter)
	route("DELETE /api/v1/modules/{moduleId}/book-chapters/{chapterId}", "mod:manage", module, deps.ContentHandler.DeleteBookChapter)

	// --- Gradebook ---
	// Structure needs grade:manage, entering grades grade:edit, seeing the
	// whole class grade:viewall. A student's own report needs grade:view
	// and is scoped to themselves by the handler.
	member("GET /api/v1/courses/{id}/gradebook/categories", "grade:view", deps.GradebookHandler.ListCategories)
	route("POST /api/v1/courses/{id}/gradebook/categories", "grade:manage", courseContext(deps, "id"), deps.GradebookHandler.CreateCategory)
	route("PATCH /api/v1/gradebook/categories/{categoryId}", "grade:manage", gradeCategoryContext(deps), deps.GradebookHandler.UpdateCategory)
	route("DELETE /api/v1/gradebook/categories/{categoryId}", "grade:manage", gradeCategoryContext(deps), deps.GradebookHandler.DeleteCategory)
	member("GET /api/v1/courses/{id}/gradebook/items", "grade:view", deps.GradebookHandler.ListItems)
	route("POST /api/v1/courses/{id}/gradebook/items", "grade:manage", courseContext(deps, "id"), deps.GradebookHandler.CreateItem)
	route("PATCH /api/v1/gradebook/items/{itemId}", "grade:manage", itemContext(deps), deps.GradebookHandler.UpdateItem)
	route("DELETE /api/v1/gradebook/items/{itemId}", "grade:manage", itemContext(deps), deps.GradebookHandler.DeleteItem)
	route("POST /api/v1/gradebook/items/{itemId}/grades/{userId}", "grade:edit", itemContext(deps), deps.GradebookHandler.SetGrade)
	route("GET /api/v1/gradebook/items/{itemId}/grades", "grade:viewall", itemContext(deps), deps.GradebookHandler.ListGrades)
	route("GET /api/v1/gradebook/items/{itemId}/grades/{userId}/history", "grade:viewall", itemContext(deps), deps.GradebookHandler.History)
	member("GET /api/v1/courses/{id}/gradebook/mine", "grade:view", deps.GradebookHandler.MyReport)
	member("GET /api/v1/courses/{id}/gradebook/report", "grade:viewall", deps.GradebookHandler.GraderReport)
	member("GET /api/v1/courses/{id}/gradebook/users/{userId}", "grade:viewall", deps.GradebookHandler.UserReport)

	// --- Groups ---
	member("GET /api/v1/courses/{id}/groups", "", deps.GroupsHandler.List)
	route("POST /api/v1/courses/{id}/groups", "group:manage", courseContext(deps, "id"), deps.GroupsHandler.Create)
	route("POST /api/v1/courses/{id}/groups/auto", "group:manage", courseContext(deps, "id"), deps.GroupsHandler.AutoCreate)
	route("PATCH /api/v1/groups/{groupId}", "group:manage", groupContext(deps), deps.GroupsHandler.Update)
	route("DELETE /api/v1/groups/{groupId}", "group:manage", groupContext(deps), deps.GroupsHandler.Delete)
	route("POST /api/v1/groups/{groupId}/members", "group:manage", groupContext(deps), deps.GroupsHandler.AddMember)
	route("DELETE /api/v1/groups/{groupId}/members/{userId}", "group:manage", groupContext(deps), deps.GroupsHandler.RemoveMember)
	member("GET /api/v1/courses/{id}/groupings", "", deps.GroupsHandler.ListGroupings)
	route("POST /api/v1/courses/{id}/groupings", "group:manage", courseContext(deps, "id"), deps.GroupsHandler.CreateGrouping)
	route("PATCH /api/v1/groupings/{groupingId}", "group:manage", groupingContext(deps), deps.GroupsHandler.UpdateGrouping)
	route("DELETE /api/v1/groupings/{groupingId}", "group:manage", groupingContext(deps), deps.GroupsHandler.DeleteGrouping)
	route("POST /api/v1/groupings/{groupingId}/groups", "group:manage", groupingContext(deps), deps.GroupsHandler.AddGroupToGrouping)
	route("DELETE /api/v1/groupings/{groupingId}/groups/{groupId}", "group:manage", groupingContext(deps), deps.GroupsHandler.RemoveGroupFromGrouping)

	// --- Calendar ---
	// Course events need calendar:manageentries, personal events only auth
	// (self-service), site events calendar:manage. Editing an existing
	// event checks who owns it, in the handler.
	route("POST /api/v1/courses/{id}/events", "calendar:manageentries", courseContext(deps, "id"), deps.CalendarHandler.CreateCourseEvent)
	member("GET /api/v1/courses/{id}/events", "", deps.CalendarHandler.ListCourseEvents)
	route("POST /api/v1/users/me/events", "", nil, deps.CalendarHandler.CreateMyEvent)
	route("GET /api/v1/users/me/events", "", nil, deps.CalendarHandler.ListMyEvents)
	route("GET /api/v1/users/me/calendar.ics", "", nil, deps.CalendarHandler.ExportICS)
	route("PATCH /api/v1/events/{eventId}", "", nil, deps.CalendarHandler.UpdateEvent)
	route("DELETE /api/v1/events/{eventId}", "", nil, deps.CalendarHandler.DeleteEvent)
	route("POST /api/v1/admin/events", "calendar:manage", system, deps.CalendarHandler.CreateSiteEvent)

	return withCORS(mux)
}

// The resolvers below turn a route's path values into the RBAC context the
// capability is checked at. Routes keyed by a child id (section, group,
// grade item, ...) look up the owning course first.

func courseContext(deps Deps, pathParam string) rbac.ContextResolver {
	return func(r *http.Request) (string, error) {
		return deps.RBACService.ContextID(r.Context(), rbac.ContextLevelCourse, r.PathValue(pathParam))
	}
}

func categoryContext(deps Deps) rbac.ContextResolver {
	return func(r *http.Request) (string, error) {
		return deps.RBACService.ContextID(r.Context(), rbac.ContextLevelCategory, r.PathValue("categoryId"))
	}
}

func moduleContext(deps Deps) rbac.ContextResolver {
	return func(r *http.Request) (string, error) {
		return deps.RBACService.ContextID(r.Context(), rbac.ContextLevelModule, r.PathValue("moduleId"))
	}
}

func viaCourse(deps Deps, courseIDFor func(r *http.Request) (string, error)) rbac.ContextResolver {
	return func(r *http.Request) (string, error) {
		courseID, err := courseIDFor(r)
		if err != nil {
			return "", err
		}
		return deps.RBACService.ContextID(r.Context(), rbac.ContextLevelCourse, courseID)
	}
}

func sectionContext(deps Deps) rbac.ContextResolver {
	return viaCourse(deps, func(r *http.Request) (string, error) {
		return deps.CourseHandler.CourseIDForSection(r, r.PathValue("sectionId"))
	})
}

func itemContext(deps Deps) rbac.ContextResolver {
	return viaCourse(deps, func(r *http.Request) (string, error) {
		return deps.GradebookHandler.CourseIDForItem(r, r.PathValue("itemId"))
	})
}

func gradeCategoryContext(deps Deps) rbac.ContextResolver {
	return viaCourse(deps, func(r *http.Request) (string, error) {
		return deps.GradebookHandler.CourseIDForCategory(r, r.PathValue("categoryId"))
	})
}

func groupContext(deps Deps) rbac.ContextResolver {
	return viaCourse(deps, func(r *http.Request) (string, error) {
		return deps.GroupsHandler.CourseIDForGroup(r, r.PathValue("groupId"))
	})
}

func groupingContext(deps Deps) rbac.ContextResolver {
	return viaCourse(deps, func(r *http.Request) (string, error) {
		return deps.GroupsHandler.CourseIDForGrouping(r, r.PathValue("groupingId"))
	})
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
