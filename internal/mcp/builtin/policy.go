package builtin

// Rules files use the prefix_rule syntax of openai/codex execpolicy
// (codex-rs/execpolicy, Apache-2.0) so policies can be shared between tools.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"go.starlark.net/starlark"
	starsyntax "go.starlark.net/syntax"
)

type Decision int

const (
	DecisionNone Decision = iota
	DecisionAllow
	DecisionPrompt
	DecisionForbidden
)

const defaultRulesFile = "default.rules"

// Rule matches argv whose leading tokens each equal one of the alternatives.
type Rule struct {
	Pattern       [][]string
	Decision      Decision
	Justification string
}

func (r Rule) Matches(argv []string) bool {
	if len(r.Pattern) == 0 || len(argv) < len(r.Pattern) {
		return false
	}
	for index, alternatives := range r.Pattern {
		if !slices.Contains(alternatives, argv[index]) {
			return false
		}
	}
	return true
}

// Policy holds user rules from ~/.rootly-cli/rules plus approvals granted
// for this session. The strictest matching decision wins.
type Policy struct {
	mu      sync.Mutex
	dir     string
	rules   []Rule
	session []Rule
	loadErr error
}

func DefaultRulesDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".rootly-cli", "rules")
}

// LoadPolicy reads every *.rules file in dir. A broken file is skipped and
// reported through LoadError; the built-in defaults still apply.
func LoadPolicy(dir string) *Policy {
	policy := &Policy{dir: dir}
	if dir == "" {
		return policy
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.rules"))
	slices.Sort(paths)
	var errs []error
	for _, path := range paths {
		rules, err := parseRulesFile(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		policy.rules = append(policy.rules, rules...)
	}
	policy.loadErr = errors.Join(errs...)
	return policy
}

func (p *Policy) Dir() string { return p.dir }

func (p *Policy) LoadError() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.loadErr
}

func (p *Policy) RuleCount() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.rules)
}

// Evaluate returns the strictest decision of the rules matching argv.
func (p *Policy) Evaluate(argv []string) (decision Decision, justification string) {
	if p == nil {
		return DecisionNone, ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, rule := range slices.Concat(p.rules, p.session) {
		if rule.Decision > decision && rule.Matches(argv) {
			decision, justification = rule.Decision, rule.Justification
		}
	}
	return decision, justification
}

func (p *Policy) AllowForSession(prefix []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.session = append(p.session, allowRule(prefix))
}

// SaveAllow appends an allow rule for prefix to default.rules.
func (p *Policy) SaveAllow(prefix []string) error {
	if p.dir == "" {
		return errors.New("no home directory for rules")
	}
	if err := os.MkdirAll(p.dir, 0o700); err != nil {
		return err
	}
	quoted := make([]string, len(prefix))
	for index, token := range prefix {
		quoted[index] = starlark.String(token).String()
	}
	line := fmt.Sprintf("prefix_rule(pattern=[%s], decision=\"allow\")\n", strings.Join(quoted, ", "))
	path := filepath.Join(p.dir, defaultRulesFile)
	if existing, err := os.ReadFile(path); err == nil && len(existing) > 0 && existing[len(existing)-1] != '\n' {
		line = "\n" + line
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(line); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rules = append(p.rules, allowRule(prefix))
	return nil
}

func allowRule(prefix []string) Rule {
	pattern := make([][]string, len(prefix))
	for index, token := range prefix {
		pattern[index] = []string{token}
	}
	return Rule{Pattern: pattern, Decision: DecisionAllow}
}

func parseRulesFile(path string) ([]Rule, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rules []Rule
	builtin := starlark.NewBuiltin("prefix_rule", func(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		rule, err := unpackPrefixRule(fn, args, kwargs)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
		return starlark.None, nil
	})
	name := filepath.Base(path)
	thread := &starlark.Thread{Name: name}
	thread.SetMaxExecutionSteps(1_000_000)
	if _, err := starlark.ExecFileOptions(&starsyntax.FileOptions{}, thread, name, source, starlark.StringDict{"prefix_rule": builtin}); err != nil {
		var evalErr *starlark.EvalError
		if errors.As(err, &evalErr) {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		return nil, err
	}
	return rules, nil
}

func unpackPrefixRule(fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (Rule, error) {
	var (
		pattern       *starlark.List
		decision      = "allow"
		justification string
		match         *starlark.List
		notMatch      *starlark.List
	)
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"pattern", &pattern, "decision?", &decision, "justification?", &justification,
		"match?", &match, "not_match?", &notMatch); err != nil {
		return Rule{}, err
	}
	rule := Rule{Justification: justification}
	switch decision {
	case "allow":
		rule.Decision = DecisionAllow
	case "prompt":
		rule.Decision = DecisionPrompt
	case "forbidden":
		rule.Decision = DecisionForbidden
	default:
		return Rule{}, fmt.Errorf("prefix_rule: decision must be allow, prompt or forbidden, got %q", decision)
	}
	if pattern.Len() == 0 {
		return Rule{}, errors.New("prefix_rule: pattern must not be empty")
	}
	for index := range pattern.Len() {
		alternatives, err := tokenAlternatives(pattern.Index(index))
		if err != nil {
			return Rule{}, fmt.Errorf("prefix_rule: pattern[%d]: %w", index, err)
		}
		rule.Pattern = append(rule.Pattern, alternatives)
	}
	if err := checkExamples(rule, match, true); err != nil {
		return Rule{}, err
	}
	if err := checkExamples(rule, notMatch, false); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

func tokenAlternatives(value starlark.Value) ([]string, error) {
	switch value := value.(type) {
	case starlark.String:
		return []string{string(value)}, nil
	case *starlark.List:
		alternatives, err := stringList(value)
		if err == nil && len(alternatives) == 0 {
			err = errors.New("alternatives must not be empty")
		}
		return alternatives, err
	}
	return nil, fmt.Errorf("want a string or a list of strings, got %s", value.Type())
}

func stringList(list *starlark.List) ([]string, error) {
	values := make([]string, 0, list.Len())
	for index := range list.Len() {
		text, ok := starlark.AsString(list.Index(index))
		if !ok {
			return nil, fmt.Errorf("want strings, got %s", list.Index(index).Type())
		}
		values = append(values, text)
	}
	return values, nil
}

func checkExamples(rule Rule, examples *starlark.List, want bool) error {
	if examples == nil {
		return nil
	}
	label := map[bool]string{true: "match", false: "not_match"}[want]
	for index := range examples.Len() {
		var argv []string
		switch example := examples.Index(index).(type) {
		case starlark.String:
			commands, ok := shellCommands(string(example))
			if !ok || len(commands) != 1 {
				return fmt.Errorf("prefix_rule: %s[%d]: not a plain command", label, index)
			}
			argv = commands[0]
		case *starlark.List:
			var err error
			if argv, err = stringList(example); err != nil {
				return fmt.Errorf("prefix_rule: %s[%d]: %w", label, index, err)
			}
		default:
			return fmt.Errorf("prefix_rule: %s[%d]: want a string or a list of strings", label, index)
		}
		if rule.Matches(argv) != want {
			return fmt.Errorf("prefix_rule: %s example %q does not hold", label, strings.Join(argv, " "))
		}
	}
	return nil
}
