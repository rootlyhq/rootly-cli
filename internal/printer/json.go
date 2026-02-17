package printer

import (
	"encoding/json"
	"io"
)

// JSONPrinter renders output as indented JSON.
type JSONPrinter struct{}

// NewJSONPrinter creates a new JSONPrinter.
func NewJSONPrinter() *JSONPrinter {
	return &JSONPrinter{}
}

// PrintObj prints a single object as indented JSON.
func (p *JSONPrinter) PrintObj(obj interface{}, w io.Writer) error {
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

// PrintRawJSON pretty-prints a raw JSON API response body.
func (p *JSONPrinter) PrintRawJSON(rawBody []byte, w io.Writer) error {
	var obj interface{}
	if err := json.Unmarshal(rawBody, &obj); err != nil {
		return err
	}
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

// PrintList converts headers and rows into a JSON array of objects,
// where each object uses headers as keys.
func (p *JSONPrinter) PrintList(headers []string, rows [][]string, w io.Writer) error {
	if rows == nil {
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

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}
