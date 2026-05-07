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
	"github.com/alikhanmurzayev/ugcboost/backend/internal/telegram"
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

	tgRig, err := setupTelegram(cfg, appLogger)
	if err != nil {
		return err
	}
	registerNotifyWaiter(tgRig.Notifier, cl)

	creatorApplicationSvc := service.NewCreatorApplicationService(pool, repoFactory, tgRig.Notifier, appLogger)
	creatorApplicationTelegramSvc := service.NewCreatorApplicationTelegramService(pool, repoFactory, tgRig.Notifier, appLogger)
	creatorSvc := service.NewCreatorService(pool, repoFactory, appLogger)
	campaignSvc := service.NewCampaignService(pool, repoFactory, appLogger)
	campaignCreatorSvc := service.NewCampaignCreatorService(pool, repoFactory, appLogger)
	dictionarySvc := service.NewDictionaryService(pool, repoFactory, appLogger)

	tgHandler := telegram.NewHandler(creatorApplicationTelegramSvc, appLogger)
	startTelegramRunner(ctx, cfg, tgHandler, appLogger, cl)

	// Seed admin
	if err := authSvc.SeedAdmin(ctx, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}

	// Router
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.Recovery(appLogger))
	r.Use(middleware.BodyLimit(int64(cfg.BodyLimitBytes)))
	r.Use(middleware.RealIP)
	r.Use(middleware.ClientIP)
	r.Use(middleware.RequestMeta)
	r.Use(middleware.RefreshCookie)
	r.Use(middleware.SecureHeaders)
	r.Use(middleware.CORS(cfg.CORSOrigins))
	r.Use(middleware.Logging(appLogger))
	// SendPulse webhook bearer auth — gates only the dedicated path; every
	// other request flows through unchanged.
	r.Use(middleware.SendPulseAuth(cfg.SendPulseWebhookSecret, appLogger))

	// Create server implementing ServerInterface
	server := handler.NewServer(authSvc, brandSvc, authzSvc, auditSvc, creatorApplicationSvc, creatorSvc, campaignSvc, campaignCreatorSvc, dictionarySvc, handler.ServerConfig{
		Version:               cfg.Version,
		CookieSecure:          cfg.CookieSecure,
		TelegramBotUsername:   cfg.TelegramBotUsername,
		LegalAgreementVersion: cfg.LegalAgreementVersion,
		LegalPrivacyVersion:   cfg.LegalPrivacyVersion,
	}, appLogger)

	api.HandlerWithOptions(handler.NewStrictAPIHandler(server), api.ChiServerOptions{
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
		testHandler := handler.NewTestAPIHandler(authSvc, pool, repoFactory, resetTokenStore, tgHandler, tgRig.Spy, appLogger)
		testapi.HandlerWithOptions(handler.NewStrictTestAPIHandler(testHandler), testapi.ChiServerOptions{
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
