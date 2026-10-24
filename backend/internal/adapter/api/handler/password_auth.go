package handler

import (
	"sync"
	"time"
)

// The limiter bounds password hashing and login attempts; entries expire together.
type authLimit struct {
	mu     sync.Mutex
	until  time.Time
	counts map[string]int
}

func (l *authLimit) allow(key string, maximum int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if time.Now().After(l.until) {
		l.until = time.Now().Add(time.Minute)
		l.counts = map[string]int{}
	}
	if l.counts[key] >= maximum || len(l.counts) >= 10000 {
		return false
	}
	l.counts[key]++
	return true
}
