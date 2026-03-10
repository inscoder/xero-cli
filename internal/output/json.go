package output

import (
	"encoding/json"
	"io"
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
