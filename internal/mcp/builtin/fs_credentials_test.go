package builtin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesystemToolsDenyRootlyCLICredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	credentialDir := filepath.Join(home, ".rootly-cli")
	if err := os.MkdirAll(credentialDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"config.yaml":    "api_key: rootly-test-secret",
		"mcp-oauth.json": `{"access_token":"rootly-test-secret"}`,
	} {
		if err := os.WriteFile(filepath.Join(credentialDir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	registry := testRegistry(t, home, nil)
	readFile := mustLookup(t, registry, "local_read_file")
	for _, path := range []string{".rootly-cli/config.yaml", ".rootly-cli/mcp-oauth.json"} {
		result := readFile.Run(context.Background(), map[string]any{"path": path})
		if !result.IsError || !strings.Contains(result.Output, "path is denied") {
			t.Errorf("read_file did not deny %q: %#v", path, result)
		}
	}

	grep := mustLookup(t, registry, "local_grep").Run(context.Background(), map[string]any{
		"pattern":     "rootly-test-secret",
		"output_mode": "content",
	})
	if strings.Contains(grep.Output, "rootly-test-secret") || strings.Contains(grep.Output, ".rootly-cli") {
		t.Errorf("grep exposed Rootly CLI credentials: %q", grep.Output)
	}

	glob := mustLookup(t, registry, "local_glob").Run(context.Background(), map[string]any{"pattern": "**/*"})
	if strings.Contains(glob.Output, ".rootly-cli") || strings.Contains(glob.Output, "config.yaml") || strings.Contains(glob.Output, "mcp-oauth.json") {
		t.Errorf("glob exposed Rootly CLI credential paths: %q", glob.Output)
	}

	listing := mustLookup(t, registry, "local_list_dir").Run(context.Background(), map[string]any{"path": "."})
	if strings.Contains(listing.Output, ".rootly-cli") {
		t.Errorf("list_dir exposed Rootly CLI credential directory: %q", listing.Output)
	}
}
