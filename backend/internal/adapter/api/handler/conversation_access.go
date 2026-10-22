package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

type conversationOwnerChecker interface {
	CheckConversationOwner(context.Context, uint, uint) error
}

func checkConversationOwner(c *gin.Context, service conversationOwnerChecker, id uint) bool {
	if err := service.CheckConversationOwner(c.Request.Context(), id, c.GetUint("userID")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在或不属于当前账号"})
		return false
	}
	return true
}
