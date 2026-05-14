package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/lee-mcfaul2/agent-sandbox/internal/config"
	"github.com/lee-mcfaul2/agent-sandbox/internal/gateway"
	"github.com/lee-mcfaul2/agent-sandbox/internal/llm"
	"github.com/lee-mcfaul2/agent-sandbox/internal/loop"
	"github.com/lee-mcfaul2/agent-sandbox/internal/obs"
	"github.com/lee-mcfaul2/agent-sandbox/internal/schemas"
	"github.com/lee-mcfaul2/agent-sandbox/internal/tools"
)

//go:embed system.txt
var systemPrompt string

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--print-bundle-digest" {
		b, err := schemas.LoadEmbedded()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(b.Digest)
		return
	}

	exitCode := run()
	os.Exit(exitCode)
}

func run() int {
	cfg, err := config.Load()
	if err != nil {
		emitMinimalFailure("", "", loop.FinishInternalError, &loop.ErrorBlock{
			Category: "CONFIG_INVALID",
			Message:  err.Error(),
		})
		return 1
	}

	logger := obs.New(os.Stderr, cfg.RequestUUID)
	logger.Info("startup", map[string]any{"prompt_uuid": cfg.PromptUUID})

	bundle, err := schemas.LoadEmbedded()
	if err != nil {
		emitMinimalFailure(cfg.RequestUUID, cfg.PromptUUID, loop.FinishInternalError, &loop.ErrorBlock{
			Category: "BUNDLE_LOAD_FAILED",
			Message:  err.Error(),
		})
		return 1
	}

	gwClient := gateway.New(cfg.GatewayMCPURL, 10*time.Second)
	gwClient.Traceparent = cfg.Traceparent
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := gwClient.VerifyDigest(ctx, bundle.Digest); err != nil {
		emitMinimalFailure(cfg.RequestUUID, cfg.PromptUUID, loop.FinishInternalError, &loop.ErrorBlock{
			Category: "BUNDLE_DIGEST_MISMATCH",
			Message:  err.Error(),
		})
		return 1
	}

	cat, err := tools.BuildCatalog(bundle, cfg.AvailableTools)
	if err != nil {
		emitMinimalFailure(cfg.RequestUUID, cfg.PromptUUID, loop.FinishInternalError, &loop.ErrorBlock{
			Category: "CATALOG_BUILD_FAILED",
			Message:  err.Error(),
		})
		return 1
	}
	reg, err := schemas.CompileValidators(bundle, cat.Refs)
	if err != nil {
		emitMinimalFailure(cfg.RequestUUID, cfg.PromptUUID, loop.FinishInternalError, &loop.ErrorBlock{
			Category: "VALIDATOR_COMPILE_FAILED",
			Message:  err.Error(),
		})
		return 1
	}

	llmClient := llm.New(cfg.LitellmURL, 60*time.Second)
	llmClient.Traceparent = cfg.Traceparent

	driver := &loop.Driver{
		Config: loop.Config{
			RequestUUID:      cfg.RequestUUID,
			PromptUUID:       cfg.PromptUUID,
			Model:            cfg.Model,
			SystemPrompt:     systemPrompt,
			UserInput:        cfg.TokenizedUserInput,
			MaxIterations:    cfg.MaxIterations,
			WallclockTimeout: time.Duration(cfg.WallclockTimeoutSec) * time.Second,
		},
		LLM:        llmClient,
		Gateway:    gwClient,
		Catalog:    cat,
		Validators: reg,
		Logger:     logger,
	}

	env, runErr := driver.Run(context.Background())
	if runErr != nil {
		emitMinimalFailure(cfg.RequestUUID, cfg.PromptUUID, loop.FinishInternalError, &loop.ErrorBlock{
			Category: "DRIVER_PANIC",
			Message:  runErr.Error(),
		})
		return 1
	}

	if err := emitEnvelope(env); err != nil {
		emitMinimalFailure(cfg.RequestUUID, cfg.PromptUUID, loop.FinishInternalError, &loop.ErrorBlock{
			Category: "EMIT_FAILED",
			Message:  err.Error(),
		})
		return 1
	}
	return loop.ExitCode(env.Terminate.FinishReason)
}

func emitEnvelope(env loop.Envelope) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(env)
}

func emitMinimalFailure(reqUUID, promptUUID string, reason loop.FinishReason, errBlock *loop.ErrorBlock) {
	env := loop.BuildEnvelope(loop.EnvelopeInput{
		RequestUUID:  reqUUID,
		PromptUUID:   promptUUID,
		FinishReason: reason,
		Error:        errBlock,
	})
	_ = emitEnvelope(env)
}
