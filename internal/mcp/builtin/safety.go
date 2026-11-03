package builtin

// The read-only command rules below follow google-gemini/gemini-cli
// (packages/core/src/sandbox/utils/commandSafety.ts, Apache-2.0,
// Copyright 2025 Google LLC), tightened for Rootly: no recursion into
// denied files, no git global options, and no shell expansion at all.

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

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
		return gitIsReadOnly(args)
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
	if !filepath.IsAbs(path) {
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

func gitIsReadOnly(args []string) bool {
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
	for _, arg := range rest {
		if arg == "--" {
			break
		}
		if slices.Contains([]string{"--output", "--ext-diff", "--textconv", "--exec", "--paginate", "--no-index"}, arg) ||
			strings.HasPrefix(arg, "--output=") || strings.HasPrefix(arg, "--exec=") {
			return false
		}
	}
	switch subcommand {
	case "status", "log", "diff", "show":
		return true
	case "branch":
		return gitBranchIsReadOnly(rest)
	}
	return false
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
