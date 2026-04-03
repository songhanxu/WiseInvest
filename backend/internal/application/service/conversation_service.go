package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/songhanxu/wiseinvest/internal/adapter/repository"
	"github.com/songhanxu/wiseinvest/internal/domain/agent"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/cache"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/llm"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// ConversationService handles conversation business logic
type ConversationService struct {
	conversationRepo *repository.ConversationRepository
	messageRepo      *repository.MessageRepository
	agentFactory     *agent.Factory
	cache            *cache.RedisClient
	logger           *logger.Logger
}

// NewConversationService creates a new conversation service
func NewConversationService(
	conversationRepo *repository.ConversationRepository,
	messageRepo *repository.MessageRepository,
	agentFactory *agent.Factory,
	cache *cache.RedisClient,
	logger *logger.Logger,
) *ConversationService {
	return &ConversationService{
		conversationRepo: conversationRepo,
		messageRepo:      messageRepo,
		agentFactory:     agentFactory,
		cache:            cache,
		logger:           logger,
	}
}

// CreateConversationRequest represents a request to create a conversation
type CreateConversationRequest struct {
	UserID    uint
	AgentType string
	Title     string
}

// CreateConversation creates a new conversation
func (s *ConversationService) CreateConversation(ctx context.Context, req CreateConversationRequest) (*model.Conversation, error) {
	// Validate agent type
	if _, err := s.agentFactory.CreateAgent(req.AgentType); err != nil {
		return nil, fmt.Errorf("invalid agent type: %w", err)
	}

	conversation := &model.Conversation{
		UserID:    req.UserID,
		AgentType: req.AgentType,
		Title:     req.Title,
		Status:    model.ConversationStatusActive,
		Metadata:  model.JSONB{},
	}

	if err := s.conversationRepo.Create(ctx, conversation); err != nil {
		return nil, err
	}

	s.logger.WithField("conversation_id", conversation.ID).Info("Conversation created")
	return conversation, nil
}

// SendMessageRequest represents a request to send a message
type SendMessageRequest struct {
	ConversationID uint
	Content        string
	Stream         bool
}

// SendMessageResponse represents a response to sending a message
type SendMessageResponse struct {
	Message          *model.Message
	AssistantMessage *model.Message
}

// SendMessage sends a message in a conversation
func (s *ConversationService) SendMessage(ctx context.Context, req SendMessageRequest) (*SendMessageResponse, error) {
	// Get conversation
	conversation, err := s.conversationRepo.GetByID(ctx, req.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("conversation not found: %w", err)
	}

	// Create user message
	userMessage := &model.Message{
		ConversationID: req.ConversationID,
		Role:           model.MessageRoleUser,
		Content:        req.Content,
	}

	if err := s.messageRepo.Create(ctx, userMessage); err != nil {
		return nil, fmt.Errorf("failed to create user message: %w", err)
	}

	// Get conversation history
	history, err := s.messageRepo.GetRecentMessages(ctx, req.ConversationID, 20)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation history: %w", err)
	}

	// Build history for agent
	agentHistory := make([]agent.HistoryMessage, 0, len(history)-1)
	for _, msg := range history {
		if msg.ID == userMessage.ID {
			continue // Skip the current message
		}
		agentHistory = append(agentHistory, agent.HistoryMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Create agent
	agentInstance, err := s.agentFactory.CreateAgent(conversation.AgentType)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	// Process message
	agentReq := agent.ProcessRequest{
		UserMessage:         req.Content,
		ConversationID:      req.ConversationID,
		ConversationHistory: agentHistory,
		Context:             make(map[string]interface{}),
	}

	resp, err := agentInstance.Process(ctx, agentReq)
	if err != nil {
		return nil, fmt.Errorf("failed to process message: %w", err)
	}

	// Create assistant message
	assistantMessage := &model.Message{
		ConversationID:   req.ConversationID,
		Role:             model.MessageRoleAssistant,
		Content:          resp.Content,
		PromptTokens:     resp.PromptTokens,
		CompletionTokens: resp.CompletionTokens,
		TotalTokens:      resp.TotalTokens,
		Metadata:         resp.Metadata,
	}

	if err := s.messageRepo.Create(ctx, assistantMessage); err != nil {
		return nil, fmt.Errorf("failed to create assistant message: %w", err)
	}

	// Update conversation
	conversation.UpdatedAt = time.Now()
	if conversation.Title == "" || conversation.Title == "New Conversation" {
		// Auto-generate title from first message
		if len(req.Content) > 50 {
			conversation.Title = req.Content[:50] + "..."
		} else {
			conversation.Title = req.Content
		}
	}
	if err := s.conversationRepo.Update(ctx, conversation); err != nil {
		s.logger.WithField("error", err).Warn("Failed to update conversation")
	}

	s.logger.WithField("conversation_id", req.ConversationID).Info("Message processed")

	return &SendMessageResponse{
		Message:          userMessage,
		AssistantMessage: assistantMessage,
	}, nil
}

// GetConversation gets a conversation with messages
func (s *ConversationService) GetConversation(ctx context.Context, id uint) (*model.Conversation, error) {
	return s.conversationRepo.GetWithMessages(ctx, id, 100)
}

// GetUserConversations gets all conversations for a user
func (s *ConversationService) GetUserConversations(ctx context.Context, userID uint) ([]model.Conversation, error) {
	return s.conversationRepo.GetByUserID(ctx, userID)
}

// DeleteConversation deletes a conversation
func (s *ConversationService) DeleteConversation(ctx context.Context, id uint) error {
	return s.conversationRepo.Delete(ctx, id)
}

// GetAvailableAgents gets available agent types
func (s *ConversationService) GetAvailableAgents() []agent.AgentInfo {
	return s.agentFactory.GetAvailableAgents()
}

// StreamMessageCallback is a callback function for streaming messages
type StreamMessageCallback func(content string) error

// SendMessageStream sends a message with streaming response
func (s *ConversationService) SendMessageStream(ctx context.Context, req SendMessageRequest, callback StreamMessageCallback) (*model.Message, error) {
	// Get conversation
	conversation, err := s.conversationRepo.GetByID(ctx, req.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("conversation not found: %w", err)
	}

	// Create user message
	userMessage := &model.Message{
		ConversationID: req.ConversationID,
		Role:           model.MessageRoleUser,
		Content:        req.Content,
	}

	if err := s.messageRepo.Create(ctx, userMessage); err != nil {
		return nil, fmt.Errorf("failed to create user message: %w", err)
	}

	// Get conversation history
	history, err := s.messageRepo.GetRecentMessages(ctx, req.ConversationID, 20)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation history: %w", err)
	}

	// Build history for agent
	agentHistory := make([]agent.HistoryMessage, 0, len(history)-1)
	for _, msg := range history {
		if msg.ID == userMessage.ID {
			continue
		}
		agentHistory = append(agentHistory, agent.HistoryMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Create agent
	agentInstance, err := s.agentFactory.CreateAgent(conversation.AgentType)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	// Process message with streaming
	agentReq := agent.ProcessRequest{
		UserMessage:         req.Content,
		ConversationID:      req.ConversationID,
		ConversationHistory: agentHistory,
		Context:             make(map[string]interface{}),
	}

	// Collect full response
	fullResponse := ""
	streamCallback := func(content string) error {
		if _, isThought := llm.ParseThoughtChunk(content); !isThought {
			fullResponse += content
		}
		return callback(content)
	}

	_ = callback(llm.ThoughtChunk("已接收问题，正在分析并准备数据"))
	if err := agentInstance.ProcessStream(ctx, agentReq, streamCallback); err != nil {
		return nil, fmt.Errorf("failed to process stream: %w", err)
	}

	// Create assistant message with full response
	assistantMessage := &model.Message{
		ConversationID: req.ConversationID,
		Role:           model.MessageRoleAssistant,
		Content:        fullResponse,
		Metadata: model.JSONB{
			"agent_type": conversation.AgentType,
			"streamed":   true,
		},
	}

	if err := s.messageRepo.Create(ctx, assistantMessage); err != nil {
		return nil, fmt.Errorf("failed to create assistant message: %w", err)
	}

	// Update conversation
	conversation.UpdatedAt = time.Now()
	if conversation.Title == "" || conversation.Title == "New Conversation" {
		if len(req.Content) > 50 {
			conversation.Title = req.Content[:50] + "..."
		} else {
			conversation.Title = req.Content
		}
	}
	if err := s.conversationRepo.Update(ctx, conversation); err != nil {
		s.logger.WithField("error", err).Warn("Failed to update conversation")
	}

	return userMessage, nil
}

// ── Group Chat ────────────────────────────────────────────────────────────────

// GroupChatEventCallback receives individual SSE events from persona agents.
// agentID is the short persona ID ("orchestrator", "value", "trend", "quant").
// eventType is one of: "agent_start", "content", "thought", "agent_end".
type GroupChatEventCallback func(agentID, eventType, content string) error

// SendGroupChatStream streams a multi-agent roundtable response.
// Each participant agent responds in canonical order; later agents receive the
// full text of earlier agents as additional context so they can build on each
// other's perspectives.
func (s *ConversationService) SendGroupChatStream(
	ctx context.Context,
	conversationID uint,
	content string,
	participants []string,
	callback GroupChatEventCallback,
) error {
	// Fetch conversation to validate it exists
	conv, err := s.conversationRepo.GetByID(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("conversation not found: %w", err)
	}

	// Persist user message
	userMsg := &model.Message{
		ConversationID: conversationID,
		Role:           model.MessageRoleUser,
		Content:        content,
	}
	if err := s.messageRepo.Create(ctx, userMsg); err != nil {
		return fmt.Errorf("failed to save user message: %w", err)
	}

	// Build conversation history (skip the message we just inserted)
	history, err := s.messageRepo.GetRecentMessages(ctx, conversationID, 20)
	if err != nil {
		return fmt.Errorf("failed to get history: %w", err)
	}
	agentHistory := make([]agent.HistoryMessage, 0, len(history))
	for _, msg := range history {
		if msg.ID == userMsg.ID {
			continue
		}
		agentHistory = append(agentHistory, agent.HistoryMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Create ordered persona agents
	groupAgents := s.agentFactory.CreateGroupChatAgents(participants)

	// Stream each agent sequentially; accumulate responses for cross-agent context
	priorResponses := make([]string, 0, len(groupAgents))
	var fullExchangeBuilder strings.Builder

	for _, ga := range groupAgents {
		agentID := ga.AgentID()
		_ = callback(agentID, "agent_start", "")

		agentReq := agent.ProcessRequest{
			UserMessage:         content,
			ConversationID:      conversationID,
			ConversationHistory: agentHistory,
			Context: map[string]interface{}{
				"prior_responses": priorResponses,
			},
		}

		var agentResponseBuilder strings.Builder

		streamCB := func(chunk string) error {
			if thought, ok := llm.ParseThoughtChunk(chunk); ok {
				return callback(agentID, "thought", thought)
			}
			agentResponseBuilder.WriteString(chunk)
			return callback(agentID, "content", chunk)
		}

		if err := ga.ProcessStream(ctx, agentReq, streamCB); err != nil {
			s.logger.WithField("error", err).Errorf("Group agent %s stream error", agentID)
			// Continue with remaining agents even if one fails
		}

		_ = callback(agentID, "agent_end", "")

		if resp := agentResponseBuilder.String(); resp != "" {
			name := agent.GroupPersonaDisplayName(agentID)
			priorResponses = append(priorResponses, fmt.Sprintf("[%s]: %s", name, resp))
			fmt.Fprintf(&fullExchangeBuilder, "\n\n[%s]: %s", name, resp)
		}
	}

	// Persist the combined assistant message
	fullExchange := strings.TrimSpace(fullExchangeBuilder.String())
	if fullExchange != "" {
		assistantMsg := &model.Message{
			ConversationID: conversationID,
			Role:           model.MessageRoleAssistant,
			Content:        fullExchange,
			Metadata: model.JSONB{
				"type":         "group_chat",
				"participants": participants,
			},
		}
		if err := s.messageRepo.Create(ctx, assistantMsg); err != nil {
			s.logger.WithField("error", err).Warn("Failed to save group chat response")
		}
	}

	// Update conversation metadata
	conv.UpdatedAt = time.Now()
	if conv.Title == "" || conv.Title == "New Conversation" {
		if len(content) > 50 {
			conv.Title = content[:50] + "..."
		} else {
			conv.Title = content
		}
	}
	_ = s.conversationRepo.Update(ctx, conv)
	return nil
}
