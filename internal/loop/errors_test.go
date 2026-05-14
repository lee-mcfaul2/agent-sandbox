package loop

import "testing"

func TestFinishReasonStrings(t *testing.T) {
	cases := map[FinishReason]string{
		FinishTerminate:        "terminate",
		FinishIterationCap:     "iteration_cap",
		FinishWallclockTimeout: "wallclock_timeout",
		FinishSchemaMismatch:   "schema_mismatch",
		FinishLLMError:         "llm_error",
		FinishInternalError:    "internal_error",
	}
	for r, want := range cases {
		if string(r) != want {
			t.Errorf("FinishReason(%v) = %q, want %q", r, string(r), want)
		}
	}
}

func TestExitCode(t *testing.T) {
	if ExitCode(FinishTerminate) != 0 {
		t.Errorf("terminate exit = %d", ExitCode(FinishTerminate))
	}
	for _, r := range []FinishReason{FinishIterationCap, FinishWallclockTimeout, FinishSchemaMismatch, FinishLLMError, FinishInternalError} {
		if ExitCode(r) != 1 {
			t.Errorf("%s exit = %d", r, ExitCode(r))
		}
	}
}
