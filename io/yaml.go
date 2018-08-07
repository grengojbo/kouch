package io

import (
	"io"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

func init() {
	registerOutputMode("yaml", &yamlMode{})
}

type yamlMode struct {
	defaultMode
}

var _ outputMode = &yamlMode{}

func (m *yamlMode) config(cmd *cobra.Command) {}

func (m *yamlMode) new(cmd *cobra.Command) (OutputProcessor, error) {
	return &yamlProcessor{}, nil
}

type yamlProcessor struct {
}

var _ OutputProcessor = &yamlProcessor{}

func (p *yamlProcessor) Output(o io.Writer, input io.ReadCloser) error {
	defer input.Close()
	unmarshaled, err := unmarshal(input)
	if err != nil {
		return err
	}
	enc := yaml.NewEncoder(o)
	return enc.Encode(unmarshaled)
}
