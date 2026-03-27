package handler

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/songhanxu/wiseinvest/internal/infrastructure/logger"
)

// ──────────────────────────────────────────────────────────────────────────────
// WebSocket Message Protocol
// ──────────────────────────────────────────────────────────────────────────────

// WSMessage is the unified envelope for all WebSocket messages (both directions).
//
// Client → Server (subscribe/unsubscribe):
//
//	{"type":"subscribe","channel":"indices","params":{"market":"a_share"}}
//	{"type":"subscribe","channel":"quote","params":{"market":"crypto","code":"BTC"}}
//	{"type":"subscribe","channel":"kline","params":{"market":"a_share","code":"sh600519","period":"1d"}}
//	{"type":"unsubscribe","channel":"indices","params":{"market":"a_share"}}
//	{"type":"ping"}
//
// Server → Client (push):
//
//	{"type":"indices","market":"a_share","data":[...]}
//	{"type":"quote","market":"crypto","code":"BTC","data":{...}}
//	{"type":"kline","market":"a_share","code":"sh600519","period":"1d","data":[...]}
//	{"type":"pong"}
type WSMessage struct {
	Type    string          `json:"type"`
	Channel string          `json:"channel,omitempty"`
	Market  string          `json:"market,omitempty"`
	Code    string          `json:"code,omitempty"`
	Period  string          `json:"period,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Data    interface{}     `json:"data,omitempty"`
}

// WSSubscribeParams is parsed from WSMessage.Params for subscribe/unsubscribe.
type WSSubscribeParams struct {
	Market string `json:"market"`
	Code   string `json:"code,omitempty"`
	Period string `json:"period,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// Channel Key — unique identifier for a subscription channel
// ──────────────────────────────────────────────────────────────────────────────

// channelKey uniquely identifies a data stream.
// Examples: "indices:a_share", "quote:crypto:BTC", "kline:a_share:sh600519:1d"
type channelKey string

func indicesChannelKey(market string) channelKey {
	return channelKey("indices:" + market)
}

func quoteChannelKey(market, code string) channelKey {
	return channelKey("quote:" + market + ":" + code)
}

func klineChannelKey(market, code, period string) channelKey {
	return channelKey("kline:" + market + ":" + code + ":" + period)
}

// ──────────────────────────────────────────────────────────────────────────────
// WSClient — represents a single WebSocket connection
// ──────────────────────────────────────────────────────────────────────────────

type WSClient struct {
	hub  *WSHub
	conn *websocket.Conn
	send chan []byte // Buffered channel of outbound messages

	mu       sync.Mutex
	channels map[channelKey]bool // Channels this client is subscribed to
}

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second
	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = 30 * time.Second
	// Maximum message size allowed from peer
	maxMessageSize = 4096
	// Send buffer size
	sendBufSize = 256
)

// readPump pumps messages from the WebSocket connection to the hub.
func (c *WSClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, msgBytes, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "ping":
			// Respond with pong
			pong, _ := json.Marshal(WSMessage{Type: "pong"})
			select {
			case c.send <- pong:
			default:
			}

		case "subscribe":
			var params WSSubscribeParams
			if msg.Params != nil {
				json.Unmarshal(msg.Params, &params)
			}
			key := c.resolveChannelKey(msg.Channel, params)
			if key != "" {
				c.mu.Lock()
				c.channels[key] = true
				c.mu.Unlock()
				c.hub.subscribe <- subscription{client: c, channel: key}

				// Send last cached data for this channel immediately
				c.hub.sendLastData(c, key)
			}

		case "unsubscribe":
			var params WSSubscribeParams
			if msg.Params != nil {
				json.Unmarshal(msg.Params, &params)
			}
			key := c.resolveChannelKey(msg.Channel, params)
			if key != "" {
				c.mu.Lock()
				delete(c.channels, key)
				c.mu.Unlock()
				c.hub.unsubscribe <- subscription{client: c, channel: key}
			}
		}
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
func (c *WSClient) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// resolveChannelKey converts channel name + params to a channelKey.
func (c *WSClient) resolveChannelKey(channel string, params WSSubscribeParams) channelKey {
	switch channel {
	case "indices":
		if params.Market == "" {
			return ""
		}
		return indicesChannelKey(params.Market)
	case "quote":
		if params.Market == "" || params.Code == "" {
			return ""
		}
		return quoteChannelKey(params.Market, params.Code)
	case "kline":
		if params.Market == "" || params.Code == "" || params.Period == "" {
			return ""
		}
		return klineChannelKey(params.Market, params.Code, params.Period)
	default:
		return ""
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// subscription pairs a client with a channel
// ──────────────────────────────────────────────────────────────────────────────

type subscription struct {
	client  *WSClient
	channel channelKey
}

// ──────────────────────────────────────────────────────────────────────────────
// broadcast carries channel data + the serialized JSON payload
// ──────────────────────────────────────────────────────────────────────────────

type broadcast struct {
	channel channelKey
	data    []byte
}

// ──────────────────────────────────────────────────────────────────────────────
// WSHub — central coordinator for all WebSocket connections
// ──────────────────────────────────────────────────────────────────────────────

type WSHub struct {
	logger *logger.Logger

	// Registered clients
	clients map[*WSClient]bool

	// Channel → set of subscribed clients
	channels map[channelKey]map[*WSClient]bool

	// Last data for each channel (for instant delivery on subscribe)
	lastData   map[channelKey][]byte
	lastDataMu sync.RWMutex

	// Inbound channels
	register    chan *WSClient
	unregister  chan *WSClient
	subscribe   chan subscription
	unsubscribe chan subscription
	broadcastCh chan broadcast
}

// NewWSHub creates and starts a new WebSocket hub.
func NewWSHub(log *logger.Logger) *WSHub {
	hub := &WSHub{
		logger:      log,
		clients:     make(map[*WSClient]bool),
		channels:    make(map[channelKey]map[*WSClient]bool),
		lastData:    make(map[channelKey][]byte),
		register:    make(chan *WSClient, 64),
		unregister:  make(chan *WSClient, 64),
		subscribe:   make(chan subscription, 256),
		unsubscribe: make(chan subscription, 256),
		broadcastCh: make(chan broadcast, 512),
	}
	go hub.run()
	return hub
}

// run is the main event loop — processes all hub events sequentially (no locks needed).
func (h *WSHub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			h.logger.Infof("[WS] Client connected (%d total)", len(h.clients))

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				// Remove from all channel subscriptions
				for ch, subscribers := range h.channels {
					delete(subscribers, client)
					if len(subscribers) == 0 {
						delete(h.channels, ch)
					}
				}
				delete(h.clients, client)
				close(client.send)
				h.logger.Infof("[WS] Client disconnected (%d remaining)", len(h.clients))
			}

		case sub := <-h.subscribe:
			if h.channels[sub.channel] == nil {
				h.channels[sub.channel] = make(map[*WSClient]bool)
			}
			h.channels[sub.channel][sub.client] = true

		case sub := <-h.unsubscribe:
			if subscribers, ok := h.channels[sub.channel]; ok {
				delete(subscribers, sub.client)
				if len(subscribers) == 0 {
					delete(h.channels, sub.channel)
				}
			}

		case bc := <-h.broadcastCh:
			// Store as last data for new subscribers
			h.lastDataMu.Lock()
			h.lastData[bc.channel] = bc.data
			h.lastDataMu.Unlock()

			// Push to all subscribed clients
			subscribers := h.channels[bc.channel]
			for client := range subscribers {
				select {
				case client.send <- bc.data:
				default:
					// Client send buffer full — disconnect it
					close(client.send)
					delete(subscribers, client)
					delete(h.clients, client)
				}
			}
		}
	}
}

// Broadcast sends data to all clients subscribed to the given channel.
func (h *WSHub) Broadcast(channel channelKey, msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.broadcastCh <- broadcast{channel: channel, data: data}
}

// sendLastData sends the last cached payload for a channel to a newly subscribed client.
func (h *WSHub) sendLastData(client *WSClient, channel channelKey) {
	h.lastDataMu.RLock()
	data, ok := h.lastData[channel]
	h.lastDataMu.RUnlock()

	if ok && len(data) > 0 {
		select {
		case client.send <- data:
		default:
		}
	}
}

// ClientCount returns the number of currently connected clients.
func (h *WSHub) ClientCount() int {
	return len(h.clients)
}

// ChannelSubscriberCount returns the number of subscribers for a given channel.
func (h *WSHub) ChannelSubscriberCount(channel channelKey) int {
	if subs, ok := h.channels[channel]; ok {
		return len(subs)
	}
	return 0
}

// ──────────────────────────────────────────────────────────────────────────────
// HTTP Upgrade Handler — Gin compatible
// ──────────────────────────────────────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	// Allow all origins (iOS app connects directly)
	CheckOrigin: func(r *http.Request) bool { return true },
}

// HandleWS upgrades HTTP to WebSocket and registers the client with the hub.
func (h *WSHub) HandleWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.WithField("error", err).Warn("[WS] Upgrade failed")
		return
	}

	client := &WSClient{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, sendBufSize),
		channels: make(map[channelKey]bool),
	}

	h.register <- client

	go client.writePump()
	go client.readPump()
}
