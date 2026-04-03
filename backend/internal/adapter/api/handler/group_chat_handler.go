package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/songhanxu/wiseinvest/internal/application/service"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// GroupChatHandler handles multi-agent roundtable streaming requests.
type GroupChatHandler struct {
	service *service.ConversationService
	logger  *logger.Logger
}

// NewGroupChatHandler creates a new GroupChatHandler.
func NewGroupChatHandler(svc *service.ConversationService, log *logger.Logger) *GroupChatHandler {
	return &GroupChatHandler{service: svc, logger: log}
}

// groupChatRequest is the JSON body for POST /api/v1/messages/group-chat/stream.
type groupChatRequest struct {
	ConversationID uint     `json:"conversation_id" binding:"required"`
	Content        string   `json:"content"         binding:"required"`
	// Participants lists the agent IDs to include (e.g. ["orchestrator","value"]).
	// Leave empty to include all four agents in the default order.
	Participants []string `json:"participants"`
}

// SendGroupChatStream handles POST /api/v1/messages/group-chat/stream.
// It streams SSE events for each participating persona agent in sequence.
//
// SSE event format:
//
//	data: {"type":"agent_start","agent_id":"value","content":""}
//	data: {"type":"content",    "agent_id":"value","content":"...chunk..."}
//	data: {"type":"thought",    "agent_id":"value","content":"...thinking..."}
//	data: {"type":"agent_end",  "agent_id":"value","content":""}
//	data: [DONE]
func (h *GroupChatHandler) SendGroupChatStream(c *gin.Context) {
	var req groupChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return
	}

	writer := bufio.NewWriter(w)

	sendEvent := func(agentID, eventType, content string) error {
		payload, err := json.Marshal(map[string]string{
			"type":     eventType,
			"agent_id": agentID,
			"content":  content,
		})
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(writer, "data: %s\n\n", payload); err != nil {
			return err
		}
		writer.Flush()
		flusher.Flush()
		return nil
	}

	// Heartbeat goroutine — prevents iOS URLSession from timing out during
	// slow LLM inference.  SSE comment lines are ignored by the client parser.
	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fmt.Fprintf(writer, ": heartbeat\n\n")
				writer.Flush()
				flusher.Flush()
			case <-heartbeatDone:
				return
			case <-c.Request.Context().Done():
				return
			}
		}
	}()

	err := h.service.SendGroupChatStream(
		c.Request.Context(),
		req.ConversationID,
		req.Content,
		req.Participants,
		sendEvent,
	)
	close(heartbeatDone)

	if err != nil {
		h.logger.WithField("error", err).Error("Group chat stream failed")
		fmt.Fprintf(writer, "data: {\"error\":\"Group chat stream failed\"}\n\n")
		writer.Flush()
		flusher.Flush()
		return
	}

	fmt.Fprintf(writer, "data: [DONE]\n\n")
	writer.Flush()
	flusher.Flush()
}
