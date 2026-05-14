package loop

type FinishReason string

const (
	FinishTerminate        FinishReason = "terminate"
	FinishIterationCap     FinishReason = "iteration_cap"
	FinishWallclockTimeout FinishReason = "wallclock_timeout"
	FinishSchemaMismatch   FinishReason = "schema_mismatch"
	FinishLLMError         FinishReason = "llm_error"
	FinishInternalError    FinishReason = "internal_error"
)

func ExitCode(r FinishReason) int {
	if r == FinishTerminate {
		return 0
	}
	return 1
}

type ErrorBlock struct {
	Category             string   `json:"category"`
	MCP                  string   `json:"mcp,omitempty"`
	Tool                 string   `json:"tool,omitempty"`
	Message              string   `json:"message,omitempty"`
	ExpectedSchemaDigest string   `json:"expected_schema_digest,omitempty"`
	ValidationErrors     []string `json:"validation_errors,omitempty"`
}
