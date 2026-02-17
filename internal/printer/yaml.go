package printer

import (
	"encoding/json"
	"io"

	"gopkg.in/yaml.v3"
)

// YAMLPrinter renders output as YAML.
type YAMLPrinter struct{}

// NewYAMLPrinter creates a new YAMLPrinter.
func NewYAMLPrinter() *YAMLPrinter {
	return &YAMLPrinter{}
}

// PrintObj prints a single object as YAML.
func (p *YAMLPrinter) PrintObj(obj interface{}, w io.Writer) error {
	data, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// PrintRawJSON converts a raw JSON API response body to YAML output.
func (p *YAMLPrinter) PrintRawJSON(rawBody []byte, w io.Writer) error {
	var obj interface{}
	if err := json.Unmarshal(rawBody, &obj); err != nil {
		return err
	}
	data, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// PrintList converts headers and rows into a YAML array of objects,
// where each object uses headers as keys.
func (p *YAMLPrinter) PrintList(headers []string, rows [][]string, w io.Writer) error {
	if len(rows) == 0 {
		_, err := w.Write([]byte("[]\n"))
		return err
	}

	result := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		obj := make(map[string]string, len(headers))
		for i, h := range headers {
			if i < len(row) {
				obj[h] = row[i]
			}
		}
		result = append(result, obj)
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
