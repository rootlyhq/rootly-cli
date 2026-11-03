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
	"unicode"

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
	root = resolveRoot(root)
	name, args := argv[0], argv[1:]
	switch {
	case slices.Contains(plainSafeCommands, name):
		return true
	case slices.Contains(fileSafeCommands, name):
		return fileCommandOnlyReads(name, args) && pathArgsStayInRoot(args, root)
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

// fileCommandOnlyReads rejects the operands and flags of fileSafeCommands
// that write a file or read paths from a file: uniq's second operand is its
// output, and wc --files0-from reads names that pathArgsStayInRoot never sees.
func fileCommandOnlyReads(name string, args []string) bool {
	switch name {
	case "uniq":
		return uniqOperands(args) <= 1
	case "wc":
		return !hasFlag(args, "--files0-from")
	}
	return true
}

func uniqOperands(args []string) int {
	operands := 0
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--":
			return operands + len(args) - index - 1
		case arg == "-f" || arg == "-s" || arg == "-w":
			index++
		case arg == "-" || !strings.HasPrefix(arg, "-"):
			operands++
		}
	}
	return operands
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
	unsafe := []string{"-exec", "-execdir", "-ok", "-okdir", "-delete", "-fls", "-fprint", "-fprint0", "-fprintf", "-files0-from"}
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

// safeReadPath reports whether candidate stays in the workspace and misses
// the denied list. An existing path is judged by where it really points, so
// a symlink can't escape and an alias of root (macOS /var -> /private/var)
// still counts as inside.
func safeReadPath(root, candidate string) bool {
	root = resolveRoot(root)
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
	if deniedPath(path) {
		return false
	}
	if _, err := os.Lstat(path); err != nil {
		return isWithin(root, path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	return err == nil && isWithin(root, resolved) && !deniedPath(resolved)
}

// resolveRoot returns the workspace root as an absolute path with symlinks
// resolved, so it compares equal to resolved candidates.
func resolveRoot(root string) string {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return filepath.Clean(root)
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return resolved
	}
	return absolute
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
// subcommand: at "--", or at the first positional git doesn't resolve to a
// commit. ok is false (ask) when a positional names a blob or tree, is both
// a commit and an existing path with no "--", or can't be classified, and
// when options follow a path, which is ambiguous to rewrite.
func gitPathStart(rest []string, root string) (start int, ok bool) {
	start = -1
	for index := 0; index < len(rest); index++ {
		arg := rest[index]
		switch {
		case arg == "--":
			return index, start < 0
		case start >= 0:
			if strings.HasPrefix(arg, "-") {
				return -1, false
			}
		case strings.HasPrefix(arg, "-"):
			if slices.Contains(gitValueFlags, arg) {
				index++
			}
		default:
			switch gitClassifyRev(root, arg) {
			case gitRevCommit:
				if _, err := os.Lstat(filepath.Join(root, arg)); err == nil && !slices.Contains(rest[index:], "--") {
					return -1, false
				}
			case gitRevPath:
				start = index
			default:
				return -1, false
			}
		}
	}
	return start, true
}

// gitValueFlags take their value as the next argument, which is neither a
// revision nor a path.
var gitValueFlags = []string{
	"-n", "--max-count", "--skip", "--since", "--after", "--until", "--before", "--author",
	"--committer", "--grep", "--format", "--pretty", "--date", "--diff-filter", "-G", "-S",
}

type gitRevKind int

const (
	gitRevPath gitRevKind = iota
	gitRevCommit
	gitRevContent
	gitRevUnknown
)

// gitClassifyRev asks git what a positional means: a commit (a range or a
// revision set), a blob or tree object, whose content pathspec excludes
// can't filter, a path git knows, or unknown, which asks rather than guess.
func gitClassifyRev(root, arg string) gitRevKind {
	sides := []string{arg}
	isRange := strings.Contains(arg, "..")
	if isRange {
		sides = strings.Split(strings.ReplaceAll(arg, "...", ".."), "..")
	}
	for _, side := range sides {
		if side == "" {
			continue
		}
		if base := gitRevSetSuffix.ReplaceAllString(side, ""); base != side {
			if base == "" || isRange {
				return gitRevUnknown
			}
			side = base
		}
		ok, err := gitSucceeds(root, "rev-parse", "--verify", "--quiet", "--end-of-options", side+"^{commit}")
		switch {
		case err != nil:
			return gitRevUnknown
		case ok:
			continue
		}
		output, err := gitOutput(root, "cat-file", "-t", side+"^{}")
		switch {
		case err != nil:
			return gitRevUnknown
		case output == "blob" || output == "tree":
			return gitRevContent
		case isRange || !gitKnowsPath(root, side):
			return gitRevUnknown
		}
		return gitRevPath
	}
	return gitRevCommit
}

// gitRevSetSuffix matches the revision-set notations HEAD^!, HEAD^@ and
// HEAD^-N, which name commits but don't peel with ^{commit}.
var gitRevSetSuffix = regexp.MustCompile(`\^(!|@|-\d*)$`)

// gitKnowsPath reports whether arg is clearly a path: it exists in the
// worktree or git tracks it (e.g. a deleted file not yet committed).
func gitKnowsPath(root, arg string) bool {
	if _, err := os.Lstat(filepath.Join(root, arg)); err == nil {
		return true
	}
	ok, err := gitSucceeds(root, "ls-files", "--error-unmatch", "--", ":(literal)"+arg)
	return err == nil && ok
}

// gitSucceeds runs a hardened git lookup in root. ok is false when git exits
// non-zero; err is set when git can't give an answer (killed, timed out).
func gitSucceeds(root string, args ...string) (ok bool, err error) {
	_, ok, err = runGitLookup(root, args)
	return ok, err
}

func gitOutput(root string, args ...string) (string, error) {
	output, _, err := runGitLookup(root, args)
	return output, err
}

func runGitLookup(root string, args []string) (output string, ok bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.fsmonitor=false"}, args...)...)
	cmd.Dir = root
	raw, runErr := cmd.Output()
	var exitError *exec.ExitError
	switch {
	case runErr == nil:
		return strings.TrimSpace(string(raw)), true, nil
	case errors.Is(runErr, exec.ErrNotFound):
		return "", false, nil
	case errors.As(runErr, &exitError) && exitError.ExitCode() > 0:
		return "", false, nil
	}
	return "", false, runErr
}

// repoConfigRunsFilters reports whether repository or worktree config (not
// the user's own global or system config) defines clean/smudge filters, which git diff and
// status run on working-tree files and no flag can switch off.
func repoConfigRunsFilters(args []string, root string) bool {
	if !slices.Contains(args, "diff") && !slices.Contains(args, "status") {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "config", "--includes", "--null", "--get-regexp", `^filter\..+\.(clean|smudge|process)$`)
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
	root = resolveRoot(root)
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

// scriptForbidden matches forbidden rules against every command a script
// could run, whatever its redirects, expansions or nesting: each simple
// command in the AST, the command behind wrappers such as sudo or env, and
// bash -c / eval bodies. A script that doesn't parse is matched token by
// token. It returns the justification of the first forbidden match.
func scriptForbidden(policy *Policy, script string) string {
	for _, argv := range forbiddenCandidates(script, 0) {
		if decision, justification := policy.Evaluate(argv); decision == DecisionForbidden {
			if justification == "" {
				justification = strings.Join(argv, " ")
			}
			return justification
		}
	}
	return ""
}

func forbiddenCandidates(script string, depth int) [][]string {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(script), "")
	if err != nil || depth > 3 {
		return tokenSuffixes(strings.FieldsFunc(script, func(r rune) bool {
			return unicode.IsSpace(r) || strings.ContainsRune(";|&()<>`'\"${}\\", r)
		}))
	}
	var candidates [][]string
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		argv := make([]string, len(call.Args))
		for index, word := range call.Args {
			argv[index] = looseWord(word)
		}
		candidates = append(candidates, commandCandidates(argv, depth)...)
		return true
	})
	return candidates
}

// looseWord renders the literal parts of a word; anything dynamic becomes a
// placeholder no rule matches.
func looseWord(word *syntax.Word) string {
	var value strings.Builder
	for _, part := range word.Parts {
		switch part := part.(type) {
		case *syntax.Lit:
			value.WriteString(part.Value)
		case *syntax.SglQuoted:
			value.WriteString(part.Value)
		case *syntax.DblQuoted:
			for _, inner := range part.Parts {
				if lit, ok := inner.(*syntax.Lit); ok {
					value.WriteString(lit.Value)
				} else {
					value.WriteString("\x00")
				}
			}
		default:
			value.WriteString("\x00")
		}
	}
	return value.String()
}

var shellInterpreters = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true}

func commandCandidates(argv []string, depth int) [][]string {
	if len(argv) == 0 {
		return nil
	}
	candidates := [][]string{argv}
	name := filepath.Base(argv[0])
	if name != argv[0] {
		candidates = append(candidates, append([]string{name}, argv[1:]...))
	}
	switch {
	case name == "eval":
		candidates = append(candidates, forbiddenCandidates(strings.Join(argv[1:], " "), depth+1)...)
	case shellInterpreters[name]:
		for index, arg := range argv[1:] {
			if strings.HasPrefix(arg, "-") && strings.Contains(arg, "c") && index+2 < len(argv) {
				candidates = append(candidates, forbiddenCandidates(argv[index+2], depth+1)...)
				break
			}
		}
	case unrememberableCommands[name]:
		for index := 1; index < len(argv); index++ {
			candidates = append(candidates, commandCandidates(argv[index:], depth)...)
		}
	}
	return candidates
}

func tokenSuffixes(tokens []string) [][]string {
	suffixes := make([][]string, 0, len(tokens)*2)
	for index := range tokens {
		suffixes = append(suffixes, tokens[index:])
		if base := filepath.Base(tokens[index]); base != tokens[index] {
			suffixes = append(suffixes, append([]string{base}, tokens[index+1:]...))
		}
	}
	return suffixes
}
