package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/authz"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/closer"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/config"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/testapi"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Logger
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})))

	// Closer (LIFO: last added = first closed)
	cl := closer.New()

	// Database
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	cl.Add("postgres", func(_ context.Context) error {
		pool.Close()
		return nil
	})

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	slog.Info("database connected")

	// Cron scheduler
	scheduler := cron.New(cron.WithSeconds())
	scheduler.Start()
	cl.Add("cron", func(_ context.Context) error {
		ctx := scheduler.Stop()
		<-ctx.Done()
		return nil
	})
	slog.Info("cron scheduler started")

	// Dependencies
	repoFactory := repository.NewRepoFactory()
	tokenSvc := service.NewTokenService(cfg.JWTSecret, cfg.JWTExpiry, cfg.RefreshExpiry, cfg.ResetExpiry)

	// Reset token store is only needed for test endpoints.
	var resetTokenStore *service.InMemoryResetTokenStore
	if cfg.EnableTestEndpoints {
		resetTokenStore = service.NewInMemoryResetTokenStore()
	}

	authSvc := service.NewAuthService(pool, repoFactory, tokenSvc, resetTokenStore, cfg.BcryptCost)
	brandSvc := service.NewBrandService(pool, repoFactory, cfg.BcryptCost)
	auditSvc := service.NewAuditService(pool, repoFactory)
	authzSvc := authz.NewAuthzService(brandSvc)

	// Seed admin
	if err := authSvc.SeedAdmin(ctx, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}

	// Router
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.Recovery)
	r.Use(middleware.BodyLimit(int64(cfg.BodyLimitBytes)))
	r.Use(middleware.RealIP)
	r.Use(middleware.ClientIP)
	r.Use(middleware.SecureHeaders)
	r.Use(middleware.CORS(cfg.CORSOrigins))
	r.Use(middleware.Logging)

	// Create server implementing ServerInterface
	server := handler.NewServer(authSvc, brandSvc, authzSvc, auditSvc, cfg.CookieSecure)

	// Register API routes via generated handler
	api.HandlerWithOptions(server, api.ChiServerOptions{
		BaseRouter: r,
		Middlewares: []api.MiddlewareFunc{
			middleware.AuthFromScopes(tokenSvc),
		},
		ErrorHandlerFunc: handler.HandleParamError,
	})

	// Test endpoints (only when ENVIRONMENT != production).
	// Seed-brand and friends impersonate the seed admin so audit rows get a
	// valid actor FK — resolve the admin here, next to the wiring.
	if cfg.EnableTestEndpoints {
		admin, err := authSvc.GetUserByEmail(ctx, cfg.AdminEmail)
		if err != nil {
			return fmt.Errorf("lookup seed admin for test endpoints: %w", err)
		}

		testHandler := handler.NewTestHandler(authSvc, brandSvc, resetTokenStore, admin.ID)
		testapi.HandlerWithOptions(testHandler, testapi.ChiServerOptions{
			BaseRouter:       r,
			ErrorHandlerFunc: handler.HandleParamError,
		})
		slog.Warn("TEST ENDPOINTS ENABLED — do not use in production")
	}

	// Server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	// Register HTTP server in closer (will be shut down first due to LIFO)
	cl.Add("http-server", srv.Shutdown)

	// Start server
	errCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "port", cfg.Port)
		errCh <- srv.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("shutting down", "signal", sig.String())
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server error: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, cfg.ShutdownTimeout)
	defer cancel()

	return cl.Close(shutdownCtx)
}
