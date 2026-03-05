package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"github.com/songhanxu/wiseinvest/internal/adapter/api"
	"github.com/songhanxu/wiseinvest/internal/adapter/api/handler"
	"github.com/songhanxu/wiseinvest/internal/adapter/repository"
	"github.com/songhanxu/wiseinvest/internal/application/service"
	"github.com/songhanxu/wiseinvest/internal/domain/agent"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/auth"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/cache"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/database"
	infraapns "github.com/songhanxu/wiseinvest/internal/infrastructure/apns"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/scheduler"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/search"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/skill"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/wxwork"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		logrus.Warn("No .env file found, using environment variables")
	}

	// Initialize logger
	log := logger.NewLogger()
	log.Info("Starting WiseInvest Server...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize database
	db, err := database.NewPostgresDB(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	log.Info("Database connected successfully")

	// Auto migrate database schema
	if err := database.AutoMigrate(db); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
	log.Info("Database migration completed")

	// Seed default data
	if err := database.SeedDefaultData(db); err != nil {
		log.Fatalf("Failed to seed default data: %v", err)
	}
	log.Info("Default data seeded successfully")

	// Initialize Redis cache
	redisClient, err := cache.NewRedisClient(cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Info("Redis connected successfully")

	// Initialize LLM client
	llmClient := llm.NewOpenAIClient(cfg.OpenAI)
	log.Info("LLM client initialized")

	// Initialize repositories
	conversationRepo := repository.NewConversationRepository(db)
	messageRepo := repository.NewMessageRepository(db)
	deviceTokenRepo := repository.NewDeviceTokenRepository(db)

	// Initialize web searcher (enabled when SEARCH_API_KEY is set in .env)
	searcher := search.New(cfg.Search.Provider, cfg.Search.APIKey)
	if cfg.Search.APIKey != "" {
		log.Infof("Web search enabled (provider: %s)", cfg.Search.Provider)
	} else {
		log.Info("Web search disabled (set SEARCH_PROVIDER + SEARCH_API_KEY in .env to enable)")
	}

	// ── Skill Registries ──────────────────────────────────────────────────────
	// Each market agent gets its own registry with market-appropriate tools.

	// A-share: web search + real-time price + sector rankings + stock fundamentals
	aShareRegistry := skill.NewRegistry()
	aShareRegistry.Register(skill.NewWebSearchSkill(searcher, "A股"))
	aShareRegistry.Register(skill.NewASharePriceSkill())
	aShareRegistry.Register(skill.NewAShareSectorSkill())
	aShareRegistry.Register(skill.NewAShareStockDetailSkill())
	aShareRegistry.Register(skill.NewLookupAShareCodeSkill())
	log.Infof("A-share skill registry: %d skills registered", aShareRegistry.Count())

	// US-stock: web search + real-time US stock quote
	usStockRegistry := skill.NewRegistry()
	usStockRegistry.Register(skill.NewWebSearchSkill(searcher, ""))
	usStockRegistry.Register(skill.NewUSStockPriceSkill())
	log.Infof("US-stock skill registry: %d skills registered", usStockRegistry.Count())

	// Crypto: web search (crypto prefix) + real-time crypto price
	cryptoRegistry := skill.NewRegistry()
	cryptoRegistry.Register(skill.NewWebSearchSkill(searcher, "crypto"))
	cryptoRegistry.Register(skill.NewCryptoPriceSkill())
	log.Infof("Crypto skill registry: %d skills registered", cryptoRegistry.Count())

	// ── Agent Factory ──────────────────────────────────────────────────────────
	agentFactory := agent.NewAgentFactory(llmClient, searcher, log, aShareRegistry, usStockRegistry, cryptoRegistry)

	// Initialize services
	conversationService := service.NewConversationService(
		conversationRepo,
		messageRepo,
		agentFactory,
		redisClient,
		log,
	)

	// ── Auth Services ──────────────────────────────────────────────────────────
	jwtSvc := auth.NewJWTService(cfg.JWT.Secret, cfg.JWT.Expiration)
	wechatSvc := auth.NewWeChatService(cfg.WeChat.AppID, cfg.WeChat.AppSecret)
	smsSvc := auth.NewSMSService(redisClient)
	authHandler := handler.NewAuthHandler(db, jwtSvc, wechatSvc, smsSvc, log)
	log.Info("Auth services initialized")

	// ── Notification Services ───────────────────────────────────────────────
	wxClient := wxwork.NewClient(cfg.Notification.WxWorkWebhookURL)
	if wxClient.IsConfigured() {
		log.Info("WeChat Work webhook configured")
	} else {
		log.Warn("WXWORK_WEBHOOK_URL not set — WeChat Work push disabled")
	}

	apnsClient, err := infraapns.NewClient(infraapns.Config{
		KeyID:      cfg.Notification.APNSKeyID,
		TeamID:     cfg.Notification.APNSTeamID,
		BundleID:   cfg.Notification.APNSBundleID,
		KeyFile:    cfg.Notification.APNSKeyFile,
		Production: cfg.Notification.APNSProduction,
	})
	if err != nil {
		log.Warnf("APNs client init failed (push disabled): %v", err)
	} else if apnsClient != nil {
		log.Info("APNs client initialized")
	} else {
		log.Warn("APNS_KEY_ID not set — APNs push disabled")
	}

	deviceHandler := handler.NewDeviceHandler(deviceTokenRepo, log)

	// ── Scheduler ────────────────────────────────────────────────────────────
	dailyTask := scheduler.NewDailyReportTask(agentFactory, wxClient, apnsClient, deviceTokenRepo, log)
	sched := scheduler.NewScheduler(dailyTask, log)
	sched.Start()
	defer sched.Stop()

	// Initialize HTTP server
	router := api.NewRouter(conversationService, authHandler, deviceHandler, jwtSvc, log)
	
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 300 * time.Second, // 5 minutes for streaming responses
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Infof("Server starting on port %s", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Info("Server exited")
}
