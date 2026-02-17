package views

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/rootlyhq/rootly-tui/internal/api"
)

func TestNewAlertsModel(t *testing.T) {
	m := NewAlertsModel()

	if m.SelectedIndex() != 0 {
		t.Errorf("expected cursor to be 0, got %d", m.SelectedIndex())
	}

	if len(m.alerts) != 0 {
		t.Errorf("expected no alerts initially, got %d", len(m.alerts))
	}
}

func TestAlertsModelSetAlerts(t *testing.T) {
	m := NewAlertsModel()
	alerts := api.MockAlerts()
	pagination := api.PaginationInfo{CurrentPage: 1, HasNext: true, HasPrev: false}

	m.SetAlerts(alerts, pagination)

	if len(m.alerts) != len(alerts) {
		t.Errorf("expected %d alerts, got %d", len(alerts), len(m.alerts))
	}

	if m.loading {
		t.Error("expected loading to be false after SetAlerts")
	}

	if m.currentPage != 1 {
		t.Errorf("expected currentPage 1, got %d", m.currentPage)
	}
}

func TestAlertsModelSetLoading(t *testing.T) {
	m := NewAlertsModel()

	m.SetLoading(true)
	if !m.loading {
		t.Error("expected loading to be true")
	}

	m.SetLoading(false)
	if m.loading {
		t.Error("expected loading to be false")
	}
}

func TestAlertsModelSetError(t *testing.T) {
	m := NewAlertsModel()

	m.SetError("test error")

	if m.error != "test error" {
		t.Errorf("expected error 'test error', got '%s'", m.error)
	}

	if m.loading {
		t.Error("expected loading to be false after SetError")
	}
}

func TestAlertsModelSelectedAlert(t *testing.T) {
	m := NewAlertsModel()
	alerts := api.MockAlerts()
	m.SetAlerts(alerts, api.PaginationInfo{CurrentPage: 1})

	selected := m.SelectedAlert()
	if selected == nil {
		t.Fatal("expected selected alert to be non-nil")
		return
	}

	if selected.ID != alerts[0].ID {
		t.Errorf("expected selected ID '%s', got '%s'", alerts[0].ID, selected.ID)
	}
}

func TestAlertsModelNavigation(t *testing.T) {
	m := NewAlertsModel()
	alerts := api.MockAlerts()
	m.SetAlerts(alerts, api.PaginationInfo{CurrentPage: 1})
	m.SetDimensions(100, 30)

	// Test move down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.SelectedIndex() != 1 {
		t.Errorf("expected cursor 1 after 'j', got %d", m.SelectedIndex())
	}

	// Test move up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.SelectedIndex() != 0 {
		t.Errorf("expected cursor 0 after 'k', got %d", m.SelectedIndex())
	}

	// Test move up at top
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.SelectedIndex() != 0 {
		t.Errorf("expected cursor 0 at top, got %d", m.SelectedIndex())
	}

	// Test go to bottom
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if m.SelectedIndex() != len(alerts)-1 {
		t.Errorf("expected cursor %d at bottom, got %d", len(alerts)-1, m.SelectedIndex())
	}

	// Test go to top
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if m.SelectedIndex() != 0 {
		t.Errorf("expected cursor 0 at top, got %d", m.SelectedIndex())
	}
}

func TestAlertsModelView(t *testing.T) {
	m := NewAlertsModel()
	m.SetDimensions(100, 30)

	// Test empty view
	view := m.View()
	if !strings.Contains(view, "No alerts found") {
		t.Error("expected 'No alerts found' in empty view")
	}

	// Test loading view
	m.SetLoading(true)
	view = m.View()
	if !strings.Contains(view, "Loading") {
		t.Error("expected 'Loading' in loading view")
	}

	// Test error view
	m.SetLoading(false)
	m.SetError("API error")
	view = m.View()
	if !strings.Contains(view, "API error") {
		t.Error("expected error message in view")
	}

	// Test with alerts
	m.SetAlerts(api.MockAlerts(), api.PaginationInfo{CurrentPage: 1})
	view = m.View()
	if !strings.Contains(view, "ALERTS") {
		t.Error("expected 'ALERTS' in view with data")
	}
}

func TestAlertsModelCursorBounds(t *testing.T) {
	m := NewAlertsModel()
	alerts := api.MockAlerts()
	m.SetAlerts(alerts, api.PaginationInfo{CurrentPage: 1})
	m.SetDimensions(100, 30)

	// Move cursor beyond bounds
	for i := 0; i < len(alerts)+5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}

	if m.SelectedIndex() >= len(alerts) {
		t.Errorf("cursor exceeded bounds: got %d, max should be %d", m.SelectedIndex(), len(alerts)-1)
	}
}

func TestAlertsModelInit(t *testing.T) {
	m := NewAlertsModel()
	cmd := m.Init()

	// Init should return nil for AlertsModel
	if cmd != nil {
		t.Error("expected Init to return nil")
	}
}

func TestAlertsModelSetSpinner(t *testing.T) {
	m := NewAlertsModel()

	m.SetSpinner("⠋")
	if m.spinnerView != "⠋" {
		t.Errorf("expected spinner '⠋', got '%s'", m.spinnerView)
	}

	m.SetSpinner("⠙")
	if m.spinnerView != "⠙" {
		t.Errorf("expected spinner '⠙', got '%s'", m.spinnerView)
	}
}

func TestAlertsModelCurrentPage(t *testing.T) {
	m := NewAlertsModel()

	// Default page should be 1
	if m.CurrentPage() != 1 {
		t.Errorf("expected default page 1, got %d", m.CurrentPage())
	}

	// After setting alerts with pagination
	m.SetAlerts(api.MockAlerts(), api.PaginationInfo{CurrentPage: 3, HasNext: true, HasPrev: true})
	if m.CurrentPage() != 3 {
		t.Errorf("expected page 3, got %d", m.CurrentPage())
	}
}

func TestAlertsModelHasNextPage(t *testing.T) {
	m := NewAlertsModel()

	// Default should be false
	if m.HasNextPage() {
		t.Error("expected HasNextPage to be false initially")
	}

	// Set with hasNext = true
	m.SetAlerts(api.MockAlerts(), api.PaginationInfo{CurrentPage: 1, HasNext: true})
	if !m.HasNextPage() {
		t.Error("expected HasNextPage to be true")
	}

	// Set with hasNext = false
	m.SetAlerts(api.MockAlerts(), api.PaginationInfo{CurrentPage: 1, HasNext: false})
	if m.HasNextPage() {
		t.Error("expected HasNextPage to be false")
	}
}

func TestAlertsModelHasPrevPage(t *testing.T) {
	m := NewAlertsModel()

	// Default should be false
	if m.HasPrevPage() {
		t.Error("expected HasPrevPage to be false initially")
	}

	// Set with hasPrev = true
	m.SetAlerts(api.MockAlerts(), api.PaginationInfo{CurrentPage: 2, HasPrev: true})
	if !m.HasPrevPage() {
		t.Error("expected HasPrevPage to be true")
	}

	// Set with hasPrev = false
	m.SetAlerts(api.MockAlerts(), api.PaginationInfo{CurrentPage: 1, HasPrev: false})
	if m.HasPrevPage() {
		t.Error("expected HasPrevPage to be false")
	}
}

func TestAlertsModelNextPage(t *testing.T) {
	m := NewAlertsModel()
	m.SetAlerts(api.MockAlerts(), api.PaginationInfo{CurrentPage: 1, HasNext: true})
	m.SetDimensions(100, 30)
	// Move cursor to non-zero
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	m.NextPage()

	if m.currentPage != 2 {
		t.Errorf("expected page 2 after NextPage, got %d", m.currentPage)
	}
	if m.SelectedIndex() != 0 {
		t.Errorf("expected cursor reset to 0 after NextPage, got %d", m.SelectedIndex())
	}
}

func TestAlertsModelNextPageAtEnd(t *testing.T) {
	m := NewAlertsModel()
	m.SetAlerts(api.MockAlerts(), api.PaginationInfo{CurrentPage: 5, HasNext: false})

	m.NextPage()

	// Should not change when hasNext is false
	if m.currentPage != 5 {
		t.Errorf("expected page to stay at 5 when no next, got %d", m.currentPage)
	}
}

func TestAlertsModelNextPageRespectsTotal(t *testing.T) {
	m := NewAlertsModel()
	// HasNext is true but we're already at the last page (18/18)
	m.SetAlerts(api.MockAlerts(), api.PaginationInfo{
		CurrentPage: 18,
		TotalPages:  18,
		TotalCount:  887,
		HasNext:     true, // API might incorrectly say hasNext
	})

	m.NextPage()

	// Should not go beyond totalPages even if hasNext is true
	if m.currentPage != 18 {
		t.Errorf("expected page to stay at 18 (totalPages), got %d", m.currentPage)
	}
}

func TestAlertsModelNextPageWithZeroTotal(t *testing.T) {
	m := NewAlertsModel()
	// TotalPages is 0 (unknown), rely on hasNext
	m.SetAlerts(api.MockAlerts(), api.PaginationInfo{
		CurrentPage: 1,
		TotalPages:  0,
		HasNext:     true,
	})

	m.NextPage()

	// Should allow pagination when totalPages is unknown
	if m.currentPage != 2 {
		t.Errorf("expected page 2 when totalPages is 0, got %d", m.currentPage)
	}
}

func TestAlertsModelPrevPage(t *testing.T) {
	m := NewAlertsModel()
	m.SetAlerts(api.MockAlerts(), api.PaginationInfo{CurrentPage: 3, HasPrev: true})
	m.SetDimensions(100, 30)
	// Move cursor to non-zero
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}) // Go to bottom

	m.PrevPage()

	if m.currentPage != 2 {
		t.Errorf("expected page 2 after PrevPage, got %d", m.currentPage)
	}
	if m.SelectedIndex() != 0 {
		t.Errorf("expected cursor reset to 0 after PrevPage, got %d", m.SelectedIndex())
	}
}

func TestAlertsModelPrevPageAtStart(t *testing.T) {
	m := NewAlertsModel()
	m.SetAlerts(api.MockAlerts(), api.PaginationInfo{CurrentPage: 1, HasPrev: false})

	m.PrevPage()

	// Should not change when hasPrev is false or at page 1
	if m.currentPage != 1 {
		t.Errorf("expected page to stay at 1 when no prev, got %d", m.currentPage)
	}
}

func TestAlertsModelSelectedAlertEmpty(t *testing.T) {
	m := NewAlertsModel()

	// With no alerts, should return nil
	selected := m.SelectedAlert()
	if selected != nil {
		t.Error("expected nil for selected alert when no alerts")
	}
}

func TestAlertsModelSetAlertsCursorAdjustment(t *testing.T) {
	m := NewAlertsModel()
	// First set a larger list to allow cursor to move beyond 2
	largeAlerts := api.MockAlerts()
	m.SetAlerts(largeAlerts, api.PaginationInfo{CurrentPage: 1})
	m.SetDimensions(100, 30)
	// Move cursor beyond 2
	for i := 0; i < 5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}

	// Set only 2 alerts - cursor should be adjusted
	alerts := largeAlerts[:2]
	m.SetAlerts(alerts, api.PaginationInfo{CurrentPage: 1})

	if m.SelectedIndex() >= len(alerts) {
		t.Errorf("cursor should be adjusted to valid range, got %d for %d alerts", m.SelectedIndex(), len(alerts))
	}
}

func TestAlertsModelWindowSizeMsg(t *testing.T) {
	m := NewAlertsModel()

	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if m.width != 120 {
		t.Errorf("expected width 120, got %d", m.width)
	}
	if m.height != 40 {
		t.Errorf("expected height 40, got %d", m.height)
	}
	if m.listWidth == 0 {
		t.Error("expected listWidth to be calculated")
	}
}

func TestFormatAlertTime(t *testing.T) {
	// Test with a known time
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	result := formatAlertTime(testTime)

	// Should contain the date
	if !strings.Contains(result, "Jan 15, 2024") {
		t.Errorf("expected result to contain 'Jan 15, 2024', got '%s'", result)
	}
}

func TestAlertsModelViewStripsNewlines(t *testing.T) {
	m := NewAlertsModel()
	m.SetDimensions(100, 40)

	// Alert with newlines in summary
	alerts := []api.Alert{
		{
			ID:        "1",
			ShortID:   "ABC123",
			Summary:   "Test alert\nwith newline\rand carriage return",
			Status:    "triggered",
			Source:    "datadog",
			CreatedAt: time.Now(),
		},
	}
	m.SetAlerts(alerts, api.PaginationInfo{CurrentPage: 1})

	view := m.View()

	// The summary should appear on a single line with the ID
	// Count lines that contain ABC123 - should be exactly 1 in the list portion
	lines := strings.Split(view, "\n")
	countWithID := 0
	for _, line := range lines {
		if strings.Contains(line, "ABC123") {
			countWithID++
		}
	}
	// ABC123 appears in list (1) and in detail pane header (1) = 2 total
	if countWithID < 1 {
		t.Error("expected at least 1 line containing ABC123")
	}
	// Verify no raw \r in any line with the ID
	for _, line := range lines {
		if strings.Contains(line, "ABC123") && strings.Contains(line, "\r") {
			t.Error("expected carriage returns to be stripped from lines containing ABC123")
		}
	}
}

func TestAlertsModelSelectedIndex(t *testing.T) {
	m := NewAlertsModel()
	alerts := api.MockAlerts()
	m.SetAlerts(alerts, api.PaginationInfo{CurrentPage: 1})
	m.SetDimensions(100, 30)

	// Initial index should be 0
	if m.SelectedIndex() != 0 {
		t.Errorf("expected initial index 0, got %d", m.SelectedIndex())
	}

	// Move cursor down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.SelectedIndex() != 1 {
		t.Errorf("expected index 1 after j, got %d", m.SelectedIndex())
	}

	// Move cursor down again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.SelectedIndex() != 2 {
		t.Errorf("expected index 2 after j, got %d", m.SelectedIndex())
	}
}

func TestAlertsModelSetDetailLoading(t *testing.T) {
	m := NewAlertsModel()

	// Default should not be loading
	if m.IsDetailLoading() {
		t.Error("expected IsDetailLoading to be false initially")
	}

	// Set loading for a specific alert ID
	m.SetDetailLoading("alert-123")
	if !m.IsDetailLoading() {
		t.Error("expected IsDetailLoading to be true after SetDetailLoading")
	}
	if !m.IsLoadingAlert("alert-123") {
		t.Error("expected IsLoadingAlert to be true for alert-123")
	}
	if m.IsLoadingAlert("alert-456") {
		t.Error("expected IsLoadingAlert to be false for different alert ID")
	}

	// Clear loading
	m.ClearDetailLoading()
	if m.IsDetailLoading() {
		t.Error("expected IsDetailLoading to be false after ClearDetailLoading")
	}
}

func TestAlertsModelUpdateAlertDetail(t *testing.T) {
	m := NewAlertsModel()
	alerts := api.MockAlerts()
	m.SetAlerts(alerts, api.PaginationInfo{CurrentPage: 1})

	// Get original alert
	originalID := m.alerts[0].ID
	if originalID != alerts[0].ID {
		t.Fatalf("expected original ID %s, got %s", alerts[0].ID, originalID)
	}

	// Create detailed alert
	detailedAlert := &api.Alert{
		ID:           alerts[0].ID,
		ShortID:      alerts[0].ShortID,
		Summary:      "Detailed Summary",
		Status:       "resolved",
		Source:       "datadog",
		DetailLoaded: true,
		Urgency:      "High",
		Responders:   []string{"On-call Engineer", "Team Lead"},
	}

	// Update at index 0
	m.UpdateAlertDetail(0, detailedAlert)

	// Verify the alert was updated
	updated := m.alerts[0]
	if !updated.DetailLoaded {
		t.Error("expected DetailLoaded to be true after update")
	}
	if updated.Urgency != "High" {
		t.Errorf("expected Urgency 'High', got '%s'", updated.Urgency)
	}
	if len(updated.Responders) != 2 {
		t.Errorf("expected 2 responders, got %d", len(updated.Responders))
	}
	if updated.Responders[0] != "On-call Engineer" {
		t.Errorf("expected first responder 'On-call Engineer', got '%s'", updated.Responders[0])
	}
}

func TestAlertsModelUpdateAlertDetailInvalidIndex(t *testing.T) {
	m := NewAlertsModel()
	alerts := api.MockAlerts()
	m.SetAlerts(alerts, api.PaginationInfo{CurrentPage: 1})

	detailedAlert := &api.Alert{
		ID:           "new",
		DetailLoaded: true,
	}

	// Update at invalid index should not panic
	m.UpdateAlertDetail(-1, detailedAlert)
	m.UpdateAlertDetail(100, detailedAlert)

	// Verify nothing changed
	if m.alerts[0].DetailLoaded {
		t.Error("expected DetailLoaded to remain unchanged for invalid index")
	}
}

func TestAlertsModelUpdateAlertDetailNil(t *testing.T) {
	m := NewAlertsModel()
	alerts := api.MockAlerts()
	m.SetAlerts(alerts, api.PaginationInfo{CurrentPage: 1})

	// Update with nil should not panic
	m.UpdateAlertDetail(0, nil)

	// Verify nothing changed
	if m.alerts[0].DetailLoaded {
		t.Error("expected DetailLoaded to remain unchanged for nil alert")
	}
}

func TestAlertsModelViewShowsDetailHint(t *testing.T) {
	m := NewAlertsModel()
	m.SetDimensions(100, 40)

	// Alert without detail loaded
	alerts := []api.Alert{
		{
			ID:           "1",
			ShortID:      "ABC123",
			Summary:      "Test alert",
			Status:       "triggered",
			Source:       "datadog",
			CreatedAt:    time.Now(),
			DetailLoaded: false,
		},
	}
	m.SetAlerts(alerts, api.PaginationInfo{CurrentPage: 1})

	view := m.View()
	if !strings.Contains(view, "Press Enter for more details") {
		t.Error("expected hint 'Press Enter for more details' when detail not loaded")
	}
}

func TestAlertsModelViewHidesDetailHint(t *testing.T) {
	m := NewAlertsModel()
	m.SetDimensions(100, 40)

	// Alert with detail loaded
	alerts := []api.Alert{
		{
			ID:           "1",
			ShortID:      "ABC123",
			Summary:      "Test alert",
			Status:       "triggered",
			Source:       "datadog",
			CreatedAt:    time.Now(),
			DetailLoaded: true,
		},
	}
	m.SetAlerts(alerts, api.PaginationInfo{CurrentPage: 1})

	view := m.View()
	if strings.Contains(view, "Press Enter for more details") {
		t.Error("expected no hint when detail is loaded")
	}
}

func TestAlertsModelViewShowsExtendedDetail(t *testing.T) {
	m := NewAlertsModel()
	m.SetDimensions(100, 40)

	// Alert with extended detail
	alerts := []api.Alert{
		{
			ID:           "1",
			ShortID:      "ABC123",
			Summary:      "Test alert",
			Status:       "resolved",
			Source:       "datadog",
			CreatedAt:    time.Now(),
			DetailLoaded: true,
			Urgency:      "High",
			Responders:   []string{"On-call Engineer", "Team Lead"},
		},
	}
	m.SetAlerts(alerts, api.PaginationInfo{CurrentPage: 1})

	view := m.View()

	// Should show urgency
	if !strings.Contains(view, "Urgency") {
		t.Error("expected 'Urgency' in detail view")
	}
	if !strings.Contains(view, "High") {
		t.Error("expected 'High' urgency in detail view")
	}

	// Should show responders
	if !strings.Contains(view, "Responders") {
		t.Error("expected 'Responders' section in detail view")
	}
	if !strings.Contains(view, "On-call Engineer") {
		t.Error("expected 'On-call Engineer' in detail view")
	}
	if !strings.Contains(view, "Team Lead") {
		t.Error("expected 'Team Lead' in detail view")
	}
}

func TestIsURL(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"https://example.com", true},
		{"http://example.com", true},
		{"https://example.com/path/to/resource", true},
		{"http://localhost:8080/api/v1", true},
		{"https://example.com?query=param&foo=bar", true},
		{"HTTPS://EXAMPLE.COM", false}, // Case sensitive prefix check
		{"HTTP://EXAMPLE.COM", false},
		{"ftp://example.com", false},
		{"example.com", false},
		{"www.example.com", false},
		{"not a url", false},
		{"", false},
		{"httpsfake://example.com", false},
		{"https", false},
		{"http://", true}, // Technically valid prefix
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isURL(tt.input)
			if result != tt.expected {
				t.Errorf("isURL(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestAlertsModelRenderLabelValue(t *testing.T) {
	m := NewAlertsModel()
	m.SetDimensions(100, 40)

	tests := []struct {
		name        string
		value       string
		expectsLink bool
	}{
		{"https URL", "https://example.com/path", true},
		{"http URL", "http://example.com/path", true},
		{"plain text", "some-value", false},
		{"email", "user@example.com", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.renderLabelValue(tt.value)

			// URLs should contain OSC 8 escape sequence for terminal hyperlinks
			// The format is: \x1b]8;;URL\x07DISPLAY_TEXT\x1b]8;;\x07
			hasOSC8 := strings.Contains(result, "\x1b]8;;")

			if tt.expectsLink && !hasOSC8 {
				t.Errorf("renderLabelValue(%q) expected to contain terminal hyperlink, got %q", tt.value, result)
			}
			if !tt.expectsLink && hasOSC8 {
				t.Errorf("renderLabelValue(%q) should not contain terminal hyperlink, got %q", tt.value, result)
			}
		})
	}
}

func TestAlertsModelRenderLabelValueTruncation(t *testing.T) {
	m := NewAlertsModel()
	// Set a small width to trigger truncation
	m.SetDimensions(80, 40)

	longURL := "https://example.com/very/long/path/that/should/definitely/be/truncated/for/display/purposes"
	result := m.renderLabelValue(longURL)

	// Should contain ellipsis for truncated display
	if !strings.Contains(result, "...") {
		t.Error("expected truncated URL to contain '...'")
	}

	// Original URL should still be in the hyperlink target
	if !strings.Contains(result, longURL) {
		t.Error("expected original URL to be preserved in hyperlink target")
	}
}

func TestAlertsModelViewShowsClickableLabels(t *testing.T) {
	m := NewAlertsModel()
	m.SetDimensions(120, 40)

	// Alert with URL in labels
	alerts := []api.Alert{
		{
			ID:           "1",
			ShortID:      "ABC123",
			Summary:      "Test alert with URL label",
			Status:       "triggered",
			Source:       "datadog",
			CreatedAt:    time.Now(),
			DetailLoaded: true,
			Labels: map[string]string{
				"runbook":    "https://wiki.example.com/runbooks/alert-abc",
				"region":     "us-west-2",
				"dashboard":  "https://grafana.example.com/d/abc123",
				"owner_team": "platform",
			},
		},
	}
	m.SetAlerts(alerts, api.PaginationInfo{CurrentPage: 1})

	view := m.View()

	// Should show labels section
	if !strings.Contains(view, "Labels") {
		t.Error("expected 'Labels' section in detail view")
	}

	// Should show the label keys
	if !strings.Contains(view, "runbook") {
		t.Error("expected 'runbook' label key in view")
	}
	if !strings.Contains(view, "region") {
		t.Error("expected 'region' label key in view")
	}

	// URLs should be rendered as terminal hyperlinks (OSC 8 escape sequences)
	if !strings.Contains(view, "\x1b]8;;https://wiki.example.com") {
		t.Error("expected runbook URL to be rendered as terminal hyperlink")
	}
	if !strings.Contains(view, "\x1b]8;;https://grafana.example.com") {
		t.Error("expected dashboard URL to be rendered as terminal hyperlink")
	}

	// Plain text values should appear without OSC 8 sequence
	// Check that "us-west-2" is in the view but preceded by region label
	if !strings.Contains(view, "us-west-2") {
		t.Error("expected 'us-west-2' plain text value in view")
	}
}

func TestAlertsModelSetLayout(t *testing.T) {
	m := NewAlertsModel()
	m.SetDimensions(100, 50)

	// Default layout should be horizontal
	if m.layout != "" && m.layout != "horizontal" {
		t.Errorf("expected default layout to be empty or 'horizontal', got '%s'", m.layout)
	}

	// Set vertical layout
	m.SetLayout("vertical")
	if m.layout != "vertical" {
		t.Errorf("expected layout 'vertical', got '%s'", m.layout)
	}

	// Set horizontal layout
	m.SetLayout("horizontal")
	if m.layout != "horizontal" {
		t.Errorf("expected layout 'horizontal', got '%s'", m.layout)
	}
}

func TestAlertsModelLayoutDimensions(t *testing.T) {
	m := NewAlertsModel()
	m.SetDimensions(200, 100)

	// Test horizontal layout dimensions
	m.SetLayout("horizontal")
	horizontalListWidth := m.listWidth
	horizontalListHeight := m.listHeight
	horizontalDetailHeight := m.detailHeight

	// List and detail should have similar heights in horizontal
	if horizontalListHeight != horizontalDetailHeight {
		t.Errorf("horizontal layout: expected equal heights, got list=%d detail=%d",
			horizontalListHeight, horizontalDetailHeight)
	}

	// Test vertical layout dimensions
	m.SetLayout("vertical")
	verticalListWidth := m.listWidth
	verticalDetailWidth := m.detailWidth
	verticalListHeight := m.listHeight
	verticalDetailHeight := m.detailHeight

	// In vertical layout, widths should be similar (full width)
	if verticalListWidth != verticalDetailWidth {
		t.Errorf("vertical layout: expected equal widths, got list=%d detail=%d",
			verticalListWidth, verticalDetailWidth)
	}

	// In vertical layout, widths should be larger than horizontal
	if verticalListWidth <= horizontalListWidth {
		t.Errorf("vertical layout: expected list width > horizontal list width, got %d <= %d",
			verticalListWidth, horizontalListWidth)
	}

	// In vertical layout, heights should add up approximately to total
	totalHeight := verticalListHeight + verticalDetailHeight
	expectedTotal := 100 - 2 // height - overhead
	if totalHeight < expectedTotal-5 || totalHeight > expectedTotal+5 {
		t.Errorf("vertical layout: heights don't add up correctly, got %d expected ~%d",
			totalHeight, expectedTotal)
	}
}

func TestAlertsModelVerticalLayoutView(t *testing.T) {
	m := NewAlertsModel()
	m.SetDimensions(100, 50)
	m.SetLayout("vertical")

	alerts := api.MockAlerts()
	m.SetAlerts(alerts, api.PaginationInfo{CurrentPage: 1})

	view := m.View()

	// View should still contain alerts
	if !strings.Contains(view, "ALERTS") {
		t.Error("expected 'ALERTS' in vertical layout view")
	}
}

func TestAlertsModelHorizontalLayoutView(t *testing.T) {
	m := NewAlertsModel()
	m.SetDimensions(100, 50)
	m.SetLayout("horizontal")

	alerts := api.MockAlerts()
	m.SetAlerts(alerts, api.PaginationInfo{CurrentPage: 1})

	view := m.View()

	// View should still contain alerts
	if !strings.Contains(view, "ALERTS") {
		t.Error("expected 'ALERTS' in horizontal layout view")
	}
}

func TestAlertsModelLayoutPageSize(t *testing.T) {
	m := NewAlertsModel()

	// Test with small height (vertical layout will have less space per pane)
	m.SetDimensions(100, 30)

	// In horizontal layout, full height available
	m.SetLayout("horizontal")
	// Page size is calculated based on tableHeight

	// In vertical layout, only ~45% height available
	m.SetLayout("vertical")
	// The table should adapt its page size to fit

	// Just verify no panic and dimensions are set
	if m.listHeight == 0 || m.detailHeight == 0 {
		t.Error("expected non-zero heights after layout set")
	}
}
