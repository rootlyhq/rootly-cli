package cmd

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

func TestConfirmAction_SkipConfirm(t *testing.T) {
	// When skipConfirm is true, should return nil immediately without prompting
	err := ConfirmAction("Delete everything?", true)
	if err != nil {
		t.Fatalf("expected nil error when skipConfirm=true, got: %v", err)
	}
}

func TestConfirmAction_NonInteractive(t *testing.T) {
	// When stdin is not a TTY (as in tests/CI), should return an error
	// telling user to use --yes flag
	err := ConfirmAction("Delete everything?", false)
	if err == nil {
		t.Fatal("expected error in non-interactive mode, got nil")
	}

	expected := "cannot prompt in non-interactive mode, use --yes flag"
	if err.Error() != expected {
		t.Fatalf("expected error %q, got %q", expected, err.Error())
	}
}

func TestFormatAPIError(t *testing.T) {
	original := errors.New("connection refused")
	wrapped := FormatAPIError("list incidents", original)

	expected := "failed to list incidents: connection refused"
	if wrapped.Error() != expected {
		t.Fatalf("expected %q, got %q", expected, wrapped.Error())
	}

	// Verify the original error is unwrappable
	if !errors.Is(wrapped, original) {
		t.Fatal("expected wrapped error to unwrap to original")
	}
}

func TestFormatAPIError_NilAction(t *testing.T) {
	original := errors.New("not found")
	wrapped := FormatAPIError("get incident", original)

	if wrapped == nil {
		t.Fatal("expected non-nil error")
	}

	if !errors.Is(wrapped, original) {
		t.Fatal("expected error chain to contain original error")
	}
}

func TestPrintDryRun(t *testing.T) {
	// Capture stderr output
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stderr = w

	PrintDryRun("delete incident", map[string]string{
		"ID":     "INC-123",
		"Status": "resolved",
	})

	w.Close()
	os.Stderr = origStderr

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	r.Close()
	output := string(buf[:n])

	// Check key parts of output
	if got := output; got == "" {
		t.Fatal("expected output on stderr, got nothing")
	}

	expectedParts := []string{
		"DRY RUN: no changes will be made",
		"Would delete incident:",
		"ID: INC-123",
		"Status: resolved",
	}

	for _, part := range expectedParts {
		if !contains(output, part) {
			t.Errorf("output missing expected part %q\nGot: %s", part, output)
		}
	}
}

func TestPrintDryRun_EmptyDetails(t *testing.T) {
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stderr = w

	PrintDryRun("update service", map[string]string{})

	w.Close()
	os.Stderr = origStderr

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	r.Close()
	output := string(buf[:n])

	if !contains(output, "DRY RUN: no changes will be made") {
		t.Errorf("expected dry run header, got: %s", output)
	}
	if !contains(output, "Would update service:") {
		t.Errorf("expected action description, got: %s", output)
	}
}

func TestAddConfirmFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	AddConfirmFlag(cmd)

	flag := cmd.Flags().Lookup("yes")
	if flag == nil {
		t.Fatal("expected --yes flag to be added")
	}
	if flag.Shorthand != "y" {
		t.Fatalf("expected shorthand 'y', got %q", flag.Shorthand)
	}
	if flag.DefValue != "false" {
		t.Fatalf("expected default value 'false', got %q", flag.DefValue)
	}
}

func TestAddDryRunFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	AddDryRunFlag(cmd)

	flag := cmd.Flags().Lookup("dry-run")
	if flag == nil {
		t.Fatal("expected --dry-run flag to be added")
	}
	if flag.DefValue != "false" {
		t.Fatalf("expected default value 'false', got %q", flag.DefValue)
	}
}

func TestErrAborted(t *testing.T) {
	// Verify ErrAborted is a proper sentinel error
	err := fmt.Errorf("wrapped: %w", ErrAborted)
	if !errors.Is(err, ErrAborted) {
		t.Fatal("expected errors.Is to match ErrAborted")
	}

	if ErrAborted.Error() != "aborted" {
		t.Fatalf("expected 'aborted', got %q", ErrAborted.Error())
	}
}

func TestSetVersionInfo(t *testing.T) {
	// Save originals
	origV, origC, origD := version, commit, date
	defer func() {
		version, commit, date = origV, origC, origD
	}()

	SetVersionInfo("1.2.3", "abc123", "2025-06-15")

	if version != "1.2.3" {
		t.Errorf("version = %q, want %q", version, "1.2.3")
	}
	if commit != "abc123" {
		t.Errorf("commit = %q, want %q", commit, "abc123")
	}
	if date != "2025-06-15" {
		t.Errorf("date = %q, want %q", date, "2025-06-15")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s != "" && containsString(s, substr))
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
