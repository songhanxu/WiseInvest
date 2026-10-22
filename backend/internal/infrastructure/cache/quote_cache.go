package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// QuoteCache provides a thin wrapper around Redis for high-frequency market quote caching.
// The background ticker in StockHandler writes fresh data; HTTP handlers read from here.
type QuoteCache struct {
	redis  *RedisClient
	logger *logger.Logger
}

// NewQuoteCache creates a new QuoteCache backed by the given Redis client.
func NewQuoteCache(redis *RedisClient, logger *logger.Logger) *QuoteCache {
	return &QuoteCache{redis: redis, logger: logger}
}

// Key helpers -----------------------------------------------------------------

// IndicesKey returns the Redis key for market indices (e.g. "quote:indices:a_share").
func IndicesKey(market string) string {
	return fmt.Sprintf("quote:indices:%s", market)
}

// StockQuoteKey returns the Redis key for a single stock quote.
func StockQuoteKey(market, code string) string {
	return fmt.Sprintf("quote:stock:%s:%s", market, code)
}

// WatchlistQuotesKey returns the Redis key for batch watchlist quotes.
func WatchlistQuotesKey(market string, codes string) string {
	return fmt.Sprintf("quote:watchlist:%s:%s", market, codes)
}

// SetJSON serializes value as JSON and stores it in Redis with the given TTL.
func (qc *QuoteCache) SetJSON(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal cache value: %w", err)
	}
	return qc.redis.Set(ctx, key, string(data), ttl)
}

// GetJSON reads a key from Redis and deserializes it into dest.
// Returns false if the key does not exist (cache miss).
func (qc *QuoteCache) GetJSON(ctx context.Context, key string, dest interface{}) (bool, error) {
	raw, err := qc.redis.Get(ctx, key)
	if err != nil {
		// go-redis returns redis.Nil on cache miss
		return false, nil
	}
	if raw == "" {
		return false, nil
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return false, fmt.Errorf("unmarshal cache value: %w", err)
	}
	return true, nil
}
