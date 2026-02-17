package debug

import (
	"os"
	"testing"
)

func TestNewRingBuffer(t *testing.T) {
	rb := newRingBuffer(10)

	if rb.maxSize != 10 {
		t.Errorf("expected maxSize 10, got %d", rb.maxSize)
	}
	if rb.pos != 0 {
		t.Errorf("expected pos 0, got %d", rb.pos)
	}
	if rb.full {
		t.Error("expected full to be false")
	}
}

func TestRingBufferWrite(t *testing.T) {
	rb := newRingBuffer(3)

	// Write first entry
	n, err := rb.Write([]byte("entry1"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != 6 {
		t.Errorf("expected 6 bytes written, got %d", n)
	}

	entries := rb.GetEntries()
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
	if entries[0] != "entry1" {
		t.Errorf("expected 'entry1', got '%s'", entries[0])
	}
}

func TestRingBufferWraparound(t *testing.T) {
	rb := newRingBuffer(3)

	// Fill buffer
	_, _ = rb.Write([]byte("a"))
	_, _ = rb.Write([]byte("b"))
	_, _ = rb.Write([]byte("c"))

	// Should not be full yet (pos wraps to 0)
	if !rb.full {
		t.Error("expected buffer to be full after 3 writes")
	}

	// Add one more to wrap around
	_, _ = rb.Write([]byte("d"))

	entries := rb.GetEntries()
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Should be in order: b, c, d (oldest first)
	expected := []string{"b", "c", "d"}
	for i, e := range expected {
		if entries[i] != e {
			t.Errorf("entry[%d] = '%s', expected '%s'", i, entries[i], e)
		}
	}
}

func TestRingBufferClear(t *testing.T) {
	rb := newRingBuffer(5)

	_, _ = rb.Write([]byte("test1"))
	_, _ = rb.Write([]byte("test2"))

	rb.Clear()

	entries := rb.GetEntries()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after clear, got %d", len(entries))
	}
	if rb.pos != 0 {
		t.Errorf("expected pos 0 after clear, got %d", rb.pos)
	}
	if rb.full {
		t.Error("expected full to be false after clear")
	}
}

func TestPrettyJSON(t *testing.T) {
	input := []byte(`{"name":"test","value":123}`)
	result := PrettyJSON(input)

	// Should contain indentation
	if result == string(input) {
		t.Error("expected prettified JSON to be different from input")
	}

	// Should contain newlines
	hasNewline := false
	for _, c := range result {
		if c == '\n' {
			hasNewline = true
			break
		}
	}
	if !hasNewline {
		t.Error("expected prettified JSON to contain newlines")
	}
}

func TestPrettyJSONInvalidInput(t *testing.T) {
	input := []byte(`not valid json`)
	result := PrettyJSON(input)

	// Should return original for invalid JSON
	if result != string(input) {
		t.Errorf("expected original input for invalid JSON, got '%s'", result)
	}
}

func TestGetLogs(t *testing.T) {
	// Clear existing logs
	ClearLogs()

	Logger.Info("test log entry")

	logs := GetLogs()
	if len(logs) < 1 {
		t.Error("expected at least 1 log entry")
	}
}

func TestClearLogs(t *testing.T) {
	Logger.Info("test entry")

	ClearLogs()

	logs := GetLogs()
	if len(logs) != 0 {
		t.Errorf("expected 0 logs after clear, got %d", len(logs))
	}
}

func TestEnableDisable(t *testing.T) {
	// Test enable
	Enable()
	if !Enabled {
		t.Error("expected Enabled to be true after Enable()")
	}

	// Test disable
	Disable()
	if Enabled {
		t.Error("expected Enabled to be false after Disable()")
	}
}

func TestSetLogFile(t *testing.T) {
	// Create temp file for test
	tmpFile, err := os.CreateTemp("", "rootly-tui-debug-test-*.log")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Set log file
	err = SetLogFile(tmpPath)
	if err != nil {
		t.Fatalf("SetLogFile failed: %v", err)
	}

	// Write a log entry
	Logger.Info("test log to file")

	// Verify file has content
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if len(content) == 0 {
		t.Error("expected log file to have content")
	}

	// Reset to buffer only
	Disable()
}

func TestSetLogFileInvalidPath(t *testing.T) {
	// Try to set log file to invalid path
	err := SetLogFile("/nonexistent/directory/file.log")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestEnableWithFileOutput(t *testing.T) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "rootly-tui-debug-test-*.log")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Set file first
	err = SetLogFile(tmpPath)
	if err != nil {
		t.Fatalf("SetLogFile failed: %v", err)
	}

	// Now enable - should use file output
	Enable()
	if !Enabled {
		t.Error("expected Enabled to be true")
	}

	// Write something
	Logger.Info("test with file enabled")

	// Cleanup
	Disable()
	fileOutput = nil
}
