package builtin

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	maxFileBytes   = 256 * 1024
	maxFileLines   = 2000
	maxSearchItems = 200
	maxDirEntries  = 500
	maxFSOutput    = 20_000
)

var errSearchLimit = errors.New("search result limit reached")

func newReadFileTool(root string) Tool {
	return localTool{
		name:          "local_read_file",
		description:   "Read a file inside the current working directory. Returns numbered lines.",
		schema:        jsonSchema(`{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"integer","minimum":0},"limit":{"type":"integer","minimum":1}},"required":["path"]}`),
		readOnly:      true,
		needsApproval: func(map[string]any) bool { return false },
		run: func(_ context.Context, args map[string]any) Result {
			filePath, ok := stringArg(args, "path")
			if !ok || strings.TrimSpace(filePath) == "" {
				return errorResult("path is required")
			}
			resolved, err := resolvePath(root, filePath)
			if err != nil {
				return errorResult("%v", err)
			}
			if deniedPath(resolved) {
				return errorResult("path is denied")
			}
			info, err := os.Stat(resolved)
			if err != nil {
				return errorResult("%v", err)
			}
			if !info.Mode().IsRegular() {
				return errorResult("path is not a regular file")
			}
			file, err := os.Open(resolved)
			if err != nil {
				return errorResult("%v", err)
			}
			data, readErr := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
			closeErr := file.Close()
			if readErr != nil {
				return errorResult("%v", readErr)
			}
			if closeErr != nil {
				return errorResult("%v", closeErr)
			}
			truncated := len(data) > maxFileBytes
			if truncated {
				data = data[:maxFileBytes]
			}
			lines := strings.Split(string(data), "\n")
			offset := intArg(args, "offset", 0)
			limit := intArg(args, "limit", maxFileLines)
			if offset < 0 || limit < 1 {
				return errorResult("offset must be non-negative and limit must be positive")
			}
			if limit > maxFileLines {
				limit = maxFileLines
			}
			start := min(offset, len(lines))
			end := min(start+limit, len(lines))
			var output strings.Builder
			for index := start; index < end; index++ {
				fmt.Fprintf(&output, "%6d\t%s", index+1, lines[index])
				if index+1 < end {
					output.WriteByte('\n')
				}
			}
			if end < len(lines) || truncated {
				fmt.Fprintf(&output, "\n…[%d more lines]", max(0, len(lines)-end))
			}
			return Result{Output: capOutput(output.String(), maxFSOutput)}
		},
	}
}

func newGrepTool(root string) Tool {
	return localTool{
		name:          "local_grep",
		description:   "Search files inside the current working directory. Supports files_with_matches, content, or count output.",
		schema:        jsonSchema(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"},"glob":{"type":"string"},"output_mode":{"type":"string","enum":["files_with_matches","content","count"]}},"required":["pattern"]}`),
		readOnly:      true,
		needsApproval: func(map[string]any) bool { return false },
		run: func(ctx context.Context, args map[string]any) Result {
			pattern, ok := stringArg(args, "pattern")
			if !ok || pattern == "" {
				return errorResult("pattern is required")
			}
			expression, err := regexp.Compile(pattern)
			if err != nil {
				return errorResult("invalid pattern: %v", err)
			}
			searchPath, ok := stringArg(args, "path")
			if !ok || strings.TrimSpace(searchPath) == "" {
				searchPath = "."
			}
			start, err := resolvePath(root, searchPath)
			if err != nil {
				return errorResult("%v", err)
			}
			if deniedPath(start) {
				return errorResult("path is denied")
			}
			info, err := os.Stat(start)
			if err != nil {
				return errorResult("%v", err)
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				return errorResult("path is not a file or directory")
			}
			mode, ok := stringArg(args, "output_mode")
			if !ok || mode == "" {
				mode = "files_with_matches"
			}
			if mode != "files_with_matches" && mode != "content" && mode != "count" {
				return errorResult("output_mode must be files_with_matches, content, or count")
			}
			glob, _ := stringArg(args, "glob")
			if glob != "" && filepath.IsAbs(glob) {
				return errorResult("glob must be relative")
			}
			results := make([]string, 0, maxSearchItems)
			err = walkFiles(ctx, root, start, func(filePath string) error {
				if glob != "" && !globMatch(glob, relativeTo(start, filePath)) {
					return nil
				}
				matches, scanErr := grepFile(filePath, expression, mode, relativeTo(root, filePath))
				if scanErr != nil {
					return nil
				}
				if len(matches) == 0 {
					return nil
				}
				if mode == "files_with_matches" {
					results = append(results, matches[0])
				} else {
					results = append(results, matches...)
				}
				if len(results) >= maxSearchItems {
					return errSearchLimit
				}
				return nil
			})
			if err != nil && !errors.Is(err, errSearchLimit) {
				return errorResult("%v", err)
			}
			if len(results) == 0 {
				return Result{Output: "(no matches)"}
			}
			return Result{Output: capOutput(strings.Join(results, "\n"), maxFSOutput)}
		},
	}
}

func newGlobTool(root string) Tool {
	return localTool{
		name:          "local_glob",
		description:   "Find files by glob pattern inside the current working directory. Returns at most 200 paths.",
		schema:        jsonSchema(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"}},"required":["pattern"]}`),
		readOnly:      true,
		needsApproval: func(map[string]any) bool { return false },
		run: func(ctx context.Context, args map[string]any) Result {
			pattern, ok := stringArg(args, "pattern")
			if !ok || strings.TrimSpace(pattern) == "" {
				return errorResult("pattern is required")
			}
			if filepath.IsAbs(pattern) || escapesRoot(pattern) {
				return errorResult("glob must stay inside the current working directory")
			}
			base, ok := stringArg(args, "path")
			if !ok || strings.TrimSpace(base) == "" {
				base = "."
			}
			start, err := resolvePath(root, base)
			if err != nil {
				return errorResult("%v", err)
			}
			if deniedPath(start) {
				return errorResult("path is denied")
			}
			info, err := os.Stat(start)
			if err != nil {
				return errorResult("%v", err)
			}
			if !info.IsDir() {
				return errorResult("path is not a directory")
			}
			matches := make([]string, 0, maxSearchItems)
			err = walkFiles(ctx, root, start, func(filePath string) error {
				if globMatch(filepath.ToSlash(pattern), filepath.ToSlash(relativeTo(start, filePath))) {
					matches = append(matches, filepath.ToSlash(relativeTo(start, filePath)))
					if len(matches) >= maxSearchItems {
						return errSearchLimit
					}
				}
				return nil
			})
			if err != nil && !errors.Is(err, errSearchLimit) {
				return errorResult("%v", err)
			}
			if len(matches) == 0 {
				return Result{Output: "(no matches)"}
			}
			sort.Strings(matches)
			return Result{Output: capOutput(strings.Join(matches, "\n"), maxFSOutput)}
		},
	}
}

func newListDirTool(root string) Tool {
	return localTool{
		name:          "local_list_dir",
		description:   "List the non-sensitive entries in a directory inside the current working directory.",
		schema:        jsonSchema(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		readOnly:      true,
		needsApproval: func(map[string]any) bool { return false },
		run: func(_ context.Context, args map[string]any) Result {
			dir, ok := stringArg(args, "path")
			if !ok || strings.TrimSpace(dir) == "" {
				dir = "."
			}
			resolved, err := resolvePath(root, dir)
			if err != nil {
				return errorResult("%v", err)
			}
			if deniedPath(resolved) {
				return errorResult("path is denied")
			}
			entries, err := os.ReadDir(resolved)
			if err != nil {
				return errorResult("%v", err)
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
			output := make([]string, 0, min(len(entries), maxDirEntries))
			for _, entry := range entries {
				entryPath := filepath.Join(resolved, entry.Name())
				target, targetErr := filepath.EvalSymlinks(entryPath)
				if entry.Name() == ".git" || deniedPath(entryPath) || targetErr != nil || !isWithin(root, target) || deniedPath(target) {
					continue
				}
				if len(output) == maxDirEntries {
					break
				}
				name := entry.Name()
				if entry.IsDir() {
					name += "/"
				}
				output = append(output, name)
			}
			if len(output) == 0 {
				return Result{Output: "(empty)"}
			}
			return Result{Output: capOutput(strings.Join(output, "\n"), maxFSOutput)}
		},
	}
}

func resolvePath(root, requested string) (string, error) {
	if strings.TrimSpace(requested) == "" {
		return "", errors.New("path is required")
	}
	requestedPath := requested
	if !filepath.IsAbs(requestedPath) {
		requestedPath = filepath.Join(root, requestedPath)
	}
	absolute, err := filepath.Abs(filepath.Clean(requestedPath))
	if err != nil {
		return "", err
	}
	if !isWithin(root, absolute) {
		return "", errors.New("path escapes the current working directory")
	}
	if deniedPath(absolute) {
		return "", errors.New("path is denied")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	if !isWithin(root, resolved) {
		return "", errors.New("path escapes the current working directory")
	}
	return resolved, nil
}

func isWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && !filepath.IsAbs(relative)
}

func deniedPath(candidate string) bool {
	clean := filepath.Clean(candidate)
	home, _ := os.UserHomeDir()
	if home != "" {
		for _, protected := range []string{
			filepath.Join(home, ".ssh"),
			filepath.Join(home, ".aws"),
			filepath.Join(home, ".config", "gcloud"),
			filepath.Join(home, ".kube", "config"),
		} {
			if isWithin(protected, clean) {
				return true
			}
		}
	}
	base := strings.ToLower(filepath.Base(clean))
	if strings.HasPrefix(base, ".env") || strings.HasSuffix(base, ".pem") ||
		strings.HasSuffix(base, ".key") || strings.HasPrefix(base, "id_") {
		return true
	}
	for parent := clean; ; parent = filepath.Dir(parent) {
		if filepath.Base(parent) == ".git" {
			return true
		}
		if parent == filepath.Dir(parent) {
			break
		}
	}
	return false
}

func walkFiles(ctx context.Context, root, start string, visit func(string) error) error {
	return filepath.WalkDir(start, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if filePath != start && deniedPath(filePath) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		resolved, err := filepath.EvalSymlinks(filePath)
		if err != nil || !isWithin(root, resolved) || deniedPath(resolved) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		return visit(filePath)
	})
}

func grepFile(filePath string, expression *regexp.Regexp, mode, displayPath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(io.LimitReader(file, maxFileBytes))
	scanner.Buffer(make([]byte, 64*1024), maxFileBytes)
	var matches []string
	count := 0
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if !expression.MatchString(line) {
			continue
		}
		count++
		switch mode {
		case "content":
			matches = append(matches, fmt.Sprintf("%s:%d:%s", displayPath, lineNumber, line))
		case "files_with_matches":
			return []string{displayPath}, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if mode == "count" && count > 0 {
		matches = append(matches, fmt.Sprintf("%s:%d", displayPath, count))
	}
	return matches, nil
}

func globMatch(pattern, value string) bool {
	patternParts := strings.Split(filepath.ToSlash(pattern), "/")
	valueParts := strings.Split(filepath.ToSlash(value), "/")
	return globSegmentsMatch(patternParts, valueParts)
}

func globSegmentsMatch(pattern, value []string) bool {
	if len(pattern) == 0 {
		return len(value) == 0
	}
	if pattern[0] == "**" {
		for index := 0; index <= len(value); index++ {
			if globSegmentsMatch(pattern[1:], value[index:]) {
				return true
			}
		}
		return false
	}
	if len(value) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], value[0])
	return err == nil && matched && globSegmentsMatch(pattern[1:], value[1:])
}

func relativeTo(base, value string) string {
	relative, err := filepath.Rel(base, value)
	if err != nil {
		return filepath.Base(value)
	}
	return relative
}

func escapesRoot(value string) bool {
	clean := filepath.Clean(value)
	return clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator))
}

func stringArg(args map[string]any, key string) (string, bool) {
	value, ok := args[key].(string)
	return value, ok
}

func intArg(args map[string]any, key string, fallback int) int {
	value, ok := args[key]
	if !ok {
		return fallback
	}
	switch number := value.(type) {
	case int:
		return number
	case int64:
		return int(number)
	case float64:
		return int(number)
	default:
		return fallback
	}
}

func errorResult(format string, values ...any) Result {
	return Result{Output: "error: " + fmt.Sprintf(format, values...), IsError: true}
}

func capOutput(output string, limit int) string {
	characters := []rune(output)
	if len(characters) <= limit {
		return output
	}
	marker := []rune("\n…[truncated]")
	if limit <= len(marker) {
		return string(characters[:limit])
	}
	return string(characters[:limit-len(marker)]) + string(marker)
}
