package handler

import (
	"bufio"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/application/service"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// ConversationHandler handles conversation-related HTTP requests
type ConversationHandler struct {
	service *service.ConversationService
	logger  *logger.Logger
}

// NewConversationHandler creates a new conversation handler
func NewConversationHandler(service *service.ConversationService, logger *logger.Logger) *ConversationHandler {
	return &ConversationHandler{
		service: service,
		logger:  logger,
	}
}

// CreateConversationRequest represents a request to create a conversation
type CreateConversationRequest struct {
	UserID    uint   `json:"user_id" binding:"required"`
	AgentType string `json:"agent_type" binding:"required"`
	Title     string `json:"title"`
}

// CreateConversation handles POST /api/v1/conversations
func (h *ConversationHandler) CreateConversation(c *gin.Context) {
	var req CreateConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Title == "" {
		req.Title = "New Conversation"
	}

	conversation, err := h.service.CreateConversation(c.Request.Context(), service.CreateConversationRequest{
		UserID:    req.UserID,
		AgentType: req.AgentType,
		Title:     req.Title,
	})
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to create conversation")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create conversation"})
		return
	}

	c.JSON(http.StatusCreated, conversation)
}

// GetConversation handles GET /api/v1/conversations/:id
func (h *ConversationHandler) GetConversation(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID"})
		return
	}

	conversation, err := h.service.GetConversation(c.Request.Context(), uint(id))
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to get conversation")
		c.JSON(http.StatusNotFound, gin.H{"error": "Conversation not found"})
		return
	}

	c.JSON(http.StatusOK, conversation)
}

// GetUserConversations handles GET /api/v1/conversations/user/:userId
func (h *ConversationHandler) GetUserConversations(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("userId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	conversations, err := h.service.GetUserConversations(c.Request.Context(), uint(userID))
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to get user conversations")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get conversations"})
		return
	}

	c.JSON(http.StatusOK, conversations)
}

// DeleteConversation handles DELETE /api/v1/conversations/:id
func (h *ConversationHandler) DeleteConversation(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID"})
		return
	}

	if err := h.service.DeleteConversation(c.Request.Context(), uint(id)); err != nil {
		h.logger.WithField("error", err).Error("Failed to delete conversation")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete conversation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation deleted"})
}

// SendMessageRequest represents a request to send a message
type SendMessageRequest struct {
	ConversationID uint   `json:"conversation_id" binding:"required"`
	Content        string `json:"content" binding:"required"`
}

// SendMessage handles POST /api/v1/messages
func (h *ConversationHandler) SendMessage(c *gin.Context) {
	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.service.SendMessage(c.Request.Context(), service.SendMessageRequest{
		ConversationID: req.ConversationID,
		Content:        req.Content,
		Stream:         false,
	})
	if err != nil {
		h.logger.WithField("error", err).Error("Failed to send message")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send message"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_message":      resp.Message,
		"assistant_message": resp.AssistantMessage,
	})
}

// SendMessageStream handles POST /api/v1/messages/stream
func (h *ConversationHandler) SendMessageStream(c *gin.Context) {
	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set headers for SSE
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	// Get response writer
	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return
	}

	// Create buffered writer
	writer := bufio.NewWriter(w)

	// Stream callback
	callback := func(content string) error {
		// Write SSE format with JSON payload
		jsonData := fmt.Sprintf(`{"content":"%s"}`, content)
		_, err := fmt.Fprintf(writer, "data: %s\n\n", jsonData)
		if err != nil {
			return err
		}
		writer.Flush()
		flusher.Flush()
		return nil
	}

	// Send message with streaming
	_, err := h.service.SendMessageStream(c.Request.Context(), service.SendMessageRequest{
		ConversationID: req.ConversationID,
		Content:        req.Content,
		Stream:         true,
	}, callback)

	if err != nil {
		h.logger.WithField("error", err).Error("Failed to stream message")
		fmt.Fprintf(writer, "data: {\"error\": \"Failed to stream message\"}\n\n")
		writer.Flush()
		flusher.Flush()
		return
	}

	// Send done signal
	fmt.Fprintf(writer, "data: [DONE]\n\n")
	writer.Flush()
	flusher.Flush()
}

// GetAvailableAgents handles GET /api/v1/agents
func (h *ConversationHandler) GetAvailableAgents(c *gin.Context) {
	agents := h.service.GetAvailableAgents()
	c.JSON(http.StatusOK, agents)
}
