package plan

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Metadata struct {
	Version   int      `yaml:"version"`
	ID        string   `yaml:"id"`
	Reviewers []string `yaml:"reviewers,omitempty"`
}

func (m Metadata) Body() (string, error) {
	data, err := yaml.Marshal(m)
	if err != nil {
		return "", err
	}
	return "Files under review. Approve to mark them ready.\n\n<!-- planctl\n" + string(data) + "-->\n", nil
}

func ParseMetadata(body string) (Metadata, error) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	const marker = "<!-- planctl\n"
	invalid := errors.New("review must contain one valid version-1 planctl metadata block")
	if strings.Count(body, marker) != 1 {
		return Metadata{}, invalid
	}
	_, rest, _ := strings.Cut(body, marker)
	data, _, found := strings.Cut(rest, "-->")
	if !found {
		return Metadata{}, invalid
	}
	decoder := yaml.NewDecoder(bytes.NewBufferString(data))
	decoder.KnownFields(true)
	var metadata Metadata
	if err := decoder.Decode(&metadata); err != nil {
		return Metadata{}, invalid
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Metadata{}, invalid
	}
	if metadata.Version != 1 || metadata.ID == "" {
		return Metadata{}, invalid
	}
	var fields struct {
		Version yaml.Node `yaml:"version"`
	}
	if err := yaml.Unmarshal([]byte(data), &fields); err != nil || fields.Version.Tag != "!!int" {
		return Metadata{}, invalid
	}
	return metadata, nil
}
