package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type ToolResult struct {
	OK   bool            `json:"ok"`
	Data json.RawMessage `json:"data,omitempty"`
	// DataPlaintext is the pre-scrub copy the gateway ships so the sandbox can
	// re-validate against the bundle's plaintext-shape response schema. It is
	// sandbox-internal only — the LLM-facing serialization in driver.go MUST
	// clear it before building the tool message (or the LLM would see plaintext
	// PII, defeating the tokenization barrier).
	DataPlaintext json.RawMessage `json:"data_plaintext,omitempty"`
	Error         string          `json:"error,omitempty"`
	Reason        string          `json:"reason,omitempty"`
	MCP           string          `json:"mcp"`
	Tool          string          `json:"tool"`
	RequestUUID   string          `json:"request_uuid"`
}

type Client struct {
	BaseURL      string
	HTTP         *http.Client
	Traceparent  string
	RetryBackoff time.Duration
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		BaseURL:      baseURL,
		HTTP:         &http.Client{Timeout: timeout},
		RetryBackoff: 1 * time.Second,
	}
}

func (c *Client) CallTool(ctx context.Context, requestUUID, mcp, tool string, args json.RawMessage) (*ToolResult, error) {
	body, err := json.Marshal(map[string]any{
		"request_uuid": requestUUID,
		"args":         args,
	})
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/v1/mcp/%s/%s", mcp, tool)

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.RetryBackoff):
			}
		}
		env, err := c.doTool(ctx, path, body)
		if err == nil {
			return env, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("gateway tool call failed after retry: %w", lastErr)
}

func (c *Client) doTool(ctx context.Context, path string, body []byte) (*ToolResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Traceparent != "" {
		req.Header.Set("traceparent", c.Traceparent)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("gateway http %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gateway rejected request: %d %s", resp.StatusCode, string(raw))
	}
	var wrap struct {
		ToolResult ToolResult `json:"tool_result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrap); err != nil {
		return nil, fmt.Errorf("decode gateway envelope: %w", err)
	}
	return &wrap.ToolResult, nil
}
