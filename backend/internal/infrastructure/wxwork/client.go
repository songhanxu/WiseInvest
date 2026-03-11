package wxwork

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a WeChat Work (企业微信) robot webhook client.
type Client struct {
	webhookURL string
	httpClient *http.Client
}

// NewClient creates a new WeChat Work webhook client.
// webhookURL is the full webhook URL including the key parameter.
func NewClient(webhookURL string) *Client {
	return &Client{
		webhookURL: webhookURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type markdownMessage struct {
	MsgType  string           `json:"msgtype"`
	Markdown markdownContent  `json:"markdown"`
}

type markdownContent struct {
	Content string `json:"content"`
}

type wxResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// SendMarkdown sends a Markdown-formatted message to the WeChat Work robot.
// content supports WeChat Work Markdown syntax (subset of CommonMark).
func (c *Client) SendMarkdown(content string) error {
	if c.webhookURL == "" {
		return fmt.Errorf("wxwork: webhook URL is not configured")
	}

	msg := markdownMessage{
		MsgType:  "markdown",
		Markdown: markdownContent{Content: content},
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("wxwork: failed to marshal message: %w", err)
	}

	resp, err := c.httpClient.Post(c.webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("wxwork: HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("wxwork: failed to read response: %w", err)
	}

	var wxResp wxResponse
	if err := json.Unmarshal(body, &wxResp); err != nil {
		return fmt.Errorf("wxwork: failed to parse response: %w", err)
	}

	if wxResp.ErrCode != 0 {
		return fmt.Errorf("wxwork: API error %d: %s", wxResp.ErrCode, wxResp.ErrMsg)
	}

	return nil
}

// IsConfigured returns true if the webhook URL has been set.
func (c *Client) IsConfigured() bool {
	return c.webhookURL != ""
}
