package service

import (
	"context"
	"fmt"
	"time"

	"github.com/songhanxu/wiseinvest/internal/adapter/repository"
	"github.com/songhanxu/wiseinvest/internal/domain/agent"
	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/cache"
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
		fullResponse += content
		return callback(content)
	}

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
