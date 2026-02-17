//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func flattenKeys(prefix string, data map[string]any, keys *[]string) {
	for k, v := range data {
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "." + k
		}
		switch val := v.(type) {
		case map[string]any:
			// Check if it's a leaf node (has "other" key)
			if _, hasOther := val["other"]; hasOther {
				*keys = append(*keys, fullKey)
			} else {
				flattenKeys(fullKey, val, keys)
			}
		}
	}
}

func loadKeys(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var nested map[string]any
	if err := yaml.Unmarshal(data, &nested); err != nil {
		return nil, err
	}
	var keys []string
	flattenKeys("", nested, &keys)
	sort.Strings(keys)
	return keys, nil
}

func main() {
	dir := "internal/i18n/locales"
	sourceFile := filepath.Join(dir, "en_US.yaml")

	sourceKeys, err := loadKeys(sourceFile)
	if err != nil {
		fmt.Printf("Error loading source: %v\n", err)
		os.Exit(1)
	}
	sourceSet := make(map[string]bool)
	for _, k := range sourceKeys {
		sourceSet[k] = true
	}

	files, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	hasErrors := false

	for _, f := range files {
		if strings.HasSuffix(f, "en_US.yaml") {
			continue
		}
		targetKeys, err := loadKeys(f)
		if err != nil {
			fmt.Printf("Error loading %s: %v\n", f, err)
			continue
		}
		targetSet := make(map[string]bool)
		for _, k := range targetKeys {
			targetSet[k] = true
		}

		// Find missing keys
		var missing []string
		for _, k := range sourceKeys {
			if !targetSet[k] {
				missing = append(missing, k)
			}
		}

		// Find extra keys
		var extra []string
		for _, k := range targetKeys {
			if !sourceSet[k] {
				extra = append(extra, k)
			}
		}

		if len(missing) > 0 || len(extra) > 0 {
			hasErrors = true
			fmt.Printf("\n%s:\n", filepath.Base(f))
			if len(missing) > 0 {
				fmt.Printf("  Missing (%d):\n", len(missing))
				for _, k := range missing {
					fmt.Printf("    - %s\n", k)
				}
			}
			if len(extra) > 0 {
				fmt.Printf("  Extra (%d):\n", len(extra))
				for _, k := range extra {
					fmt.Printf("    - %s\n", k)
				}
			}
		}
	}

	if !hasErrors {
		fmt.Println("All locale files are in sync with en_US.yaml")
	} else {
		os.Exit(1)
	}
}
