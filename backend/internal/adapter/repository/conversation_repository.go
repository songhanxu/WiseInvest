package repository

import (
	"context"
	"fmt"

	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"gorm.io/gorm"
)

// ConversationRepository handles conversation data operations
type ConversationRepository struct {
	db *gorm.DB
}

// NewConversationRepository creates a new conversation repository
func NewConversationRepository(db *gorm.DB) *ConversationRepository {
	return &ConversationRepository{db: db}
}

// Create creates a new conversation
func (r *ConversationRepository) Create(ctx context.Context, conversation *model.Conversation) error {
	if err := r.db.WithContext(ctx).Create(conversation).Error; err != nil {
		return fmt.Errorf("failed to create conversation: %w", err)
	}
	return nil
}

// GetByID gets a conversation by ID
func (r *ConversationRepository) GetByID(ctx context.Context, id uint) (*model.Conversation, error) {
	var conversation model.Conversation
	if err := r.db.WithContext(ctx).First(&conversation, id).Error; err != nil {
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}
	return &conversation, nil
}

// GetByUserID gets all conversations for a user
func (r *ConversationRepository) GetByUserID(ctx context.Context, userID uint) ([]model.Conversation, error) {
	var conversations []model.Conversation
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND status != ?", userID, model.ConversationStatusDeleted).
		Order("updated_at DESC").
		Find(&conversations).Error; err != nil {
		return nil, fmt.Errorf("failed to get conversations: %w", err)
	}
	return conversations, nil
}

// Update updates a conversation
func (r *ConversationRepository) Update(ctx context.Context, conversation *model.Conversation) error {
	if err := r.db.WithContext(ctx).Save(conversation).Error; err != nil {
		return fmt.Errorf("failed to update conversation: %w", err)
	}
	return nil
}

// Delete soft deletes a conversation
func (r *ConversationRepository) Delete(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).
		Model(&model.Conversation{}).
		Where("id = ?", id).
		Update("status", model.ConversationStatusDeleted).Error; err != nil {
		return fmt.Errorf("failed to delete conversation: %w", err)
	}
	return nil
}

// GetWithMessages gets a conversation with its messages
func (r *ConversationRepository) GetWithMessages(ctx context.Context, id uint, limit int) (*model.Conversation, error) {
	var conversation model.Conversation
	query := r.db.WithContext(ctx).Preload("Messages", func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at ASC").Limit(limit)
	})
	
	if err := query.First(&conversation, id).Error; err != nil {
		return nil, fmt.Errorf("failed to get conversation with messages: %w", err)
	}
	return &conversation, nil
}
