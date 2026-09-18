// Command server boots the Polaris API: it loads config, connects to
// Postgres, applies pending migrations, wires up the domain services and
// HTTP handlers, and serves.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"polaris-api/internal/auth"
	"polaris-api/internal/config"
	"polaris-api/internal/course"
	"polaris-api/internal/database"
	"polaris-api/internal/eventbus"
	"polaris-api/internal/httpserver"
	"polaris-api/internal/rbac"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx := context.Background()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool); err != nil {
		return err
	}
	log.Println("database ready")

	authService := auth.NewService(pool, cfg.JWTSecret, cfg.JWTTTLHours)
	authHandler := auth.NewHandler(authService)

	rbacService := rbac.NewService(pool)
	events := eventbus.New(pool)

	courseService := course.NewService(pool, rbacService, events)
	courseHandler := course.NewHandler(courseService)

	router := httpserver.NewRouter(httpserver.Deps{
		AuthHandler:   authHandler,
		CourseHandler: courseHandler,
		RBACService:   rbacService,
	})

	return serve(cfg.Port, router)
}

func serve(port string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("polaris-api listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-stop:
		log.Println("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}
