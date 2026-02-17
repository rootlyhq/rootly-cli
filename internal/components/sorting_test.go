package components

import (
	"testing"
)

func TestNewSortState(t *testing.T) {
	s := NewSortState()

	if s.Field != nil {
		t.Errorf("expected Field to be nil, got %v", s.Field)
	}

	if s.Direction != SortDesc {
		t.Errorf("expected Direction to be SortDesc, got %v", s.Direction)
	}

	if s.enabled {
		t.Error("expected enabled to be false")
	}
}

func TestSortStateToggleNewField(t *testing.T) {
	s := NewSortState()

	// Toggle to a new field
	fieldChanged := s.Toggle("field1")

	if !fieldChanged {
		t.Error("expected fieldChanged to be true when setting a new field")
	}

	if s.Field != "field1" {
		t.Errorf("expected Field to be 'field1', got %v", s.Field)
	}

	if s.Direction != SortDesc {
		t.Errorf("expected Direction to be SortDesc, got %v", s.Direction)
	}

	if !s.enabled {
		t.Error("expected enabled to be true")
	}
}

func TestSortStateToggleSameFieldFlipsDirection(t *testing.T) {
	s := NewSortState()

	// Set initial field
	s.Toggle("field1")

	// Toggle same field should flip direction
	fieldChanged := s.Toggle("field1")

	if fieldChanged {
		t.Error("expected fieldChanged to be false when toggling same field")
	}

	if s.Direction != SortAsc {
		t.Errorf("expected Direction to be SortAsc after first toggle, got %v", s.Direction)
	}

	// Toggle again should flip back to Desc
	fieldChanged = s.Toggle("field1")

	if fieldChanged {
		t.Error("expected fieldChanged to be false when toggling same field")
	}

	if s.Direction != SortDesc {
		t.Errorf("expected Direction to be SortDesc after second toggle, got %v", s.Direction)
	}
}

func TestSortStateToggleDifferentField(t *testing.T) {
	s := NewSortState()

	// Set initial field
	s.Toggle("field1")
	s.Toggle("field1") // Flip to Asc

	// Toggle to a different field should reset to Desc
	fieldChanged := s.Toggle("field2")

	if !fieldChanged {
		t.Error("expected fieldChanged to be true when changing to different field")
	}

	if s.Field != "field2" {
		t.Errorf("expected Field to be 'field2', got %v", s.Field)
	}

	if s.Direction != SortDesc {
		t.Errorf("expected Direction to be SortDesc when changing field, got %v", s.Direction)
	}
}

func TestSortStateIsEnabled(t *testing.T) {
	s := NewSortState()

	if s.IsEnabled() {
		t.Error("expected IsEnabled to return false initially")
	}

	s.Toggle("field1")

	if !s.IsEnabled() {
		t.Error("expected IsEnabled to return true after toggling a field")
	}
}

func TestSortStateIsField(t *testing.T) {
	s := NewSortState()

	// Should return false when not enabled
	if s.IsField("field1") {
		t.Error("expected IsField to return false when not enabled")
	}

	s.Toggle("field1")

	// Should return true for the current field
	if !s.IsField("field1") {
		t.Error("expected IsField to return true for current field")
	}

	// Should return false for a different field
	if s.IsField("field2") {
		t.Error("expected IsField to return false for different field")
	}
}

func TestSortStateGetIndicator(t *testing.T) {
	s := NewSortState()

	// Should return empty string when not enabled
	if s.GetIndicator() != "" {
		t.Errorf("expected empty indicator when not enabled, got '%s'", s.GetIndicator())
	}

	// Set to descending
	s.Toggle("field1")
	if s.GetIndicator() != "↓" {
		t.Errorf("expected '↓' for descending, got '%s'", s.GetIndicator())
	}

	// Flip to ascending
	s.Toggle("field1")
	if s.GetIndicator() != "↑" {
		t.Errorf("expected '↑' for ascending, got '%s'", s.GetIndicator())
	}
}

func TestSortStateGetInfo(t *testing.T) {
	s := NewSortState()

	// Should return empty string when not enabled
	if s.GetInfo("TestField") != "" {
		t.Errorf("expected empty info when not enabled, got '%s'", s.GetInfo("TestField"))
	}

	// Set to descending
	s.Toggle("field1")
	expected := "↓ TestField"
	if s.GetInfo("TestField") != expected {
		t.Errorf("expected '%s', got '%s'", expected, s.GetInfo("TestField"))
	}

	// Flip to ascending
	s.Toggle("field1")
	expected = "↑ TestField"
	if s.GetInfo("TestField") != expected {
		t.Errorf("expected '%s', got '%s'", expected, s.GetInfo("TestField"))
	}
}

func TestSortStateApplyDirection(t *testing.T) {
	s := NewSortState()
	s.Toggle("field1")

	tests := []struct {
		name      string
		direction SortDirection
		less      bool
		expected  bool
	}{
		{"desc_true_returns_false", SortDesc, true, false},
		{"desc_false_returns_true", SortDesc, false, true},
		{"asc_true_returns_true", SortAsc, true, true},
		{"asc_false_returns_false", SortAsc, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s.Direction = tt.direction
			result := s.ApplyDirection(tt.less)
			if result != tt.expected {
				t.Errorf("ApplyDirection(%v) with direction %v = %v, want %v",
					tt.less, tt.direction, result, tt.expected)
			}
		})
	}
}

func TestSortStateWithIntValues(t *testing.T) {
	s := NewSortState()

	// Test with integer field values
	s.Toggle(1)
	if s.Field != 1 {
		t.Errorf("expected Field to be 1, got %v", s.Field)
	}

	if !s.IsField(1) {
		t.Error("expected IsField(1) to return true")
	}

	if s.IsField(2) {
		t.Error("expected IsField(2) to return false")
	}
}

func TestSortStateMultipleFieldChanges(t *testing.T) {
	s := NewSortState()

	// Change through multiple fields
	fields := []interface{}{"field1", "field2", "field3"}

	for _, field := range fields {
		s.Toggle(field)
		if s.Field != field {
			t.Errorf("expected Field to be %v, got %v", field, s.Field)
		}
		if s.Direction != SortDesc {
			t.Errorf("expected Direction to be SortDesc after field change, got %v", s.Direction)
		}
	}
}
