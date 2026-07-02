// Package app assembles and runs the reports service.
package app

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/pizdagladki/full/internal/platform/logger"
	"github.com/pizdagladki/full/services/reports/internal/api/delivery"
	appmiddleware "github.com/pizdagladki/full/services/reports/internal/api/middleware"
	"github.com/pizdagladki/full/services/reports/internal/api/repository"
	"github.com/pizdagladki/full/services/reports/internal/api/service"
	"github.com/pizdagladki/full/services/reports/internal/config"
)

const (
	defaultCooldownTTL       = 1800 * time.Second
	defaultBugReportPrefix   = "bug-reports/"
	defaultMaxUploadBytes    = 500 * 1024 * 1024
	defaultSessionCookieName = "session"
)

// App holds the service dependencies and drives its lifecycle.
type App struct {
	name string

	logger    *zap.Logger
	validator echo.Validator
	cfg       *config.Config

	pgxPool     *pgxpool.Pool
	redisClient *redis.Client
	minioClient *minio.Client

	cheatRepo     repository.CheatReportsRepository
	cooldownStore repository.CooldownStore
	sessionRepo   repository.SessionRepository
	bugRepo       repository.BugReportsRepository
	bugStorage    repository.BugRecordingStorage

	reportsService service.ReportsService
	sessionSvc     service.SessionService
	bugSvc         service.BugReportService
	telegramNotify service.TelegramNotifier

	reportsHandler delivery.ReportsHandler
	authMiddleware *appmiddleware.AuthMiddleware
}

// New returns an empty App for the given service name.
func New(name string) *App {
	return &App{name: name}
}

// Run initializes dependencies in order and runs the workers until ctx is
// canceled (graceful shutdown). A failed Postgres or Redis ping aborts startup.
func (a *App) Run(ctx context.Context) error {
	err := a.initLogger()
	if err != nil {
		return err
	}
	defer func() { _ = a.logger.Sync() }()

	a.logger.Info("starting service", zap.String("service", a.name))

	a.initValidator()

	err = a.populateConfig()
	if err != nil {
		return err
	}

	// Fail fast if Telegram is not configured (AC4).
	if a.cfg.Telegram.BotToken == "" || a.cfg.Telegram.ChatID == "" {
		a.logger.Error("telegram config missing: bot_token and chat_id are required")

		return errMissingTelegramConfig
	}

	err = a.initPostgres(ctx)
	if err != nil {
		return err
	}
	defer a.pgxPool.Close()

	err = a.initRedis(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = a.redisClient.Close() }()

	err = a.initStorage(ctx)
	if err != nil {
		return err
	}

	a.initRepositories()
	a.initServices()
	a.initHandlers()

	return a.runWorkers(ctx)
}

// errMissingTelegramConfig is returned when Telegram credentials are absent.
var errMissingTelegramConfig = &missingTelegramConfigError{}

type missingTelegramConfigError struct{}

func (e *missingTelegramConfigError) Error() string {
	return "telegram config missing: bot_token and chat_id are required"
}

func (a *App) initLogger() error {
	l, err := logger.New()
	if err != nil {
		return err
	}

	a.logger = l

	return nil
}

func (a *App) populateConfig() error {
	cfg, err := config.Load("cmd/config.yaml")
	if err != nil {
		return err
	}

	a.cfg = cfg

	return nil
}

func (a *App) initRepositories() {
	a.cheatRepo = repository.NewCheatReportsRepository(a.pgxPool)
	a.cooldownStore = repository.NewCooldownStore(a.redisClient)
	a.sessionRepo = repository.NewSessionRepository(a.redisClient)
	a.bugRepo = repository.NewBugReportsRepository(a.pgxPool)

	prefix := a.cfg.BugReport.ReportsKeyPrefix
	if prefix == "" {
		prefix = defaultBugReportPrefix
	}

	a.bugStorage = repository.NewBugRecordingStorage(a.minioClient, a.cfg.Storage.Bucket, prefix)
}

func (a *App) initServices() {
	ttl := defaultCooldownTTL
	if a.cfg.Reports.CooldownTTLSeconds > 0 {
		ttl = time.Duration(a.cfg.Reports.CooldownTTLSeconds) * time.Second
	}

	a.reportsService = service.NewReportsService(a.cheatRepo, a.cooldownStore, a.logger, ttl)
	a.sessionSvc = service.NewSessionService(a.sessionRepo)
	a.telegramNotify = service.NewTelegramNotifier(a.cfg.Telegram.BotToken, a.cfg.Telegram.ChatID, a.logger)

	prefix := a.cfg.BugReport.ReportsKeyPrefix
	if prefix == "" {
		prefix = defaultBugReportPrefix
	}

	a.bugSvc = service.NewBugReportService(a.bugRepo, a.bugStorage, a.telegramNotify, a.logger, prefix)
}

func (a *App) initHandlers() {
	cookieName := a.cfg.Session.CookieName
	if cookieName == "" {
		cookieName = defaultSessionCookieName
	}

	a.authMiddleware = appmiddleware.NewAuthMiddleware(a.sessionSvc, cookieName, a.logger)

	maxBytes := a.cfg.BugReport.MaxUploadBytes
	if maxBytes == 0 {
		maxBytes = defaultMaxUploadBytes
	}

	a.reportsHandler = delivery.NewReportsHandler(a.reportsService, a.bugSvc, maxBytes, a.logger)
}
