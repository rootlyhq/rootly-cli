package builtin

// The read-only command rules below follow google-gemini/gemini-cli
// (packages/core/src/sandbox/utils/commandSafety.ts, Apache-2.0,
// Copyright 2025 Google LLC), tightened for Rootly: no recursion into
// denied files, no git global options, and no shell expansion at all.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"
)

// shellCommands splits a bash script into the argv of each simple command.
// It returns ok=false for anything it cannot read literally: expansions,
// globs, subshells, loops, assignments, background jobs or redirections
// other than 2>&1 and >/dev/null.
func shellCommands(script string) ([][]string, bool) {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(script), "")
	if err != nil || len(file.Stmts) == 0 {
		return nil, false
	}
	var commands [][]string
	for _, stmt := range file.Stmts {
		if !collectStmt(stmt, &commands) {
			return nil, false
		}
	}
	return commands, len(commands) > 0
}

func collectStmt(stmt *syntax.Stmt, commands *[][]string) bool {
	if stmt == nil || stmt.Negated || stmt.Background || stmt.Coprocess || stmt.Disown {
		return false
	}
	for _, redirect := range stmt.Redirs {
		if !harmlessRedirect(redirect) {
			return false
		}
	}
	switch command := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		if len(command.Assigns) > 0 || len(command.Args) == 0 {
			return false
		}
		argv := make([]string, 0, len(command.Args))
		for _, word := range command.Args {
			value, ok := literalWord(word)
			if !ok {
				return false
			}
			argv = append(argv, value)
		}
		*commands = append(*commands, argv)
		return true
	case *syntax.BinaryCmd:
		switch command.Op {
		case syntax.AndStmt, syntax.OrStmt, syntax.Pipe, syntax.PipeAll:
			return collectStmt(command.X, commands) && collectStmt(command.Y, commands)
		}
	}
	return false
}

func harmlessRedirect(redirect *syntax.Redirect) bool {
	if redirect == nil || redirect.Hdoc != nil {
		return false
	}
	fd := ""
	if redirect.N != nil {
		fd = redirect.N.Value
	}
	target, ok := literalWord(redirect.Word)
	if !ok {
		return false
	}
	switch redirect.Op {
	case syntax.DplOut:
		return fd == "2" && target == "1"
	case syntax.RdrOut, syntax.AppOut:
		return (fd == "" || fd == "1" || fd == "2") && target == "/dev/null"
	case syntax.RdrAll, syntax.AppAll:
		return fd == "" && target == "/dev/null"
	}
	return false
}

func literalWord(word *syntax.Word) (string, bool) {
	if word == nil || len(word.Parts) == 0 {
		return "", false
	}
	var value strings.Builder
	for index, part := range word.Parts {
		switch part := part.(type) {
		case *syntax.Lit:
			if strings.ContainsAny(part.Value, "*?[]{}\\$`") || tildeExpands(part.Value, index == 0) {
				return "", false
			}
			value.WriteString(part.Value)
		case *syntax.SglQuoted:
			if part.Dollar {
				return "", false
			}
			value.WriteString(part.Value)
		case *syntax.DblQuoted:
			if part.Dollar {
				return "", false
			}
			for _, inner := range part.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok || strings.ContainsAny(lit.Value, "\\$`") {
					return "", false
				}
				value.WriteString(lit.Value)
			}
		default:
			return "", false
		}
	}
	return value.String(), true
}

// tildeExpands reports whether bash would expand ~ in this unquoted literal.
func tildeExpands(value string, wordStart bool) bool {
	return (wordStart && strings.HasPrefix(value, "~")) || strings.Contains(value, "=~") || strings.Contains(value, ":~")
}

var (
	plainSafeCommands = []string{"echo", "expr", "false", "id", "pwd", "seq", "true", "uname", "which", "whoami", "numfmt", "tr"}
	fileSafeCommands  = []string{"cat", "cut", "head", "ls", "nl", "paste", "rev", "stat", "tail", "tac", "uniq", "wc"}
	sedPrintArg       = regexp.MustCompile(`^\d+(,\d+)?p$`)
)

// isKnownSafeCommand reports whether argv only reads files under root.
func isKnownSafeCommand(argv []string, root string) bool {
	if len(argv) == 0 {
		return false
	}
	name, args := argv[0], argv[1:]
	switch {
	case slices.Contains(plainSafeCommands, name):
		return true
	case slices.Contains(fileSafeCommands, name):
		return pathArgsStayInRoot(args, root)
	}
	switch name {
	case "grep":
		return !grepRecurses(args) && pathArgsStayInRoot(args, root)
	case "base64":
		return !hasFlag(args, "-o", "--output") && pathArgsStayInRoot(args, root)
	case "find":
		return findIsReadOnly(args) && pathArgsStayInRoot(args, root)
	case "sed":
		return len(args) >= 2 && len(args) <= 3 && args[0] == "-n" && sedPrintArg.MatchString(args[1]) &&
			pathArgsStayInRoot(args[2:], root)
	case "git":
		return gitIsReadOnly(args, root) && !repoConfigRunsFilters(args, root)
	}
	return false
}

func hasFlag(args []string, flags ...string) bool {
	for _, arg := range args {
		for _, flag := range flags {
			if arg == flag || strings.HasPrefix(arg, flag+"=") || (len(flag) == 2 && !strings.HasPrefix(arg, "--") && strings.HasPrefix(arg, flag)) {
				return true
			}
		}
	}
	return false
}

func grepRecurses(args []string) bool {
	for _, arg := range args {
		switch {
		case arg == "--":
			return false
		case strings.HasPrefix(arg, "--"):
			if arg == "--recursive" || arg == "--dereference-recursive" || strings.HasPrefix(arg, "--directories") ||
				strings.HasPrefix(arg, "--include") || strings.HasPrefix(arg, "--exclude") {
				return true
			}
		case strings.HasPrefix(arg, "-") && strings.ContainsAny(arg, "rRd"):
			return true
		}
	}
	return false
}

func findIsReadOnly(args []string) bool {
	unsafe := []string{"-exec", "-execdir", "-ok", "-okdir", "-delete", "-fls", "-fprint", "-fprint0", "-fprintf"}
	for _, arg := range args {
		if slices.Contains(unsafe, arg) {
			return false
		}
	}
	return true
}

// pathArgsStayInRoot treats every argument (and every --flag=value) as a
// possible path: none may leave root or name a denied file.
func pathArgsStayInRoot(args []string, root string) bool {
	for _, arg := range args {
		candidate := arg
		if strings.HasPrefix(arg, "-") {
			_, value, ok := strings.Cut(arg, "=")
			if !ok {
				continue
			}
			candidate = value
		}
		if candidate == "" {
			continue
		}
		if !safeReadPath(root, candidate) {
			return false
		}
	}
	return true
}

func safeReadPath(root, candidate string) bool {
	path := candidate
	switch {
	case filepath.IsAbs(path):
	case filepath.VolumeName(path) != "" || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`):
		// Rooted or drive-relative on Windows: never inside the workspace.
		return false
	default:
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	if !isWithin(root, path) || deniedPath(path) {
		return false
	}
	if _, err := os.Lstat(path); err != nil {
		return true
	}
	resolved, err := filepath.EvalSymlinks(path)
	return err == nil && isWithin(root, resolved) && !deniedPath(resolved)
}

var (
	gitUnsafeFlags = []string{
		"--output", "--ext-diff", "--textconv", "--exec", "--paginate", "--no-index", "--full-diff", "--show-signature",
		"--open-files-in-pager",
	}
	gitDiffCommands = []string{"diff", "log", "show"}
)

// gitIsReadOnly allows status/log/diff/show/branch reads. Every revision or
// path must stay in the workspace and miss the denied list; blob specs
// (rev:path) and pathspec magic ask because they can name any file.
func gitIsReadOnly(args []string, root string) bool {
	index := 0
	for index < len(args) && strings.HasPrefix(args[index], "-") {
		if args[index] != "--no-pager" {
			return false
		}
		index++
	}
	if index >= len(args) {
		return false
	}
	subcommand, rest := args[index], args[index+1:]
	pathsOnly := false
	for _, arg := range rest {
		switch {
		case pathsOnly || !strings.HasPrefix(arg, "-"):
			if strings.Contains(arg, ":") || !safeReadPath(root, arg) {
				return false
			}
		case arg == "--":
			pathsOnly = true
		case slices.Contains(gitUnsafeFlags, arg) || strings.HasPrefix(arg, "--output=") || strings.HasPrefix(arg, "--exec=") ||
			strings.HasPrefix(arg, "-L"):
			return false
		}
	}
	switch subcommand {
	case "status":
		return true
	case "log", "diff", "show":
		_, ok := gitPathStart(rest, root)
		return ok
	case "branch":
		return gitBranchIsReadOnly(rest)
	}
	return false
}

// gitPathStart finds where paths begin in the arguments after a diff/log/show
// subcommand: at "--", or at the first argument naming an existing path.
// Options after such a path are ambiguous to rewrite, so ok is false.
func gitPathStart(rest []string, root string) (start int, ok bool) {
	start = -1
	for index, arg := range rest {
		if arg == "--" {
			return index, start < 0
		}
		if start < 0 && !strings.HasPrefix(arg, "-") {
			if _, err := os.Lstat(filepath.Join(root, arg)); err == nil {
				start = index
			}
		} else if start >= 0 && strings.HasPrefix(arg, "-") {
			return -1, false
		}
	}
	return start, true
}

// repoConfigRunsFilters reports whether repository config (not the user's
// own global config) defines clean/smudge filters, which git diff and
// status run on working-tree files and no flag can switch off.
func repoConfigRunsFilters(args []string, root string) bool {
	if !slices.Contains(args, "diff") && !slices.Contains(args, "status") {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "config", "--null", "--get-regexp", `^filter\..+\.(clean|smudge|process)$`)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	err := cmd.Run()
	var exitError *exec.ExitError
	switch {
	case err == nil:
		return true
	case errors.As(err, &exitError):
		return exitError.ExitCode() != 1 && exitError.ExitCode() != 128
	case errors.Is(err, exec.ErrNotFound):
		return false
	}
	return true
}

var (
	gitHardeningConfig = "-c core.fsmonitor=false -c log.showSignature=false"
	gitDiffHardening   = "--no-ext-diff --no-textconv"
	gitDeniedPathspecs = `':(exclude,glob,icase)**/.env*' ':(exclude,glob,icase)**/*.pem' ` +
		`':(exclude,glob,icase)**/*.key' ':(exclude,glob,icase)**/id_*'`
)

// hardenSafeGit rewrites the git reads that run without approval so repo
// config cannot run programs (fsmonitor, signature checks, textconv and
// external diff drivers) and patches never include denied files.
func hardenSafeGit(script, root string) string {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(script), "")
	if err != nil {
		return script
	}
	type insertion struct {
		offset uint
		text   string
	}
	var inserts []insertion
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		argv := make([]string, len(call.Args))
		for index, word := range call.Args {
			if argv[index], ok = literalWord(word); !ok {
				return true
			}
		}
		if argv[0] != "git" || !isKnownSafeCommand(argv, root) {
			return true
		}
		inserts = append(inserts, insertion{call.Args[0].End().Offset(), " " + gitHardeningConfig})
		sub := 1
		for argv[sub] == "--no-pager" {
			sub++
		}
		if !slices.Contains(gitDiffCommands, argv[sub]) {
			return true
		}
		inserts = append(inserts, insertion{call.Args[sub].End().Offset(), " " + gitDiffHardening})
		if slices.Contains(argv, "--follow") {
			return true
		}
		start, _ := gitPathStart(argv[sub+1:], root)
		separator := " --"
		switch {
		case start >= 0 && argv[sub+1+start] == "--":
			separator = ""
		case start >= 0:
			inserts = append(inserts, insertion{call.Args[sub+1+start].Pos().Offset(), "-- "})
			separator = ""
		}
		inserts = append(inserts, insertion{call.Args[len(call.Args)-1].End().Offset(), separator + " " + gitDeniedPathspecs})
		return true
	})
	slices.SortStableFunc(inserts, func(a, b insertion) int { return int(a.offset) - int(b.offset) })
	for index := len(inserts) - 1; index >= 0; index-- {
		insert := inserts[index]
		script = script[:insert.offset] + insert.text + script[insert.offset:]
	}
	return script
}

func gitBranchIsReadOnly(args []string) bool {
	if len(args) == 0 {
		return true
	}
	for _, arg := range args {
		if !slices.Contains([]string{"--list", "-l", "--show-current", "-a", "--all", "-r", "--remotes", "-v", "-vv", "--verbose"}, arg) &&
			!strings.HasPrefix(arg, "--format=") {
			return false
		}
	}
	return true
}
