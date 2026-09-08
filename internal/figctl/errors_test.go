package figctl

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, ExitOK},
		{"usage", New(CodeUsage, "x"), ExitUsage},
		{"auth missing", New(CodeAuthMissing, "x"), ExitAuth},
		{"auth invalid", New(CodeAuthInvalid, "x"), ExitAuth},
		{"auth scope", New(CodeAuthScope, "x"), ExitAuth},
		{"not found", New(CodeNotFound, "x"), ExitNotFound},
		{"forbidden", New(CodeForbidden, "x"), ExitError},
		{"rate limited", New(CodeRateLimited, "x"), ExitRateLimited},
		{"plan required", New(CodePlanRequired, "x"), ExitError},
		{"render failed", New(CodeRenderFailed, "x"), ExitError},
		{"image mismatch", New(CodeImageMismatch, "x"), ExitImageMismatch},
		{"image IO", New(CodeImageIO, "x"), ExitError},
		{"partial", New(CodePartial, "x"), ExitPartial},
		{"network", New(CodeNetwork, "x"), ExitError},
		{"internal", New(CodeInternal, "x"), ExitError},
		{"untyped", errors.New("boom"), ExitError},
		{"wrapped typed", fmt.Errorf("ctx: %w", New(CodeNotFound, "x")), ExitNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCode(tc.err); got != tc.want {
				t.Fatalf("ExitCode = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestFrom(t *testing.T) {
	cause := errors.New("boom")
	e := From(cause)
	if e.Code != CodeInternal || e.Message != "boom" {
		t.Fatalf("unexpected conversion: %+v", e)
	}
	if !errors.Is(e, cause) {
		t.Fatal("cause not preserved")
	}
	typed := New(CodeUsage, "bad").WithHint("try %s", "--help")
	if From(fmt.Errorf("wrap: %w", typed)) != typed {
		t.Fatal("typed error not returned as is")
	}
	if typed.Hint != "try --help" {
		t.Fatalf("hint = %q", typed.Hint)
	}
}
