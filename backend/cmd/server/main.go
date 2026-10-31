package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"github.com/thienel/tlog"
	"go.uber.org/zap"

	"github.com/tiloneeee/ai-fraud-detection/backend/internal/infra/database"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/infra/kbs"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/infra/persistence"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/handler"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/middleware"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/interface/api/router"
	"github.com/tiloneeee/ai-fraud-detection/backend/internal/usecase/service/serviceimpl"
	"github.com/tiloneeee/ai-fraud-detection/backend/pkg/config"
)

func main() {
	// Probe before loading config, logging, or database clients.
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8000"
		}
		if err := checkHealth("http://127.0.0.1:" + port + "/health"); err != nil {
			log.Print(err)
			os.Exit(1)
		}
		return
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize tlog
	err = tlog.Init(tlog.Config{
		Environment:   cfg.Server.Env,
		Level:         cfg.Log.Level,
		AppName:       cfg.Server.ServiceName,
		EnableConsole: cfg.Log.EnableConsole,
		EnableFile:    true,
		FilePath:      cfg.Log.FilePath,
		MaxSizeMB:     cfg.Log.MaxSizeMB,
		MaxBackups:    cfg.Log.MaxBackups,
		MaxAgeDays:    cfg.Log.MaxAgeDays,
		Compress:      cfg.Log.Compress,
	})
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer tlog.Sync()

	tlog.Info("Starting server",
		zap.String("service", cfg.Server.ServiceName),
		zap.String("version", cfg.Server.Version),
		zap.String("env", cfg.Server.Env),
	)

	// Initialize database
	if err := database.Init(&cfg.Database); err != nil {
		tlog.Fatal("Failed to initialize database", zap.Error(err))
	}
	defer database.Close()

	// Schema is managed by SQL files in migrations/ (make migrate-up), not AutoMigrate.

	// Redis rate limiter (RATE_LIMIT_ENABLED=false skips Redis entirely)
	var limiter *middleware.RateLimiter
	if cfg.RateLimit.Enabled {
		opt, err := goredis.ParseURL(cfg.RedisURL)
		if err != nil {
			tlog.Fatal("Invalid REDIS_URL", zap.Error(err))
		}
		rdb := goredis.NewClient(opt)
		defer rdb.Close()
		pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err = rdb.Ping(pingCtx).Err()
		cancel()
		if err != nil {
			tlog.Fatal("Failed to connect Redis", zap.Error(err))
		}
		limiter = middleware.NewRateLimiter(rdb)
	}

	// Initialize repositories
	db := database.GetDB()
	userRepo := persistence.NewUserRepository(db)
	transactor := persistence.NewTransactor(db)
	profileRepo := persistence.NewProfileRepository(db)
	catalogRepo := persistence.NewCatalogRepository(db)
	addressRepo := persistence.NewAddressRepository(db)
	orderRepo := persistence.NewOrderRepository(db)

	// Initialize services
	jwtService := serviceimpl.NewJWTService(
		cfg.JWT.Secret,
		cfg.JWT.AccessExpiryMinutes,
		cfg.JWT.RefreshExpiryHours,
	)
	authService := serviceimpl.NewAuthService(userRepo, jwtService)
	userService := serviceimpl.NewUserService(userRepo, transactor)
	profileService := serviceimpl.NewProfileService(profileRepo)
	catalogService := serviceimpl.NewCatalogService(catalogRepo)
	riskClient, err := kbs.NewClient(cfg.KBSURL)
	if err != nil {
		tlog.Fatal("Invalid KBS_URL configuration")
		return
	}
	buyerService := serviceimpl.NewBuyerService(addressRepo, catalogRepo, orderRepo, transactor, riskClient)
	sellerService := serviceimpl.NewSellerService(catalogRepo, orderRepo, transactor, riskClient)

	// Initialize middleware
	origins := strings.Join(cfg.CORSAllowedOrigins, ",")
	mw := middleware.New(authService, profileService, sellerService, limiter, origins)

	// Initialize handlers
	authHandler := handler.NewAuthHandler(authService, userService)
	userHandler := handler.NewUserHandler(userService)
	profileHandler := handler.NewProfileHandler(profileService)
	catalogHandler := handler.NewCatalogHandler(catalogService)
	buyerHandler := handler.NewBuyerHandler(buyerService)
	sellerHandler := handler.NewSellerHandler(sellerService)

	// Set Gin mode
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	// Setup router
	engine := router.SetupRouter(
		authHandler, userHandler, profileHandler, catalogHandler, buyerHandler, sellerHandler,
		mw, cfg.RateLimit.RequestsPerMinute,
	)

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      engine,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		tlog.Info("Server starting",
			zap.String("port", cfg.Server.Port),
			zap.String("env", cfg.Server.Env),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			tlog.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	tlog.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		tlog.Fatal("Server forced to shutdown", zap.Error(err))
	}

	tlog.Info("Server exited gracefully")
}
