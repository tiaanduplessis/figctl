package output

import (
	"bytes"
	"encoding/json"

	"github.com/tiaanduplessis/figctl/internal/figctl"
)

// Project keeps only the named top level keys of data. When data is an
// array, each object element is projected. Non-object values are returned
// unchanged.
func Project(data any, fields []string) (any, error) {
	if len(fields) == 0 {
		return data, nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "projecting output: "+err.Error())
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return nil, figctl.Wrap(figctl.CodeInternal, err, "projecting output: "+err.Error())
	}
	switch v := generic.(type) {
	case map[string]any:
		return pick(v, fields), nil
	case []any:
		for i, el := range v {
			if m, ok := el.(map[string]any); ok {
				v[i] = pick(m, fields)
			}
		}
		return v, nil
	default:
		return generic, nil
	}
}

func pick(m map[string]any, fields []string) map[string]any {
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		if v, ok := m[f]; ok {
			out[f] = v
		}
	}
	return out
}
