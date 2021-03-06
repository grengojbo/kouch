package io

import (
	"encoding/json"
	"io"

	"github.com/spf13/pflag"
)

func init() {
	registerOutputMode("json", &jsonMode{defaultMode: true})
}

const (
	optJSONPrefix     = "json-prefix"
	optJSONIndent     = "json-indent"
	optJSONEscapeHTML = "json-escape-html"
)

type jsonMode struct {
	defaultMode
}

var _ outputMode = &jsonMode{}

func (m *jsonMode) config(flags *pflag.FlagSet) {
	flags.String(optJSONPrefix, "", "Prefix to begin each line of the JSON output. See [https://golang.org/pkg/encoding/json/#Indent] for more information.")
	flags.String(optJSONIndent, "", "Indentation string for JSON output. See [https://golang.org/pkg/encoding/json/#Indent] for more information.")
	flags.Bool(optJSONEscapeHTML, false, "Enable escaping of special HTML characters. See [https://golang.org/pkg/encoding/json/#Encoder.SetEscapeHTML].")
}

func (m *jsonMode) new(flags *pflag.FlagSet, w io.Writer) (io.WriteCloser, error) {
	prefix, err := flags.GetString(optJSONPrefix)
	if err != nil {
		return nil, err
	}
	indent, err := flags.GetString(optJSONIndent)
	if err != nil {
		return nil, err
	}
	escapeHTML, err := flags.GetBool(optJSONEscapeHTML)
	if err != nil {
		return nil, err
	}
	return newProcessor(w, func(o io.Writer, i interface{}) error {
		enc := json.NewEncoder(o)
		enc.SetIndent(prefix, indent)
		enc.SetEscapeHTML(escapeHTML)
		return enc.Encode(i)
	}), nil
}
