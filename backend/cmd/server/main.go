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
	"github.com/songhanxu/wiseinvest/internal/adapter/repository"
	"github.com/songhanxu/wiseinvest/internal/application/service"
	"github.com/songhanxu/wiseinvest/internal/domain/agent"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/cache"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/config"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/database"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
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

	// Initialize Agent Factory
	agentFactory := agent.NewAgentFactory(llmClient, log)

	// Initialize services
	conversationService := service.NewConversationService(
		conversationRepo,
		messageRepo,
		agentFactory,
		redisClient,
		log,
	)

	// Initialize HTTP server
	router := api.NewRouter(conversationService, log)
	
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
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
