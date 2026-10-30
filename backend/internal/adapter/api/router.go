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
	deviceHandler *handler.DeviceHandler,
	stockHandler *handler.StockHandler,
	accountHandler *handler.AccountHandler,
	alertHandler *handler.PriceAlertHandler,
	jwtSvc *auth.JWTService,
	logger *logger.Logger,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	stockHandler.SetAccountHandler(accountHandler)

	router := gin.New()
	_ = router.SetTrustedProxies([]string{"127.0.0.1", "::1", "172.16.0.0/12"})
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
			authGroup.POST("/register", authHandler.Register)
			authGroup.POST("/login", authHandler.Login)
			authGroup.POST("/email/send-code", authHandler.SendEmailCode)
			authGroup.POST("/email/login", authHandler.EmailLogin)
			authGroup.POST("/wechat/login", authHandler.WeChatLogin)
			authGroup.POST("/wechat/miniprogram/login", authHandler.MiniProgramLogin)
			authGroup.POST("/phone/send-code", authHandler.SendPhoneCode)
			authGroup.POST("/phone/login", authHandler.PhoneLogin)
		}

		// ── Auth (requires token) ─────────────────────────────────────────
		authProtected := v1.Group("/auth")
		authProtected.Use(middleware.Auth(jwtSvc))
		{
			authProtected.POST("/phone/bind", authHandler.BindPhone)
			authProtected.POST("/email/bind", authHandler.BindEmail)
			authProtected.GET("/me", authHandler.GetMe)
		}

		// ── Agents (public) ───────────────────────────────────────────────
		conversationHandler := handler.NewConversationHandler(conversationService, logger)
		v1.GET("/agents", conversationHandler.GetAvailableAgents)
		v1.GET("/automation/bitget/holdings", accountHandler.GetAutomationBitgetHoldings)

		// ── Stock Market Data (public — no auth needed) ─────────────────
		stocks := v1.Group("/stocks")
		{
			stocks.GET("/indices", stockHandler.GetIndices)
			stocks.GET("/search", stockHandler.SearchStocks)
			stocks.GET("/quote", stockHandler.GetStockQuote)
			stocks.GET("/fundamentals", stockHandler.GetFundamentals)
			stocks.GET("/kline", stockHandler.GetKLineData)
			stocks.GET("/news", stockHandler.GetStockNews)
			stocks.GET("/news/enhance", stockHandler.EnhanceNewsItem)
			stocks.POST("/news/enhance/batch", stockHandler.EnhanceBatchNews)
			stocks.POST("/news/batch", stockHandler.GetBatchNews)
			stocks.POST("/analysis/enhance", stockHandler.EnhanceAnalysis)
			stocks.GET("/analysis/conclusion", stockHandler.GetAnalysisConclusion)
			stocks.POST("/analysis/batch-conclusion", stockHandler.BatchAnalysisConclusion)
			stocks.POST("/analysis/batch-enhance", stockHandler.BatchEnhanceAnalysis)
			stocks.GET("/analysis/cached", stockHandler.GetCachedAnalysis)
		}

		// ── WebSocket (public — real-time market data push) ──────────────
		v1.GET("/ws", stockHandler.WSHub().HandleWS)

		// ── Protected API ─────────────────────────────────────────────────
		protected := v1.Group("")
		protected.Use(middleware.Auth(jwtSvc))
		{
			// Device token registration for push notifications
			devices := protected.Group("/devices")
			{
				devices.POST("/token", deviceHandler.RegisterToken)
			}

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

				// Multi-agent roundtable (慧投圆桌)
				groupChatHandler := handler.NewGroupChatHandler(conversationService, logger)
				messages.POST("/group-chat/stream", groupChatHandler.SendGroupChatStream)
			}

			// ── Watchlist CRUD (requires auth, bound to UserID) ──────────
			protected.GET("/price-alerts", alertHandler.List)
			protected.PUT("/price-alerts/settings", alertHandler.Settings)
			protected.PUT("/price-alerts", alertHandler.Save)
			protected.DELETE("/price-alerts", alertHandler.Remove)
			protected.GET("/accounts", accountHandler.GetAccounts)
			protected.GET("/accounts/:accountID/holdings", accountHandler.GetHoldings)
			watchlist := protected.Group("/stocks")
			{
				watchlist.GET("/watchlist", stockHandler.GetWatchlist)
				watchlist.GET("/watchlist/all", stockHandler.GetAllWatchlist)
				watchlist.POST("/watchlist", stockHandler.AddToWatchlist)
				watchlist.DELETE("/watchlist", stockHandler.RemoveFromWatchlist)
				watchlist.GET("/watchlist/filing-insights", stockHandler.GetFilingInsights)
				watchlist.POST("/watchlist/filing-insights/refresh", stockHandler.RefreshFilingInsights)
			}
		}
	}

	return router
}
