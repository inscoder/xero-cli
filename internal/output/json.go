package output

import (
	"encoding/json"
	"io"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
)

type Breadcrumb struct {
	Action string `json:"action"`
	Cmd    string `json:"cmd"`
}

type Envelope struct {
	OK          bool         `json:"ok"`
	Data        any          `json:"data"`
	Summary     string       `json:"summary,omitempty"`
	Breadcrumbs []Breadcrumb `json:"breadcrumbs,omitempty"`
}

type ErrorDetail struct {
	Kind     string `json:"kind"`
	Message  string `json:"message"`
	ExitCode int    `json:"exitCode"`
}

type ErrorEnvelope struct {
	OK    bool        `json:"ok"`
	Error ErrorDetail `json:"error"`
}

func WriteJSON(writer io.Writer, data any, summary string, breadcrumbs []Breadcrumb, quiet bool) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if quiet {
		return encoder.Encode(data)
	}
	return encoder.Encode(Envelope{
		OK:          true,
		Data:        data,
		Summary:     summary,
		Breadcrumbs: breadcrumbs,
	})
}

func WriteErrorJSON(writer io.Writer, err error, quiet bool) error {
	detail := ErrorDetail{
		Kind:     string(clierrors.KindOf(err)),
		Message:  err.Error(),
		ExitCode: clierrors.ExitCode(err),
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if quiet {
		return encoder.Encode(detail)
	}

	return encoder.Encode(ErrorEnvelope{
		OK:    false,
		Error: detail,
	})
}
