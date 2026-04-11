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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/closer"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/config"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
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
	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))

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
	userRepo := repository.NewUserRepository(pool)
	brandRepo := repository.NewBrandRepository(pool)
	auditRepo := repository.NewAuditRepository(pool)
	tokenSvc := service.NewTokenService(cfg.JWTSecret, cfg.JWTExpiry, cfg.RefreshExpiry, cfg.ResetExpiry)

	// Reset token store is only needed for test endpoints.
	var resetTokenStore *service.InMemoryResetTokenStore
	if cfg.EnableTestEndpoints {
		resetTokenStore = service.NewInMemoryResetTokenStore()
	}

	authSvc := service.NewAuthService(userRepo, tokenSvc, resetTokenStore, cfg.BcryptCost)
	brandSvc := service.NewBrandService(brandRepo, userRepo)
	auditSvc := service.NewAuditService(auditRepo)

	// Seed admin
	if err := authSvc.SeedAdmin(ctx, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}

	// Handlers
	authHandler := handler.NewAuthHandler(authSvc, auditSvc, cfg.CookieSecure)
	brandHandler := handler.NewBrandHandler(brandSvc, auditSvc)
	auditHandler := handler.NewAuditHandler(auditSvc)

	// Router
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.Recovery)
	r.Use(middleware.BodyLimit(1 << 20)) // 1 MB
	r.Use(middleware.SecureHeaders)
	r.Use(middleware.CORS(cfg.CORSOrigins))
	r.Use(middleware.Logging)

	// Public routes
	r.Get("/healthz", handler.HandleHealthz())
	r.Post("/auth/login", authHandler.Login)
	r.Post("/auth/refresh", authHandler.Refresh)
	r.Post("/auth/password-reset-request", authHandler.RequestPasswordReset)
	r.Post("/auth/password-reset", authHandler.ResetPassword)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(tokenSvc))

		r.Post("/auth/logout", authHandler.Logout)
		r.Get("/auth/me", authHandler.GetMe)

		// Brand management
		r.Route("/brands", func(r chi.Router) {
			r.Post("/", brandHandler.CreateBrand)
			r.Get("/", brandHandler.ListBrands)
			r.Get("/{brandID}", brandHandler.GetBrand)
			r.Put("/{brandID}", brandHandler.UpdateBrand)
			r.Delete("/{brandID}", brandHandler.DeleteBrand)
			r.Post("/{brandID}/managers", brandHandler.AssignManager)
			r.Delete("/{brandID}/managers/{userID}", brandHandler.RemoveManager)
		})

		// Audit logs
		r.Get("/audit-logs", auditHandler.ListAuditLogs)
	})

	// Test endpoints (only when ENABLE_TEST_ENDPOINTS=true)
	if cfg.EnableTestEndpoints {
		testHandler := handler.NewTestHandler(authSvc, brandSvc, resetTokenStore)
		r.Route("/test", func(r chi.Router) {
			r.Post("/seed-user", testHandler.SeedUser)
			r.Post("/seed-brand", testHandler.SeedBrand)
			r.Get("/reset-tokens", testHandler.GetResetToken)
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

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return cl.Close(shutdownCtx)
}
