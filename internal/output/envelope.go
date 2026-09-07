package output

import (
	"github.com/tiaanduplessis/figctl/internal/figctl"
)

// SchemaVersion is the current output contract version. Only additive
// changes are allowed within a version.
const SchemaVersion = 1

// ProfileInfo identifies the account a command ran as.
type ProfileInfo struct {
	Name   string `json:"name"`
	Handle string `json:"handle,omitempty"`
}

// FileInfo identifies the Figma file a command operated on.
type FileInfo struct {
	Key          string `json:"key"`
	Name         string `json:"name,omitempty"`
	Version      string `json:"version,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
}

// Envelope is the success envelope every command emits.
type Envelope struct {
	SchemaVersion int          `json:"schemaVersion"`
	Command       string       `json:"command"`
	Profile       *ProfileInfo `json:"profile"`
	File          *FileInfo    `json:"file"`
	Data          any          `json:"data"`
	Truncated     bool         `json:"truncated"`
	NextCursor    *string      `json:"nextCursor"`
	Hints         []string     `json:"hints"`
}

// ErrorEnvelope is the error envelope, always on stdout in JSON mode.
type ErrorEnvelope struct {
	SchemaVersion int       `json:"schemaVersion"`
	Error         ErrorBody `json:"error"`
}

// ErrorBody is the error block inside ErrorEnvelope.
type ErrorBody struct {
	Code              figctl.Code    `json:"code"`
	Message           string         `json:"message"`
	Hint              string         `json:"hint,omitempty"`
	RetryAfterSeconds int            `json:"retryAfterSeconds,omitempty"`
	HTTPStatus        int            `json:"httpStatus,omitempty"`
	Details           map[string]any `json:"details,omitempty"`
}

// NewEnvelope creates a success envelope for the given command and data.
func NewEnvelope(command string, data any) *Envelope {
	return &Envelope{
		SchemaVersion: SchemaVersion,
		Command:       command,
		Data:          data,
		Hints:         []string{},
	}
}

// AddHint appends a hint and returns the envelope for chaining.
func (e *Envelope) AddHint(hint string) *Envelope {
	e.Hints = append(e.Hints, hint)
	return e
}

// NewErrorEnvelope converts any error into the error envelope.
func NewErrorEnvelope(err error) *ErrorEnvelope {
	e := figctl.From(err)
	return &ErrorEnvelope{
		SchemaVersion: SchemaVersion,
		Error: ErrorBody{
			Code:              e.Code,
			Message:           e.Message,
			Hint:              e.Hint,
			RetryAfterSeconds: e.RetryAfterSeconds,
			HTTPStatus:        e.HTTPStatus,
			Details:           e.Details,
		},
	}
}
