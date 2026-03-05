package api

import (
	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/adapter/api/handler"
	"github.com/songhanxu/wiseinvest/internal/adapter/api/middleware"
	"github.com/songhanxu/wiseinvest/internal/application/service"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/auth"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// NewRouter creates a new HTTP router
func NewRouter(
	conversationService *service.ConversationService,
	authHandler *handler.AuthHandler,
	jwtSvc *auth.JWTService,
	logger *logger.Logger,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(middleware.Logger(logger))
	router.Use(middleware.Recovery(logger))
	router.Use(middleware.CORS())

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "wiseinvest-api"})
	})

	v1 := router.Group("/api/v1")
	{
		// ── Auth (public) ─────────────────────────────────────────────────
		authGroup := v1.Group("/auth")
		{
			authGroup.POST("/wechat/login", authHandler.WeChatLogin)
			authGroup.POST("/phone/send-code", authHandler.SendPhoneCode)
		}

		// ── Auth (requires token) ─────────────────────────────────────────
		authProtected := v1.Group("/auth")
		authProtected.Use(middleware.Auth(jwtSvc))
		{
			authProtected.POST("/phone/bind", authHandler.BindPhone)
			authProtected.GET("/me", authHandler.GetMe)
		}

		// ── Agents (public) ───────────────────────────────────────────────
		conversationHandler := handler.NewConversationHandler(conversationService, logger)
		v1.GET("/agents", conversationHandler.GetAvailableAgents)

		// ── Protected API ─────────────────────────────────────────────────
		protected := v1.Group("")
		protected.Use(middleware.Auth(jwtSvc))
		{
			conversations := protected.Group("/conversations")
			{
				conversations.POST("", conversationHandler.CreateConversation)
				conversations.GET("/:id", conversationHandler.GetConversation)
				conversations.GET("/user/:userId", conversationHandler.GetUserConversations)
				conversations.DELETE("/:id", conversationHandler.DeleteConversation)
			}

			messages := protected.Group("/messages")
			{
				messages.POST("", conversationHandler.SendMessage)
				messages.POST("/stream", conversationHandler.SendMessageStream)
			}
		}
	}

	return router
}
