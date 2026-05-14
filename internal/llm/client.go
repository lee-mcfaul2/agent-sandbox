package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

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

func (c *Client) Call(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.RetryBackoff):
			}
		}
		res, err := c.do(ctx, body)
		if err == nil {
			return res, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("litellm call failed after retry: %w", lastErr)
}

func (c *Client) do(ctx context.Context, body []byte) (*ChatCompletionResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.Traceparent != "" {
		httpReq.Header.Set("traceparent", c.Traceparent)
	}

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("litellm http %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("litellm rejected request: %d %s", resp.StatusCode, string(raw))
	}

	var out ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode litellm response: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("litellm returned no choices")
	}
	return &out, nil
}
