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
	"polaris-api/internal/content"
	"polaris-api/internal/course"
	"polaris-api/internal/coursemodule"
	"polaris-api/internal/database"
	"polaris-api/internal/eventbus"
	"polaris-api/internal/httpserver"
	"polaris-api/internal/rbac"
	"polaris-api/internal/storage"
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

	objectStorage, err := storage.NewS3Storage(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3UseSSL)
	if err != nil {
		return err
	}

	courseModules := coursemodule.NewService(pool)
	contentService := content.NewService(pool, courseModules, events, objectStorage)
	contentHandler := content.NewHandler(contentService)

	router := httpserver.NewRouter(httpserver.Deps{
		AuthHandler:    authHandler,
		CourseHandler:  courseHandler,
		ContentHandler: contentHandler,
		RBACService:    rbacService,
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
