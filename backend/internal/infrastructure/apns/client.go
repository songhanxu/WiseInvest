package apns

import (
	"fmt"

	apns2 "github.com/sideshow/apns2"
	"github.com/sideshow/apns2/payload"
	"github.com/sideshow/apns2/token"
)

// Client wraps the APNs HTTP/2 client for sending push notifications.
type Client struct {
	client   *apns2.Client
	bundleID string
}

// Config holds APNs configuration.
type Config struct {
	KeyID      string
	TeamID     string
	BundleID   string
	KeyFile    string // path to .p8 private key file
	Production bool
}

// NewClient creates a new APNs client using token-based (.p8) authentication.
// Returns (nil, nil) when KeyID is empty, allowing callers to skip APNs gracefully.
func NewClient(cfg Config) (*Client, error) {
	if cfg.KeyID == "" {
		return nil, nil
	}

	authKey, err := token.AuthKeyFromFile(cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("apns: failed to load auth key from %s: %w", cfg.KeyFile, err)
	}

	t := &token.Token{
		AuthKey: authKey,
		KeyID:   cfg.KeyID,
		TeamID:  cfg.TeamID,
	}

	var c *apns2.Client
	if cfg.Production {
		c = apns2.NewTokenClient(t).Production()
	} else {
		c = apns2.NewTokenClient(t).Development()
	}

	return &Client{client: c, bundleID: cfg.BundleID}, nil
}

// SendAlert sends an alert push notification to a single device token.
func (c *Client) SendAlert(deviceToken, title, body string) error {
	p := payload.NewPayload().
		AlertTitle(title).
		AlertBody(body).
		Sound("default")

	notification := &apns2.Notification{
		DeviceToken: deviceToken,
		Topic:       c.bundleID,
		Payload:     p,
	}

	res, err := c.client.Push(notification)
	if err != nil {
		return fmt.Errorf("apns: push failed: %w", err)
	}

	if !res.Sent() {
		return fmt.Errorf("apns: notification rejected: %s (reason: %s)", res.ApnsID, res.Reason)
	}

	return nil
}

// IsConfigured returns true when the client is ready to send.
func (c *Client) IsConfigured() bool {
	return c != nil && c.client != nil
}
