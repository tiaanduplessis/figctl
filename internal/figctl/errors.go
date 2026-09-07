// Package figctl holds the error model shared by every layer of the CLI:
// stable error codes, the typed error, and the exit code mapping.
package figctl

import (
	"errors"
	"fmt"
)

// Code is a stable, machine readable error code an agent can switch on.
type Code string

// Error codes emitted in the error envelope.
const (
	CodeUsage        Code = "USAGE"
	CodeAuthMissing  Code = "AUTH_MISSING"
	CodeAuthInvalid  Code = "AUTH_INVALID"
	CodeAuthScope    Code = "AUTH_SCOPE"
	CodeNotFound     Code = "NOT_FOUND"
	CodeForbidden    Code = "FORBIDDEN"
	CodeRateLimited  Code = "RATE_LIMITED"
	CodePlanRequired Code = "PLAN_REQUIRED"
	CodeRenderFailed Code = "RENDER_FAILED"
	CodePartial      Code = "PARTIAL"
	CodeNetwork      Code = "NETWORK"
	CodeInternal     Code = "INTERNAL"
)

// Process exit codes.
const (
	ExitOK          = 0
	ExitError       = 1
	ExitUsage       = 2
	ExitAuth        = 3
	ExitNotFound    = 4
	ExitRateLimited = 5
	ExitPartial     = 6
)

// Error is the typed error every command returns. It carries everything the
// error envelope needs.
type Error struct {
	Code              Code
	Message           string
	Hint              string
	HTTPStatus        int
	RetryAfterSeconds int
	Details           map[string]any
	cause             error
}

// New creates an Error with the given code and message.
func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Newf creates an Error with a formatted message.
func Newf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap creates an Error that keeps the underlying cause for errors.Is and
// errors.As while presenting the given message.
func Wrap(code Code, cause error, message string) *Error {
	return &Error{Code: code, Message: message, cause: cause}
}

// WithHint sets the hint and returns the error for chaining.
func (e *Error) WithHint(format string, args ...any) *Error {
	e.Hint = fmt.Sprintf(format, args...)
	return e
}

// WithDetails sets the details map and returns the error for chaining.
func (e *Error) WithDetails(details map[string]any) *Error {
	e.Details = details
	return e
}

// Error implements the error interface.
func (e *Error) Error() string {
	return e.Message
}

// Unwrap exposes the cause, when one was recorded.
func (e *Error) Unwrap() error {
	return e.cause
}

// From converts any error into an *Error. Typed errors are returned as is,
// everything else becomes INTERNAL.
func From(err error) *Error {
	if err == nil {
		return nil
	}
	var typed *Error
	if errors.As(err, &typed) {
		return typed
	}
	return Wrap(CodeInternal, err, err.Error())
}

// ExitCode maps an error to the process exit code.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	switch From(err).Code {
	case CodeUsage:
		return ExitUsage
	case CodeAuthMissing, CodeAuthInvalid, CodeAuthScope:
		return ExitAuth
	case CodeNotFound:
		return ExitNotFound
	case CodeRateLimited:
		return ExitRateLimited
	case CodePartial:
		return ExitPartial
	default:
		return ExitError
	}
}
