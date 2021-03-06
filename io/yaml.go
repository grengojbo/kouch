package io

import (
	"io"

	"github.com/spf13/pflag"
	"gopkg.in/yaml.v2"
)

func init() {
	registerOutputMode("yaml", &yamlMode{})
}

type yamlMode struct {
	defaultMode
}

var _ outputMode = &yamlMode{}

func (m *yamlMode) config(_ *pflag.FlagSet) {}

func (m *yamlMode) new(_ *pflag.FlagSet, w io.Writer) (io.WriteCloser, error) {
	return newProcessor(w, func(o io.Writer, i interface{}) error {
		return yaml.NewEncoder(o).Encode(i)
	}), nil
}
