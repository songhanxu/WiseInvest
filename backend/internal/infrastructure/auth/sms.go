package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/cache"
)

const (
	smsCachePrefix = "sms:code:"
	smsTTL         = 5 * time.Minute
	smsCooldown    = 60 * time.Second
)

var (
	ErrSMSUnavailable = errors.New("sms provider is not configured")
	ErrSMSRateLimited = errors.New("sms rate limit exceeded")
)

type SMSSender interface {
	Send(ctx context.Context, phone, code string) error
}

// SMSService manages one-time codes and uses an explicit sender. No production
// request can succeed until an SMS provider has been configured.
type SMSService struct {
	redis  *cache.RedisClient
	sender SMSSender
}

func NewSMSService(redisClient *cache.RedisClient) *SMSService {
	var sender SMSSender
	if tencent := NewTencentSMSSenderFromEnv(); tencent != nil {
		sender = tencent
	} else if os.Getenv("SERVER_MODE") == "debug" && os.Getenv("ALLOW_MOCK_AUTH") == "true" {
		sender = localSMSSender{}
	}
	return &SMSService{redis: redisClient, sender: sender}
}

func (s *SMSService) IsConfigured() bool { return s.sender != nil }

func (s *SMSService) SendCode(ctx context.Context, phone, ip string) error {
	if s.sender == nil {
		return ErrSMSUnavailable
	}
	phoneKey := smsCachePrefix + phone
	phoneCount, err := s.redis.IncrementWithExpiry(ctx, "sms:send:phone:"+phone, time.Hour)
	if err != nil {
		return err
	}
	ipCount, err := s.redis.IncrementWithExpiry(ctx, "sms:send:ip:"+hashIdentifier(ip), time.Hour)
	if err != nil {
		return err
	}
	if phoneCount > 5 || ipCount > 10 {
		return ErrSMSRateLimited
	}
	allowed, err := s.redis.SetNX(ctx, "sms:cooldown:"+phone, "1", smsCooldown)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrSMSRateLimited
	}

	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	hash := sha256.Sum256([]byte(code))
	if err := s.redis.Set(ctx, phoneKey, hex.EncodeToString(hash[:]), smsTTL); err != nil {
		_ = s.redis.Delete(ctx, "sms:cooldown:"+phone)
		return err
	}
	if err := s.sender.Send(ctx, phone, code); err != nil {
		_ = s.redis.Delete(ctx, phoneKey, "sms:cooldown:"+phone)
		return err
	}
	return nil
}

func (s *SMSService) VerifyCode(ctx context.Context, phone, code string) (bool, error) {
	count, err := s.redis.IncrementWithExpiry(ctx, "sms:verify:"+phone, smsTTL)
	if err != nil {
		return false, err
	}
	if count > 5 {
		return false, ErrSMSRateLimited
	}
	stored, err := s.redis.Get(ctx, smsCachePrefix+phone)
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	provided := sha256.Sum256([]byte(code))
	if subtle.ConstantTimeCompare([]byte(stored), []byte(hex.EncodeToString(provided[:]))) != 1 {
		return false, nil
	}
	_ = s.redis.Delete(ctx, smsCachePrefix+phone, "sms:verify:"+phone)
	return true, nil
}

func hashIdentifier(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:8])
}

type localSMSSender struct{}

func (localSMSSender) Send(_ context.Context, phone, code string) error {
	fmt.Printf("[SMS DEV ONLY] %s: %s\n", phone, code)
	return nil
}
