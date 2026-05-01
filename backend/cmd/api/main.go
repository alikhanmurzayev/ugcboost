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
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
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
	slogLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(slogLogger)
	appLogger := logger.New(slogLogger)

	// Closer (LIFO: last added = first closed)
	cl := closer.New(appLogger)

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
	appLogger.Info(ctx, "database connected")

	// Cron scheduler
	scheduler := cron.New(cron.WithSeconds())
	scheduler.Start()
	cl.Add("cron", func(_ context.Context) error {
		ctx := scheduler.Stop()
		<-ctx.Done()
		return nil
	})
	appLogger.Info(ctx, "cron scheduler started")

	// Dependencies
	repoFactory := repository.NewRepoFactory()
	tokenSvc := service.NewTokenService(cfg.JWTSecret, cfg.JWTExpiry, cfg.RefreshExpiry, cfg.ResetExpiry)

	// Reset token store is only needed for test endpoints.
	var resetTokenStore *service.InMemoryResetTokenStore
	if cfg.EnableTestEndpoints {
		resetTokenStore = service.NewInMemoryResetTokenStore()
	}

	authSvc := service.NewAuthService(pool, repoFactory, tokenSvc, resetTokenStore, cfg.BcryptCost, appLogger)
	brandSvc := service.NewBrandService(pool, repoFactory, cfg.BcryptCost, appLogger)
	auditSvc := service.NewAuditService(pool, repoFactory)
	authzSvc := authz.NewAuthzService(brandSvc)
	creatorApplicationSvc := service.NewCreatorApplicationService(pool, repoFactory, appLogger)
	dictionarySvc := service.NewDictionaryService(pool, repoFactory, appLogger)

	// Seed admin
	if err := authSvc.SeedAdmin(ctx, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}

	// Seed dev brand-manager + matching brand. Skipped in production via the
	// EnableTestEndpoints guard, and skipped anywhere if BRAND_DEV_PASSWORD
	// is empty. Idempotent — safe to run on every boot.
	if cfg.EnableTestEndpoints && cfg.BrandDevPassword != "" {
		if err := seedDevBrand(ctx, authSvc, brandSvc, cfg, appLogger); err != nil {
			return fmt.Errorf("seed dev brand: %w", err)
		}
	}

	// Router
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.Recovery(appLogger))
	r.Use(middleware.BodyLimit(int64(cfg.BodyLimitBytes)))
	r.Use(middleware.RealIP)
	r.Use(middleware.ClientIP)
	r.Use(middleware.SecureHeaders)
	r.Use(middleware.CORS(cfg.CORSOrigins))
	r.Use(middleware.Logging(appLogger))

	// Create server implementing ServerInterface
	server := handler.NewServer(authSvc, brandSvc, authzSvc, auditSvc, creatorApplicationSvc, dictionarySvc, handler.ServerConfig{
		Version:               cfg.Version,
		CookieSecure:          cfg.CookieSecure,
		TelegramBotUsername:   cfg.TelegramBotUsername,
		LegalAgreementVersion: cfg.LegalAgreementVersion,
		LegalPrivacyVersion:   cfg.LegalPrivacyVersion,
	}, appLogger)

	// Register API routes via generated handler
	api.HandlerWithOptions(server, api.ChiServerOptions{
		BaseRouter: r,
		Middlewares: []api.MiddlewareFunc{
			middleware.AuthFromScopes(tokenSvc),
		},
		ErrorHandlerFunc: handler.HandleParamError(appLogger),
	})

	// Test endpoints (only when ENVIRONMENT != production). The cleanup
	// endpoint uses the repo factory directly — the hard-delete for users
	// is test-only and intentionally not exposed through any service.
	if cfg.EnableTestEndpoints {
		testHandler := handler.NewTestAPIHandler(authSvc, pool, repoFactory, resetTokenStore, appLogger)
		testapi.HandlerWithOptions(testHandler, testapi.ChiServerOptions{
			BaseRouter:       r,
			ErrorHandlerFunc: handler.HandleParamError(appLogger),
		})
		appLogger.Warn(ctx, "TEST ENDPOINTS ENABLED — do not use in production")
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
		appLogger.Info(ctx, "server starting", "port", cfg.Port)
		errCh <- srv.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		appLogger.Info(ctx, "shutting down", "signal", sig.String())
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server error: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, cfg.ShutdownTimeout)
	defer cancel()

	return cl.Close(shutdownCtx)
}

// seedDevBrand provisions a brand-manager account + a matching brand so the
// /prototype brand-cabinet has something to show. Idempotent: skips when the
// manager already manages at least one brand.
func seedDevBrand(
	ctx context.Context,
	authSvc *service.AuthService,
	brandSvc *service.BrandService,
	cfg *config.Config,
	log logger.Logger,
) error {
	user, err := authSvc.SeedBrandManager(ctx, cfg.BrandDevEmail, cfg.BrandDevPassword)
	if err != nil {
		return fmt.Errorf("seed brand-manager: %w", err)
	}
	if user == nil {
		return nil
	}

	owned, err := brandSvc.ListBrands(ctx, &user.ID)
	if err != nil {
		return fmt.Errorf("list manager brands: %w", err)
	}
	if len(owned) > 0 {
		log.Info(ctx, "dev brand already linked to manager", "email", user.Email, "brand", owned[0].Name)
		return nil
	}

	brand, err := brandSvc.CreateBrand(ctx, cfg.BrandDevBrandName, nil)
	if err != nil {
		return fmt.Errorf("create dev brand: %w", err)
	}
	if _, _, err := brandSvc.AssignManager(ctx, brand.ID, user.Email); err != nil {
		return fmt.Errorf("assign dev brand manager: %w", err)
	}

	log.Info(ctx, "dev brand seeded", "email", user.Email, "brand", brand.Name)
	return nil
}
