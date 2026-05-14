package loop

type TokensUsed struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

type ToolCallSummary struct {
	MCP     string `json:"mcp"`
	Tool    string `json:"tool"`
	Outcome string `json:"outcome"`
}

type TerminateBody struct {
	RequestUUID  string            `json:"request_uuid"`
	PromptUUID   string            `json:"prompt_uuid"`
	Response     *string           `json:"response"`
	Iterations   int               `json:"iterations"`
	ToolsCalled  []ToolCallSummary `json:"tools_called"`
	Model        string            `json:"model"`
	TokensUsed   *TokensUsed       `json:"tokens_used,omitempty"`
	FinishReason FinishReason      `json:"finish_reason"`
	Error        *ErrorBlock       `json:"error,omitempty"`
}

type Envelope struct {
	Terminate TerminateBody `json:"terminate"`
}

type EnvelopeInput struct {
	RequestUUID  string
	PromptUUID   string
	Response     string
	HasResponse  bool
	Iterations   int
	Tools        []ToolCallSummary
	Model        string
	TokensUsed   TokensUsed
	FinishReason FinishReason
	Error        *ErrorBlock
}

func BuildEnvelope(in EnvelopeInput) Envelope {
	tools := in.Tools
	if tools == nil {
		tools = []ToolCallSummary{}
	}
	var responsePtr *string
	if in.HasResponse || in.Response != "" {
		v := in.Response
		responsePtr = &v
	}
	var tokensPtr *TokensUsed
	if in.TokensUsed != (TokensUsed{}) {
		t := in.TokensUsed
		tokensPtr = &t
	}
	return Envelope{
		Terminate: TerminateBody{
			RequestUUID:  in.RequestUUID,
			PromptUUID:   in.PromptUUID,
			Response:     responsePtr,
			Iterations:   in.Iterations,
			ToolsCalled:  tools,
			Model:        in.Model,
			TokensUsed:   tokensPtr,
			FinishReason: in.FinishReason,
			Error:        in.Error,
		},
	}
}
