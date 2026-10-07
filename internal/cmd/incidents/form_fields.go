package incidents

import (
	"fmt"
	"strings"

	"github.com/rootlyhq/rootly-cli/internal/api"
)

type fieldAssignment struct {
	field  api.FormField
	values []string
}

func resolveFormFieldSelections(fields []api.FormField, inputs []string) ([]map[string]interface{}, error) {
	assignments := make([]fieldAssignment, 0, len(inputs))
	assignmentIndexes := make(map[string]int, len(inputs))

	for _, input := range inputs {
		slug, value, ok := strings.Cut(input, "=")
		if !ok || slug == "" {
			return nil, fmt.Errorf("invalid --field value %q: expected slug=value", input)
		}

		field, found := findFormField(fields, slug)
		if !found {
			return nil, fmt.Errorf("unknown form field slug %q; run 'rootly form-fields list' to see available fields", slug)
		}

		if index, exists := assignmentIndexes[field.ID]; exists {
			assignments[index].values = append(assignments[index].values, value)
			continue
		}
		assignmentIndexes[field.ID] = len(assignments)
		assignments = append(assignments, fieldAssignment{field: field, values: []string{value}})
	}

	selections := make([]map[string]interface{}, 0, len(assignments))
	for _, assignment := range assignments {
		field := assignment.field
		if field.ValueKind != "inherit" {
			return nil, unsupportedFieldValueKind(field)
		}

		switch field.InputKind {
		case "text", "textarea", "rich_text", "number", "date", "datetime", "tags":
			if len(assignment.values) != 1 {
				return nil, fmt.Errorf("field %q accepts only one --field value", field.Slug)
			}
			selections = append(selections, map[string]interface{}{
				"form_field_id": field.ID,
				"value":         assignment.values[0],
			})
		case "select":
			if len(assignment.values) != 1 {
				return nil, fmt.Errorf("field %q is a single-select field and accepts only one --field value", field.Slug)
			}
			optionID, err := resolveFormFieldOption(field, assignment.values[0])
			if err != nil {
				return nil, err
			}
			selections = append(selections, map[string]interface{}{
				"form_field_id":       field.ID,
				"selected_option_ids": []string{optionID},
			})
		case "multi_select", "checkbox":
			optionIDs := make([]string, 0, len(assignment.values))
			seenOptionIDs := make(map[string]struct{}, len(assignment.values))
			for _, value := range assignment.values {
				optionID, err := resolveFormFieldOption(field, value)
				if err != nil {
					return nil, err
				}
				if _, seen := seenOptionIDs[optionID]; seen {
					continue
				}
				seenOptionIDs[optionID] = struct{}{}
				optionIDs = append(optionIDs, optionID)
			}
			selections = append(selections, map[string]interface{}{
				"form_field_id":       field.ID,
				"selected_option_ids": optionIDs,
			})
		default:
			return nil, unsupportedFieldValueKind(field)
		}
	}

	return selections, nil
}

func findFormField(fields []api.FormField, slug string) (api.FormField, bool) {
	for _, field := range fields {
		if field.Slug == slug {
			return field, true
		}
	}
	for _, field := range fields {
		if strings.EqualFold(field.Slug, slug) {
			return field, true
		}
	}
	return api.FormField{}, false
}

func resolveFormFieldOption(field api.FormField, value string) (string, error) {
	for _, option := range field.Options {
		if strings.EqualFold(option.Value, value) {
			return option.ID, nil
		}
	}

	validOptions := make([]string, 0, len(field.Options))
	for _, option := range field.Options {
		validOptions = append(validOptions, option.Value)
	}
	validValues := "(none)"
	if len(validOptions) > 0 {
		validValues = strings.Join(validOptions, ", ")
	}
	return "", fmt.Errorf("unknown option %q for field %q; valid options: %s", value, field.Slug, validValues)
}

func unsupportedFieldValueKind(field api.FormField) error {
	return fmt.Errorf("field %q (%s/%s) is not supported by --field yet", field.Slug, field.InputKind, field.ValueKind)
}
