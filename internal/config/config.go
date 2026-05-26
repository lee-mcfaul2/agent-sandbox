package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type Config struct {
	RequestUUID         string
	PromptUUID          string
	LitellmURL          string
	GatewayMCPURL       string
	AvailableTools      []string
	TokenizedUserInput  string
	Model               string
	MaxIterations       int
	WallclockTimeoutSec int
	Traceparent         string
	OTLPEndpoint        string
	ServiceName         string
}

func Load() (*Config, error) {
	cfg := &Config{
		RequestUUID:        os.Getenv("REQUEST_UUID"),
		PromptUUID:         os.Getenv("PROMPT_UUID"),
		LitellmURL:         os.Getenv("LITELLM_URL"),
		GatewayMCPURL:      os.Getenv("GATEWAY_MCP_URL"),
		TokenizedUserInput: os.Getenv("TOKENIZED_USER_INPUT"),
		Model:              os.Getenv("MODEL"),
		Traceparent:        os.Getenv("TRACEPARENT"),
		OTLPEndpoint:       os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		ServiceName:        envOr(os.Getenv("OTEL_SERVICE_NAME"), "agent-sandbox"),
	}

	required := map[string]string{
		"REQUEST_UUID":         cfg.RequestUUID,
		"PROMPT_UUID":          cfg.PromptUUID,
		"LITELLM_URL":          cfg.LitellmURL,
		"GATEWAY_MCP_URL":      cfg.GatewayMCPURL,
		"TOKENIZED_USER_INPUT": cfg.TokenizedUserInput,
		"MODEL":                cfg.Model,
	}
	for k, v := range required {
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("missing required env var: %s", k)
		}
	}

	if _, err := uuid.Parse(cfg.RequestUUID); err != nil {
		return nil, fmt.Errorf("REQUEST_UUID is not a valid UUID: %w", err)
	}
	if _, err := uuid.Parse(cfg.PromptUUID); err != nil {
		return nil, fmt.Errorf("PROMPT_UUID is not a valid UUID: %w", err)
	}

	tools := os.Getenv("AVAILABLE_TOOLS")
	if strings.TrimSpace(tools) == "" {
		return nil, errors.New("missing required env var: AVAILABLE_TOOLS")
	}
	for _, t := range strings.Split(tools, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		cfg.AvailableTools = append(cfg.AvailableTools, t)
	}
	if len(cfg.AvailableTools) == 0 {
		return nil, errors.New("AVAILABLE_TOOLS parsed to zero tools")
	}

	maxIter, err := strconv.Atoi(os.Getenv("MAX_ITERATIONS"))
	if err != nil || maxIter < 1 {
		return nil, fmt.Errorf("MAX_ITERATIONS must be >=1, got %q", os.Getenv("MAX_ITERATIONS"))
	}
	cfg.MaxIterations = maxIter

	wallclock, err := strconv.Atoi(os.Getenv("WALLCLOCK_TIMEOUT_SECONDS"))
	if err != nil || wallclock < 1 {
		return nil, fmt.Errorf("WALLCLOCK_TIMEOUT_SECONDS must be >=1, got %q", os.Getenv("WALLCLOCK_TIMEOUT_SECONDS"))
	}
	cfg.WallclockTimeoutSec = wallclock

	return cfg, nil
}

func envOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
