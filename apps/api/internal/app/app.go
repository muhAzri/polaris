// Package app wires the domain services and HTTP handlers into one router.
// The server binary and the integration tests both build the API through
// New, so they always exercise the same wiring.
package app

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"polaris-api/internal/auth"
	"polaris-api/internal/calendar"
	"polaris-api/internal/content"
	"polaris-api/internal/course"
	"polaris-api/internal/coursemodule"
	"polaris-api/internal/eventbus"
	"polaris-api/internal/gradebook"
	"polaris-api/internal/groups"
	"polaris-api/internal/httpserver"
	"polaris-api/internal/rbac"
	"polaris-api/internal/storage"
)

type Options struct {
	JWTSecret   string
	JWTTTLHours int
}

// New builds the complete API handler on top of an already-migrated pool.
func New(pool *pgxpool.Pool, opts Options, objectStorage storage.Storage) http.Handler {
	authService := auth.NewService(pool, opts.JWTSecret, opts.JWTTTLHours)
	rbacService := rbac.NewService(pool)
	events := eventbus.New(pool)
	courseModules := coursemodule.NewService(pool)

	return httpserver.NewRouter(httpserver.Deps{
		AuthHandler:      auth.NewHandler(authService),
		CourseHandler:    course.NewHandler(course.NewService(pool, rbacService, events, courseModules)),
		ContentHandler:   content.NewHandler(content.NewService(pool, courseModules, events, objectStorage, rbacService)),
		GradebookHandler: gradebook.NewHandler(gradebook.NewService(pool, events), rbacService),
		GroupsHandler:    groups.NewHandler(groups.NewService(pool, events)),
		CalendarHandler:  calendar.NewHandler(calendar.NewService(pool, rbacService)),
		RBACHandler:      rbac.NewHandler(rbacService),
		RBACService:      rbacService,
	})
}
