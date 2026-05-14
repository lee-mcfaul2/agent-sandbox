package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

func (c *Client) VerifyDigest(ctx context.Context, expected string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/bundle_digest", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("bundle_digest fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bundle_digest http %d", resp.StatusCode)
	}
	var body struct {
		Digest string `json:"digest"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("decode bundle_digest: %w", err)
	}
	if body.Digest != expected {
		return fmt.Errorf("bundle digest mismatch: gateway=%q sandbox=%q", body.Digest, expected)
	}
	return nil
}
