package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// Recovery returns a gin middleware for panic recovery
func Recovery(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.WithField("error", err).Error("Panic recovered")
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "Internal server error",
				})
				c.Abort()
			}
		}()
		c.Next()
	}
}
