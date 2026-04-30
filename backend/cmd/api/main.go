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
	creatorApplicationSvc := service.NewCreatorApplicationService(pool, repoFactory, appLogger)
	dictionarySvc := service.NewDictionaryService(pool, repoFactory, appLogger)

	// Telegram skeleton: handler stays in-process for both real polling and
	// the test endpoint. Real long polling only starts when a token is set
	// AND test endpoints are off — otherwise the bot is a passive
	// dependency that the test API drives via synthetic updates.
	tgHandler := telegram.NewHandler()
	if cfg.TelegramBotToken != "" && !cfg.EnableTestEndpoints {
		runnerCtx, runnerCancel := context.WithCancel(context.Background())
		defer runnerCancel()
		go telegram.Run(runnerCtx, cfg.TelegramBotToken, tgHandler, appLogger)
		cl.Add("telegram-runner", func(_ context.Context) error {
			runnerCancel()
			return nil
		})
		appLogger.Info(ctx, "telegram bot polling enabled")
	} else {
		appLogger.Info(ctx, "telegram bot polling disabled",
			"has_token", cfg.TelegramBotToken != "",
			"test_endpoints", cfg.EnableTestEndpoints)
	}

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
		testHandler := handler.NewTestAPIHandler(authSvc, pool, repoFactory, resetTokenStore, tgHandler, appLogger)
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
