package api

import (
	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/adapter/api/handler"
	"github.com/songhanxu/wiseinvest/internal/adapter/api/middleware"
	"github.com/songhanxu/wiseinvest/internal/application/service"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// NewRouter creates a new HTTP router
func NewRouter(
	conversationService *service.ConversationService,
	logger *logger.Logger,
) *gin.Engine {
	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()

	// Middleware
	router.Use(middleware.Logger(logger))
	router.Use(middleware.Recovery(logger))
	router.Use(middleware.CORS())

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
			"service": "wiseinvest-api",
		})
	})

	// API v1
	v1 := router.Group("/api/v1")
	{
		// Conversation handler
		conversationHandler := handler.NewConversationHandler(conversationService, logger)

		// Agents
		agents := v1.Group("/agents")
		{
			agents.GET("", conversationHandler.GetAvailableAgents)
		}

		// Conversations
		conversations := v1.Group("/conversations")
		{
			conversations.POST("", conversationHandler.CreateConversation)
			conversations.GET("/:id", conversationHandler.GetConversation)
			conversations.GET("/user/:userId", conversationHandler.GetUserConversations)
			conversations.DELETE("/:id", conversationHandler.DeleteConversation)
		}

		// Messages
		messages := v1.Group("/messages")
		{
			messages.POST("", conversationHandler.SendMessage)
			messages.POST("/stream", conversationHandler.SendMessageStream)
		}
	}

	return router
}
