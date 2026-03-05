package auth

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/cache"
)

const (
	smsCachePrefix = "sms:code:"
	smsTTL         = 5 * time.Minute
)

// SMSService handles SMS verification code sending and verification
type SMSService struct {
	redis *cache.RedisClient
}

// NewSMSService creates a new SMS service
func NewSMSService(redis *cache.RedisClient) *SMSService {
	return &SMSService{redis: redis}
}

// SendCode generates and sends (mocked) an SMS verification code
func (s *SMSService) SendCode(ctx context.Context, phone string) error {
	code := fmt.Sprintf("%06d", rand.Intn(1000000)) //nolint:gosec
	key := smsCachePrefix + phone

	if err := s.redis.Set(ctx, key, code, smsTTL); err != nil {
		return fmt.Errorf("failed to cache SMS code: %w", err)
	}

	// In production, replace with real SMS provider (e.g. Aliyun, Tencent Cloud SMS)
	fmt.Printf("[SMS] Phone: %s  Code: %s  (expires in 5 min)\n", phone, code)
	return nil
}

// VerifyCode verifies an SMS code for a phone number
func (s *SMSService) VerifyCode(ctx context.Context, phone, code string) (bool, error) {
	key := smsCachePrefix + phone
	stored, err := s.redis.Get(ctx, key)
	if err != nil {
		// Key not found means code expired or never sent
		return false, nil
	}

	if stored == code {
		_ = s.redis.Delete(ctx, key)
		return true, nil
	}
	return false, nil
}

// init seeds the random number generator
func init() {
	rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:staticcheck
}
