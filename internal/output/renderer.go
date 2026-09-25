// Package output renders typed command results for humans and automation.
package output

import (
	"encoding/json"
	"fmt"
	"io"
)

const Version = 1

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorResult struct {
	Version int         `json:"version"`
	Error   ErrorDetail `json:"error"`
}

type VersionResult struct {
	Version int    `json:"version"`
	Build   string `json:"build"`
}

type Renderer struct {
	Stdout io.Writer
	Stderr io.Writer
	JSON   bool
}

func (r Renderer) Error(result ErrorResult) error {
	if r.JSON {
		return writeJSON(r.Stdout, result)
	}
	_, err := fmt.Fprintf(r.Stderr, "error: %s\n", result.Error.Message)
	return err
}

func (r Renderer) Version(result VersionResult) error {
	if r.JSON {
		return writeJSON(r.Stdout, result)
	}
	_, err := fmt.Fprintf(r.Stdout, "planctl %s\n", result.Build)
	return err
}

func writeJSON(w io.Writer, result any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(result)
}
