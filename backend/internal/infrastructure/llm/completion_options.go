package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type thinkingModeKey struct{}

// The pinned SDK lacks DeepSeek's thinking field. Extend only requests that
// explicitly opt in; leave chat, streaming, and tool requests untouched.
type completionOptionsTransport struct {
	base http.RoundTripper
}

func (t completionOptionsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	mode, _ := req.Context().Value(thinkingModeKey{}).(string)
	if mode != "enabled" && mode != "disabled" {
		return t.base.RoundTrip(req)
	}
	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return nil, err
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("encode completion options: %w", err)
	}
	payload["thinking"], _ = json.Marshal(map[string]string{"type": mode})
	body, err = json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	cloned := req.Clone(req.Context())
	cloned.Body = io.NopCloser(bytes.NewReader(body))
	cloned.ContentLength = int64(len(body))
	cloned.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	return t.base.RoundTrip(cloned)
}

func (c *OpenAIClient) SupportsThinkingControl() bool {
	model := strings.ToLower(c.model)
	return c.deepSeekEndpoint && (strings.HasPrefix(model, "deepseek-v4-") || model == "deepseek-flash")
}
