package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
	"golang.org/x/net/proxy"
)

// ──────────────────────────────────────────────────────────────────────────────
// Binance WebSocket Stream Client
//
// Connects to Binance's real-time WebSocket stream for crypto ticker data,
// replacing the HTTP polling approach for near-instant price updates.
//
// Endpoints (in priority order):
//   - Primary:  wss://data-stream.binance.vision (accessible from mainland China)
//   - Fallback: wss://stream.binance.com:9443     (blocked in some regions)
//   - Fallback: wss://stream.binance.com:443      (blocked in some regions)
//
// Environment variables:
//   - BINANCE_WS_DISABLED=true  — disable WebSocket entirely, rely on HTTP fallback
//   - BINANCE_WS_PROXY=socks5://host:port — route WebSocket through a SOCKS5 proxy
//
// We subscribe to the combined stream for individual <symbol>@ticker streams
// which push 24hr rolling window data every ~1 second per symbol.
// ──────────────────────────────────────────────────────────────────────────────

const (
	// data-stream.binance.vision resolves to different IPs than stream.binance.com
	// and is not blocked by the GFW. It's the same backend service.
	binancePrimaryWS   = "wss://data-stream.binance.vision"
	binanceFallbackWS1 = "wss://stream.binance.com:9443"
	binanceFallbackWS2 = "wss://stream.binance.com:443"

	// Reconnection parameters
	wsReconnectBaseDelay = 1 * time.Second
	wsReconnectMaxDelay  = 5 * time.Minute // long backoff when all endpoints are blocked

	// Binance sends a ping every 3 minutes; we must respond within 10 min.
	wsPongTimeout = 5 * time.Minute
	wsPingPeriod  = 2 * time.Minute // send client-side pings periodically
)

// BinanceWSCallback is called whenever a new ticker update arrives.
type BinanceWSCallback func(tickers map[string]BinanceTicker24hr)

// BinanceWSClient manages a persistent WebSocket connection to Binance streams.
type BinanceWSClient struct {
	logger  *logger.Logger
	symbols []string // e.g. ["btcusdt", "ethusdt"]

	callback BinanceWSCallback
	disabled bool // set via BINANCE_WS_DISABLED=true

	// Latest ticker data — always available for readers even during reconnect
	mu          sync.RWMutex
	lastTickers map[string]BinanceTicker24hr

	// Connection management
	conn             *websocket.Conn
	connMu           sync.Mutex
	connected        atomic.Bool
	wasEverConnected atomic.Bool // set true when dial succeeds; used by Start() to reset backoff

	// Logging throttle: don't spam warnings on every retry cycle
	consecutiveFailures atomic.Int64

	// Custom dialer (may include SOCKS5 proxy)
	dialer *websocket.Dialer

	cancel context.CancelFunc
}

// BinanceWSTickerEvent matches the Binance Individual Symbol Ticker Stream payload.
// Stream: <symbol>@ticker
// Docs: https://developers.binance.com/docs/binance-spot-api-docs/web-socket-streams#individual-symbol-ticker-streams
type BinanceWSTickerEvent struct {
	EventType          string `json:"e"` // "24hrTicker"
	EventTime          int64  `json:"E"` // Event time (ms)
	Symbol             string `json:"s"` // "BTCUSDT"
	PriceChange        string `json:"p"` // Price change
	PriceChangePercent string `json:"P"` // Price change percent
	WeightedAvgPrice   string `json:"w"` // Weighted average price
	PrevClosePrice     string `json:"x"` // Previous close price
	LastPrice          string `json:"c"` // Last price
	LastQty            string `json:"Q"` // Last quantity
	OpenPrice          string `json:"o"` // Open price
	HighPrice          string `json:"h"` // High price
	LowPrice          string `json:"l"` // Low price
	Volume             string `json:"v"` // Total traded base asset volume
	QuoteVolume        string `json:"q"` // Total traded quote asset volume
}

// BinanceCombinedStreamMsg wraps stream name + data for combined streams.
type BinanceCombinedStreamMsg struct {
	Stream string                `json:"stream"` // e.g. "btcusdt@ticker"
	Data   json.RawMessage       `json:"data"`
}

// NewBinanceWSClient creates a new Binance WebSocket stream client.
// symbols should be uppercase like ["BTCUSDT", "ETHUSDT"].
//
// Environment variables:
//   - BINANCE_WS_DISABLED=true — client will be created but Start() returns immediately
//   - BINANCE_WS_PROXY=socks5://host:port — route WS through a SOCKS5 proxy
func NewBinanceWSClient(log *logger.Logger, symbols []string, cb BinanceWSCallback) *BinanceWSClient {
	lower := make([]string, len(symbols))
	for i, s := range symbols {
		lower[i] = strings.ToLower(s)
	}

	disabled := strings.EqualFold(os.Getenv("BINANCE_WS_DISABLED"), "true")

	// Build dialer, optionally with SOCKS5 proxy
	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	if proxyURL := os.Getenv("BINANCE_WS_PROXY"); proxyURL != "" && !disabled {
		if parsed, err := url.Parse(proxyURL); err == nil {
			switch parsed.Scheme {
			case "socks5", "socks5h":
				var auth *proxy.Auth
				if parsed.User != nil {
					pwd, _ := parsed.User.Password()
					auth = &proxy.Auth{
						User:     parsed.User.Username(),
						Password: pwd,
					}
				}
				socksDialer, dialErr := proxy.SOCKS5("tcp", parsed.Host, auth, proxy.Direct)
				if dialErr == nil {
					dialer.NetDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
						return socksDialer.Dial(network, addr)
					}
					log.WithField("proxy", parsed.Host).Info("[BinanceWS] Using SOCKS5 proxy")
				} else {
					log.WithField("error", dialErr).Warn("[BinanceWS] Failed to create SOCKS5 dialer, connecting directly")
				}
			case "http", "https":
				dialer.Proxy = http.ProxyURL(parsed)
				log.WithField("proxy", parsed.Host).Info("[BinanceWS] Using HTTP proxy")
			default:
				log.WithField("scheme", parsed.Scheme).Warn("[BinanceWS] Unsupported proxy scheme, connecting directly")
			}
		} else {
			log.WithField("error", err).Warn("[BinanceWS] Failed to parse BINANCE_WS_PROXY, connecting directly")
		}
	}

	if disabled {
		log.Info("[BinanceWS] Disabled via BINANCE_WS_DISABLED=true, using HTTP fallback only")
	}

	return &BinanceWSClient{
		logger:      log,
		symbols:     lower,
		callback:    cb,
		lastTickers: make(map[string]BinanceTicker24hr),
		disabled:    disabled,
		dialer:      dialer,
	}
}

// Start connects to Binance WebSocket and begins receiving data.
// It blocks until ctx is cancelled. Must be called in a goroutine.
// If BINANCE_WS_DISABLED=true, returns immediately.
func (c *BinanceWSClient) Start(ctx context.Context) {
	if c.disabled {
		return // HTTP fallback handles everything
	}

	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	defer cancel()

	delay := wsReconnectBaseDelay

	endpoints := []string{binancePrimaryWS, binanceFallbackWS1, binanceFallbackWS2}

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("[BinanceWS] Context cancelled, stopping")
			return
		default:
		}

		var err error
		wasConnected := false
		for _, ep := range endpoints {
			err = c.connectAndRead(ctx, ep)
			if ctx.Err() != nil {
				break
			}
			// If the client was connected at some point during this attempt,
			// it means dial succeeded but the stream later broke — not a DNS/firewall issue.
			if c.wasEverConnected.Load() {
				wasConnected = true
				c.wasEverConnected.Store(false)
				break // no need to try other endpoints; just reconnect to the same one
			}
			// Throttle logging: only log every endpoint failure on first cycle
			// or every 10th consecutive failure cycle thereafter.
			failures := c.consecutiveFailures.Load()
			if failures == 0 || failures%10 == 0 {
				c.logger.WithField("error", err).WithField("endpoint", ep).
					Warn("[BinanceWS] Connection failed, trying next endpoint")
			}
		}

		if ctx.Err() != nil {
			return
		}

		if wasConnected {
			// Was connected then lost — reconnect immediately with reset delay
			delay = wsReconnectBaseDelay
			c.consecutiveFailures.Store(0)
			c.logger.Info("[BinanceWS] Connection lost, reconnecting immediately")
			continue
		}

		c.consecutiveFailures.Add(1)
		failures := c.consecutiveFailures.Load()

		// Only log every failure on first few attempts, then throttle
		if failures <= 3 || failures%10 == 0 {
			c.logger.WithField("error", err).WithField("delay", delay).WithField("failures", failures).
				Warn("[BinanceWS] All endpoints failed, reconnecting after delay")
		}

		// Exponential backoff
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = time.Duration(math.Min(float64(delay)*2, float64(wsReconnectMaxDelay)))
	}
}

// Stop gracefully closes the WebSocket connection.
func (c *BinanceWSClient) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.connMu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.connMu.Unlock()
	c.connected.Store(false)
}

// IsConnected returns true if the WebSocket is currently connected.
func (c *BinanceWSClient) IsConnected() bool {
	return c.connected.Load()
}

// IsDisabled returns true if the WebSocket is disabled via environment variable.
func (c *BinanceWSClient) IsDisabled() bool {
	return c.disabled
}

// GetLastTickers returns a snapshot of the latest ticker data.
// Safe to call from any goroutine.
func (c *BinanceWSClient) GetLastTickers() map[string]BinanceTicker24hr {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]BinanceTicker24hr, len(c.lastTickers))
	for k, v := range c.lastTickers {
		result[k] = v
	}
	return result
}

// buildStreamURL constructs the combined stream URL.
// e.g. wss://stream.binance.com:9443/stream?streams=btcusdt@ticker/ethusdt@ticker
func (c *BinanceWSClient) buildStreamURL(baseURL string) string {
	var streams []string
	for _, sym := range c.symbols {
		streams = append(streams, sym+"@ticker")
	}
	u, _ := url.Parse(baseURL)
	u.Path = "/stream"
	q := u.Query()
	q.Set("streams", strings.Join(streams, "/"))
	u.RawQuery = q.Encode()
	return u.String()
}

// connectAndRead establishes a WebSocket connection and reads messages until
// an error occurs or context is cancelled.
func (c *BinanceWSClient) connectAndRead(ctx context.Context, baseURL string) error {
	streamURL := c.buildStreamURL(baseURL)
	c.logger.WithField("url", streamURL).Info("[BinanceWS] Connecting...")

	conn, _, err := c.dialer.DialContext(ctx, streamURL, nil)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()
	c.connected.Store(true)
	c.wasEverConnected.Store(true)

	c.logger.WithField("url", baseURL).Info("[BinanceWS] Connected successfully")

	// Reset reconnect delay on successful connection
	defer func() {
		c.connMu.Lock()
		c.conn = nil
		c.connMu.Unlock()
		c.connected.Store(false)
		conn.Close()
	}()

	// Set up pong handler
	conn.SetPongHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
		return nil
	})

	// Also handle Binance server pings
	conn.SetPingHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
	})

	// Start client-side ping sender
	pingDone := make(chan struct{})
	connDone := make(chan struct{}) // closed when connectAndRead is about to return
	go func() {
		defer close(pingDone)
		ticker := time.NewTicker(wsPingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-connDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.connMu.Lock()
				if c.conn != nil {
					c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
				}
				c.connMu.Unlock()
			}
		}
	}()
	defer func() {
		close(connDone)
		<-pingDone
	}()

	// Read loop
	conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read error: %w", err)
		}

		// Reset read deadline on every message
		conn.SetReadDeadline(time.Now().Add(wsPongTimeout))

		// Parse combined stream message
		var combined BinanceCombinedStreamMsg
		if err := json.Unmarshal(msgBytes, &combined); err != nil {
			c.logger.WithField("error", err).Debug("[BinanceWS] Failed to parse combined message")
			continue
		}

		// Parse ticker event
		var event BinanceWSTickerEvent
		if err := json.Unmarshal(combined.Data, &event); err != nil {
			c.logger.WithField("error", err).Debug("[BinanceWS] Failed to parse ticker event")
			continue
		}

		// Convert to BinanceTicker24hr (reuse existing struct)
		ticker := BinanceTicker24hr{
			Symbol:             event.Symbol,
			PriceChange:        event.PriceChange,
			PriceChangePercent: event.PriceChangePercent,
			WeightedAvgPrice:   event.WeightedAvgPrice,
			PrevClosePrice:     event.PrevClosePrice,
			LastPrice:          event.LastPrice,
			OpenPrice:          event.OpenPrice,
			HighPrice:          event.HighPrice,
			LowPrice:           event.LowPrice,
			Volume:             event.Volume,
			QuoteVolume:        event.QuoteVolume,
		}

		// Update in-memory cache
		c.mu.Lock()
		c.lastTickers[event.Symbol] = ticker
		c.mu.Unlock()

		// Invoke callback with all latest tickers
		if c.callback != nil {
			c.callback(c.GetLastTickers())
		}
	}
}
