package output

import (
	"encoding/json"
	"fmt"
	"io"
)

const SchemaVersion = "v1"

type CLIError struct {
	ExitCode int            `json:"-"`
	Code     string         `json:"code"`
	Message  string         `json:"message"`
	Details  map[string]any `json:"details,omitempty"`
}

func (e *CLIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

func NewError(exitCode int, code, message string, details map[string]any) *CLIError {
	return &CLIError{ExitCode: exitCode, Code: code, Message: message, Details: details}
}

func SuccessEnvelope(payload map[string]any) map[string]any {
	if payload == nil {
		payload = map[string]any{}
	}
	if _, ok := payload["schemaVersion"]; !ok {
		payload["schemaVersion"] = SchemaVersion
	}
	return payload
}

func ErrorEnvelope(err *CLIError) map[string]any {
	return map[string]any{
		"schemaVersion": SchemaVersion,
		"error": map[string]any{
			"code":    err.Code,
			"message": err.Message,
			"details": err.Details,
		},
	}
}

func WriteJSON(w io.Writer, payload any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(payload)
}

func WriteTextError(w io.Writer, err error) {
	_, _ = fmt.Fprintln(w, err.Error())
}
