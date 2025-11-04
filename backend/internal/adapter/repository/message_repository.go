package repository

import (
	"context"
	"fmt"

	"github.com/songhanxu/wiseinvest/internal/domain/model"
	"gorm.io/gorm"
)

// MessageRepository handles message data operations
type MessageRepository struct {
	db *gorm.DB
}

// NewMessageRepository creates a new message repository
func NewMessageRepository(db *gorm.DB) *MessageRepository {
	return &MessageRepository{db: db}
}

// Create creates a new message
func (r *MessageRepository) Create(ctx context.Context, message *model.Message) error {
	if err := r.db.WithContext(ctx).Create(message).Error; err != nil {
		return fmt.Errorf("failed to create message: %w", err)
	}
	return nil
}

// GetByConversationID gets all messages for a conversation
func (r *MessageRepository) GetByConversationID(ctx context.Context, conversationID uint, limit int) ([]model.Message, error) {
	var messages []model.Message
	query := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("created_at ASC")
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	
	if err := query.Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}
	return messages, nil
}

// GetRecentMessages gets recent messages for a conversation
func (r *MessageRepository) GetRecentMessages(ctx context.Context, conversationID uint, limit int) ([]model.Message, error) {
	var messages []model.Message
	if err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("created_at DESC").
		Limit(limit).
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get recent messages: %w", err)
	}
	
	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	
	return messages, nil
}

// Delete deletes a message
func (r *MessageRepository) Delete(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Delete(&model.Message{}, id).Error; err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}
	return nil
}

// DeleteByConversationID deletes all messages for a conversation
func (r *MessageRepository) DeleteByConversationID(ctx context.Context, conversationID uint) error {
	if err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Delete(&model.Message{}).Error; err != nil {
		return fmt.Errorf("failed to delete messages: %w", err)
	}
	return nil
}
