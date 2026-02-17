package components

import "fmt"

// SortState and SortDirection provide shared sorting utilities that can be reused
// across different views (incidents, alerts, etc.). Each view defines its own
// sort field enum but uses these common types for direction and state management.

type SortDirection int

const (
	SortAsc SortDirection = iota
	SortDesc
)

type SortState struct {
	Field     interface{}
	Direction SortDirection
	enabled   bool
}

func NewSortState() *SortState {
	return &SortState{
		Field:     nil,
		Direction: SortDesc,
		enabled:   false,
	}
}

func (s *SortState) Toggle(newField interface{}) bool {
	if s.Field == newField && s.enabled {
		// Toggling direction on same field
		if s.Direction == SortAsc {
			s.Direction = SortDesc
		} else {
			s.Direction = SortAsc
		}
		return false // Direction changed, need to reload
	}
	// New field selected
	s.Field = newField
	s.Direction = SortDesc
	s.enabled = true
	return true // Field changed, need to reload
}

func (s *SortState) IsEnabled() bool {
	return s.enabled
}

func (s *SortState) IsField(field interface{}) bool {
	return s.enabled && s.Field == field
}

func (s *SortState) GetIndicator() string {
	if !s.enabled {
		return ""
	}
	if s.Direction == SortDesc {
		return "↓"
	}
	return "↑"
}

func (s *SortState) GetInfo(fieldName string) string {
	if !s.enabled {
		return ""
	}
	return fmt.Sprintf("%s %s", s.GetIndicator(), fieldName)
}

func (s *SortState) ApplyDirection(less bool) bool {
	if s.Direction == SortDesc {
		return !less
	}
	return less
}
