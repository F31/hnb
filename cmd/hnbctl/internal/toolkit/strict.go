package toolkit

import (
	"bytes"
	"io"

	"gopkg.in/yaml.v3"
)

// decodeStrict parses YAML with unknown-field rejection via the decoder's
// KnownFields option (gopkg.in/yaml.v3 v3.0.1 has no top-level strict helper).
func decodeStrict(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return err
	}
	// Ensure no trailing documents/fragments are silently ignored.
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return &yamlError{msg: "unexpected additional YAML document"}
	}
	return nil
}

type yamlError struct{ msg string }

func (e *yamlError) Error() string { return e.msg }
