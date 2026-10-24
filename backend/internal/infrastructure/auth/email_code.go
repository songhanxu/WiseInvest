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

var ErrEmailUnavailable = errors.New("email provider is not configured")

type EmailSender interface {
	Send(ctx context.Context, address, code string) error
}

// EmailCodeService uses a separate Redis namespace from SMS codes. A code is
// single-use and expires after five minutes.
type EmailCodeService struct {
	redis  *cache.RedisClient
	sender EmailSender
}

func NewEmailCodeService(redisClient *cache.RedisClient) *EmailCodeService {
	var sender EmailSender = newSMTPEmailSenderFromEnv()
	if sender == nil && os.Getenv("SERVER_MODE") == "debug" && os.Getenv("ALLOW_MOCK_AUTH") == "true" {
		sender = localEmailSender{}
	}
	return &EmailCodeService{redis: redisClient, sender: sender}
}

func (s *EmailCodeService) IsConfigured() bool { return s != nil && s.sender != nil }

func (s *EmailCodeService) SendCode(ctx context.Context, address, ip string) error {
	if !s.IsConfigured() {
		return ErrEmailUnavailable
	}
	addressKey := hashIdentifier(address)
	addressCount, err := s.redis.IncrementWithExpiry(ctx, "email:send:address:"+addressKey, time.Hour)
	if err != nil {
		return err
	}
	ipCount, err := s.redis.IncrementWithExpiry(ctx, "email:send:ip:"+hashIdentifier(ip), time.Hour)
	if err != nil {
		return err
	}
	if addressCount > 5 || ipCount > 10 {
		return ErrSMSRateLimited
	}
	allowed, err := s.redis.SetNX(ctx, "email:cooldown:"+addressKey, "1", time.Minute)
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
	codeKey := "email:code:" + addressKey
	if err := s.redis.Set(ctx, codeKey, hex.EncodeToString(hash[:]), 5*time.Minute); err != nil {
		_ = s.redis.Delete(ctx, "email:cooldown:"+addressKey)
		return err
	}
	if err := s.sender.Send(ctx, address, code); err != nil {
		_ = s.redis.Delete(ctx, codeKey, "email:cooldown:"+addressKey)
		return err
	}
	return nil
}

func (s *EmailCodeService) VerifyCode(ctx context.Context, address, code string) (bool, error) {
	if !s.IsConfigured() {
		return false, ErrEmailUnavailable
	}
	addressKey := hashIdentifier(address)
	count, err := s.redis.IncrementWithExpiry(ctx, "email:verify:"+addressKey, 5*time.Minute)
	if err != nil {
		return false, err
	}
	if count > 5 {
		return false, ErrSMSRateLimited
	}
	stored, err := s.redis.Get(ctx, "email:code:"+addressKey)
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
	_ = s.redis.Delete(ctx, "email:code:"+addressKey, "email:verify:"+addressKey)
	return true, nil
}

type localEmailSender struct{}

func (localEmailSender) Send(_ context.Context, address, code string) error {
	fmt.Printf("[EMAIL DEV ONLY] %s: %s\n", address, code)
	return nil
}
