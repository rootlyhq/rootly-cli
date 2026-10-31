package builtin

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
)

type Result struct {
	Output  string
	IsError bool
}

type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	AdvertisedReadOnly() bool
	NeedsApproval(map[string]any) bool
	Run(context.Context, map[string]any) Result
}

type LookPathFunc func(string) (string, error)

type Registry struct {
	tools  []Tool
	byName map[string]Tool
}

type localTool struct {
	name          string
	description   string
	schema        json.RawMessage
	readOnly      bool
	needsApproval func(map[string]any) bool
	run           func(context.Context, map[string]any) Result
}

func (t localTool) Name() string                           { return t.name }
func (t localTool) Description() string                    { return t.description }
func (t localTool) Schema() json.RawMessage                { return append(json.RawMessage(nil), t.schema...) }
func (t localTool) AdvertisedReadOnly() bool               { return t.readOnly }
func (t localTool) NeedsApproval(args map[string]any) bool { return t.needsApproval(args) }
func (t localTool) Run(ctx context.Context, args map[string]any) Result {
	return t.run(ctx, args)
}

func NewRegistry(root string, lookPath LookPathFunc) (*Registry, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	canonicalRoot, err := canonicalizeRoot(root)
	if err != nil {
		return nil, err
	}
	registry := &Registry{byName: map[string]Tool{}}
	registry.add(newReadFileTool(canonicalRoot))
	registry.add(newGrepTool(canonicalRoot))
	registry.add(newGlobTool(canonicalRoot))
	registry.add(newListDirTool(canonicalRoot))
	for _, spec := range cliRegistry {
		if _, err := lookPath(spec.bin); err == nil {
			registry.add(newCLITool(canonicalRoot, spec, lookPath))
		}
	}
	registry.add(newBashTool(canonicalRoot))
	return registry, nil
}

func (r *Registry) add(tool Tool) {
	r.tools = append(r.tools, tool)
	r.byName[tool.Name()] = tool
}

func (r *Registry) Tools() []Tool {
	return append([]Tool(nil), r.tools...)
}

func (r *Registry) Lookup(name string) (Tool, bool) {
	tool, ok := r.byName[name]
	return tool, ok
}

func canonicalizeRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		var err error
		root, err = filepath.Abs(".")
		if err != nil {
			return "", err
		}
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}
