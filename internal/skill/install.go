package skill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Agent is an install target: the coding agent whose convention decides
// where the skill is written and in what shape.
type Agent string

// The supported install targets. AgentAll expands to every target that
// writes a distinct path.
const (
	AgentClaude  Agent = "claude"
	AgentCursor  Agent = "cursor"
	AgentCopilot Agent = "copilot"
	AgentCodex   Agent = "codex"
	AgentGeneric Agent = "generic"
	AgentAll     Agent = "all"
)

// Agents lists the individual targets in install order. AgentGeneric is
// omitted because it writes the same AGENTS.md block as AgentCodex.
var Agents = []Agent{AgentClaude, AgentCursor, AgentCopilot, AgentCodex}

// AgentNames lists every accepted --agent value.
func AgentNames() []string {
	return []string{
		string(AgentClaude), string(AgentCursor), string(AgentCopilot),
		string(AgentCodex), string(AgentGeneric), string(AgentAll),
	}
}

// ParseAgent validates an --agent value.
func ParseAgent(s string) (Agent, error) {
	a := Agent(strings.ToLower(strings.TrimSpace(s)))
	switch a {
	case AgentClaude, AgentCursor, AgentCopilot, AgentCodex, AgentGeneric, AgentAll:
		return a, nil
	}
	return "", fmt.Errorf("unknown agent %q, want one of %s", s, strings.Join(AgentNames(), ", "))
}

// The operations an action reports.
const (
	OpWrite        = "write"
	OpReplace      = "replace"
	OpAppendBlock  = "append-block"
	OpReplaceBlock = "replace-block"
	OpRemove       = "remove"
	OpRemoveBlock  = "remove-block"
	OpUnchanged    = "unchanged"
	OpSkip         = "skip"
	OpAbsent       = "absent"
)

// Action is one file change an install or uninstall made, or would make
// under --dry-run.
type Action struct {
	Agent Agent  `json:"agent"`
	Path  string `json:"path"`
	Op    string `json:"op"`
	// Bytes is the size of the figctl content: the whole file for a file
	// figctl owns, or the marked block for a shared instruction file.
	Bytes  int    `json:"bytes"`
	Reason string `json:"reason,omitempty"`
}

// Written reports whether the action changed the file on disk (or would
// have, under --dry-run).
func (a Action) Written() bool {
	switch a.Op {
	case OpWrite, OpReplace, OpAppendBlock, OpReplaceBlock, OpRemove, OpRemoveBlock:
		return true
	}
	return false
}

// Options describes one install or uninstall.
type Options struct {
	// Agent is the target, possibly AgentAll.
	Agent Agent
	// Home is the user home directory the user level Claude skill goes
	// under. Defaults to the process home directory.
	Home string
	// Dir is the project directory the project scoped targets go under.
	// Defaults to the working directory.
	Dir string
	// Project installs the Claude skill into the repository instead of
	// the user home. The other targets are project scoped either way.
	Project bool
	// Force replaces a file figctl does not recognize as its own.
	Force bool
	// DryRun plans the actions without touching the filesystem.
	DryRun bool
}

// Directory and file permissions for everything the installer creates.
const (
	dirPerm  os.FileMode = 0o755
	filePerm os.FileMode = 0o644
)

// kind distinguishes a file figctl owns outright from a marked block
// inside a file that belongs to the project.
type kind int

const (
	kindDirectory kind = iota
	kindFile
	kindBlock
)

// target is one resolved install location.
type target struct {
	agent Agent
	kind  kind
	// root is the directory every write is confined to.
	root string
	// path is the file written for kindFile and kindBlock.
	path string
	// content is the file body for kindFile, or the block body for
	// kindBlock, without the block delimiters.
	content string
}

func (o Options) home() (string, error) {
	if o.Home != "" {
		return o.Home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating the home directory: %w", err)
	}
	return home, nil
}

func (o Options) dir() (string, error) {
	if o.Dir != "" {
		return o.Dir, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("locating the working directory: %w", err)
	}
	return cwd, nil
}

// expand turns AgentAll into the list of targets to act on.
func expand(a Agent) []Agent {
	if a == AgentAll {
		return append([]Agent(nil), Agents...)
	}
	return []Agent{a}
}

// resolveTargets builds the install locations for the requested agents.
func resolveTargets(opts Options) ([]target, error) {
	var out []target
	for _, agent := range expand(opts.Agent) {
		t, err := resolveTarget(agent, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func resolveTarget(agent Agent, opts Options) (target, error) {
	switch agent {
	case AgentClaude:
		base := ""
		if opts.Project {
			dir, err := opts.dir()
			if err != nil {
				return target{}, err
			}
			base = dir
		} else {
			home, err := opts.home()
			if err != nil {
				return target{}, err
			}
			base = home
		}
		return target{agent: agent, kind: kindDirectory, root: filepath.Join(base, ".claude", "skills", "figctl")}, nil
	case AgentCursor:
		dir, err := opts.dir()
		if err != nil {
			return target{}, err
		}
		root := filepath.Join(dir, ".cursor", "rules")
		return target{agent: agent, kind: kindFile, root: root, path: filepath.Join(root, "figctl.mdc"), content: cursorRule()}, nil
	case AgentCopilot:
		dir, err := opts.dir()
		if err != nil {
			return target{}, err
		}
		root := filepath.Join(dir, ".github")
		return target{agent: agent, kind: kindBlock, root: root, path: filepath.Join(root, "copilot-instructions.md"), content: portableBody()}, nil
	case AgentCodex, AgentGeneric:
		dir, err := opts.dir()
		if err != nil {
			return target{}, err
		}
		return target{agent: agent, kind: kindBlock, root: dir, path: filepath.Join(dir, "AGENTS.md"), content: portableBody()}, nil
	}
	return target{}, fmt.Errorf("unknown agent %q", agent)
}

// Install writes the skill for the requested agents and returns what it
// did, one action per file. It is idempotent: a file that already holds
// the current content is reported as unchanged and left alone.
func Install(opts Options) ([]Action, error) {
	targets, err := resolveTargets(opts)
	if err != nil {
		return nil, err
	}
	var actions []Action
	for _, t := range targets {
		got, err := installTarget(t, opts)
		if err != nil {
			return nil, err
		}
		actions = append(actions, got...)
	}
	return actions, nil
}

func installTarget(t target, opts Options) ([]Action, error) {
	switch t.kind {
	case kindDirectory:
		var actions []Action
		for _, f := range files {
			path, err := safeJoin(t.root, f.Path)
			if err != nil {
				return nil, err
			}
			action, err := writeOwned(t.agent, path, f.Content(), opts)
			if err != nil {
				return nil, err
			}
			actions = append(actions, action)
		}
		return actions, nil
	case kindFile:
		if _, err := safeJoin(t.root, filepath.Base(t.path)); err != nil {
			return nil, err
		}
		action, err := writeOwned(t.agent, t.path, t.content, opts)
		if err != nil {
			return nil, err
		}
		return []Action{action}, nil
	case kindBlock:
		action, err := writeBlock(t.agent, t.path, t.content, opts)
		if err != nil {
			return nil, err
		}
		return []Action{action}, nil
	}
	return nil, fmt.Errorf("unknown target kind for agent %q", t.agent)
}

// writeOwned writes a file figctl owns. An existing file that does not
// carry the marker is left alone unless --force, so a hand written
// document is never clobbered.
func writeOwned(agent Agent, path, content string, opts Options) (Action, error) {
	action := Action{Agent: agent, Path: path, Bytes: len(content), Op: OpWrite}
	existing, found, err := readFile(path)
	if err != nil {
		return Action{}, err
	}
	switch {
	case !found:
		action.Op = OpWrite
	case existing == content:
		action.Op = OpUnchanged
		return action, nil
	case strings.Contains(existing, Marker) || opts.Force:
		action.Op = OpReplace
	default:
		action.Op = OpSkip
		action.Reason = "the file exists and was not written by figctl; pass --force to replace it"
		return action, nil
	}
	if opts.DryRun {
		return action, nil
	}
	if err := writeFile(path, content); err != nil {
		return Action{}, err
	}
	return action, nil
}

// writeBlock adds or replaces the figctl block in a file the project
// owns, leaving every other line of it untouched.
func writeBlock(agent Agent, path, body string, opts Options) (Action, error) {
	block := blockContent(body)
	action := Action{Agent: agent, Path: path, Bytes: len(block)}
	existing, found, err := readFile(path)
	if err != nil {
		return Action{}, err
	}
	var next string
	switch {
	case !found || strings.TrimSpace(existing) == "":
		action.Op = OpAppendBlock
		next = block
	case hasBlock(existing):
		replaced, err := replaceBlock(existing, block)
		if err != nil {
			return Action{}, err
		}
		if replaced == existing {
			action.Op = OpUnchanged
			return action, nil
		}
		action.Op = OpReplaceBlock
		next = replaced
	default:
		action.Op = OpAppendBlock
		next = strings.TrimRight(existing, "\n") + "\n\n" + block
	}
	if opts.DryRun {
		return action, nil
	}
	if err := writeFile(path, next); err != nil {
		return Action{}, err
	}
	return action, nil
}

// Uninstall removes the skill for the requested agents: the files figctl
// owns, and the marked block from files the project owns.
func Uninstall(opts Options) ([]Action, error) {
	targets, err := resolveTargets(opts)
	if err != nil {
		return nil, err
	}
	var actions []Action
	for _, t := range targets {
		got, err := uninstallTarget(t, opts)
		if err != nil {
			return nil, err
		}
		actions = append(actions, got...)
	}
	return actions, nil
}

func uninstallTarget(t target, opts Options) ([]Action, error) {
	switch t.kind {
	case kindDirectory:
		var actions []Action
		for _, f := range files {
			path, err := safeJoin(t.root, f.Path)
			if err != nil {
				return nil, err
			}
			action, err := removeOwned(t.agent, path, opts)
			if err != nil {
				return nil, err
			}
			actions = append(actions, action)
		}
		if !opts.DryRun {
			pruneEmpty(t.root, filepath.Join(t.root, "reference"))
		}
		return actions, nil
	case kindFile:
		action, err := removeOwned(t.agent, t.path, opts)
		if err != nil {
			return nil, err
		}
		return []Action{action}, nil
	case kindBlock:
		action, err := removeBlock(t.agent, t.path, opts)
		if err != nil {
			return nil, err
		}
		return []Action{action}, nil
	}
	return nil, fmt.Errorf("unknown target kind for agent %q", t.agent)
}

func removeOwned(agent Agent, path string, opts Options) (Action, error) {
	action := Action{Agent: agent, Path: path, Op: OpRemove}
	existing, found, err := readFile(path)
	if err != nil {
		return Action{}, err
	}
	if !found {
		action.Op = OpAbsent
		return action, nil
	}
	action.Bytes = len(existing)
	if !strings.Contains(existing, Marker) && !opts.Force {
		action.Op = OpSkip
		action.Reason = "the file was not written by figctl; pass --force to remove it"
		return action, nil
	}
	if opts.DryRun {
		return action, nil
	}
	if err := os.Remove(path); err != nil {
		return Action{}, fmt.Errorf("removing %s: %w", path, err)
	}
	return action, nil
}

func removeBlock(agent Agent, path string, opts Options) (Action, error) {
	action := Action{Agent: agent, Path: path, Op: OpRemoveBlock}
	existing, found, err := readFile(path)
	if err != nil {
		return Action{}, err
	}
	if !found || !hasBlock(existing) {
		action.Op = OpAbsent
		return action, nil
	}
	next, err := replaceBlock(existing, "")
	if err != nil {
		return Action{}, err
	}
	action.Bytes = len(existing) - len(next)
	if opts.DryRun {
		return action, nil
	}
	if strings.TrimSpace(next) == "" {
		if err := os.Remove(path); err != nil {
			return Action{}, fmt.Errorf("removing %s: %w", path, err)
		}
		action.Op = OpRemove
		return action, nil
	}
	if err := writeFile(path, next); err != nil {
		return Action{}, err
	}
	return action, nil
}

// hasBlock reports whether the content carries a complete figctl block.
func hasBlock(content string) bool {
	begin := strings.Index(content, BlockBegin)
	return begin >= 0 && strings.Contains(content[begin:], BlockEnd)
}

// replaceBlock swaps the figctl block for the replacement, keeping every
// line outside it. An empty replacement removes the block.
func replaceBlock(content, replacement string) (string, error) {
	begin := strings.Index(content, BlockBegin)
	if begin < 0 {
		return "", errors.New("no figctl block to replace")
	}
	rest := content[begin:]
	end := strings.Index(rest, BlockEnd)
	if end < 0 {
		return "", errors.New("figctl block is not closed")
	}
	tail := rest[end+len(BlockEnd):]
	head := content[:begin]
	if replacement == "" {
		head = strings.TrimRight(head, " \t")
		tail = strings.TrimLeft(tail, " \t")
		joined := strings.TrimRight(head, "\n")
		trimmedTail := strings.TrimLeft(tail, "\n")
		if joined != "" && trimmedTail != "" {
			return joined + "\n\n" + trimmedTail, nil
		}
		if trimmedTail != "" {
			return trimmedTail, nil
		}
		if joined != "" {
			return joined + "\n", nil
		}
		return "", nil
	}
	return head + strings.TrimRight(replacement, "\n") + "\n" + strings.TrimLeft(tail, "\n"), nil
}

// safeJoin resolves a relative skill path under root and refuses
// anything that would escape it.
func safeJoin(root, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("empty skill path under %s", root)
	}
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return "", fmt.Errorf("refusing to write absolute skill path %q", rel)
	}
	clean := filepath.Clean(root)
	joined := filepath.Clean(filepath.Join(clean, filepath.FromSlash(rel)))
	if joined != clean && !strings.HasPrefix(joined, clean+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing to write %q outside %s", rel, clean)
	}
	return joined, nil
}

func readFile(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reading %s: %w", path, err)
	}
	return string(data), true, nil
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), filePerm); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// pruneEmpty removes the given directories, deepest first, when they are
// empty. Anything else is left in place.
func pruneEmpty(dirs ...string) {
	sorted := append([]string(nil), dirs...)
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i]) > len(sorted[j]) })
	for _, dir := range sorted {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			continue
		}
		_ = os.Remove(dir)
	}
}

// cursorRule renders the Cursor rule file: the .mdc front matter Cursor
// expects, then the skill body.
func cursorRule() string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("description: " + Description + "\n")
	b.WriteString("alwaysApply: false\n")
	b.WriteString("---\n\n")
	b.WriteString(Marker + "\n\n")
	b.WriteString(portableBody())
	return b.String()
}

// portableBody is the skill body for an agent that reads one file: the
// routing table is replaced by the commands that print the reference
// documents, since there is no reference directory next to it.
func portableBody() string {
	body := Body()
	if cut := strings.Index(body, "\n## Reference\n"); cut >= 0 {
		body = body[:cut+1]
	}
	return strings.TrimRight(body, "\n") + "\n" + portableReference
}

// portableReference routes to the reference documents through the CLI.
const portableReference = `
## Reference

| Question | Command |
| --- | --- |
| Which command, which flag, what does it do? | ` + "`figctl skill print --file commands`" + ` |
| What fields does the output have? | ` + "`figctl skill print --file schemas`" + ` |
| How do I do a whole task end to end? | ` + "`figctl skill print --file workflows`" + ` |

` + "`figctl <cmd> --help`" + ` carries the same flags and examples as the command
reference, and ` + "`figctl schema <command>`" + ` prints the JSON Schema of any
command's data payload.
`

// blockContent wraps a body in the figctl block delimiters.
func blockContent(body string) string {
	return BlockBegin + "\n" + strings.TrimRight(body, "\n") + "\n" + BlockEnd + "\n"
}

// Render returns a document as the given agent receives it: the file as
// written for claude, the Cursor rule for cursor, and the marked block
// for the agents that share an instruction file. Reference documents are
// the same for every agent.
func Render(agent Agent, name string) (string, error) {
	f, ok := Lookup(name)
	if !ok {
		return "", fmt.Errorf("unknown skill file %q, want one of %s", name, strings.Join(FileNames(), ", "))
	}
	if f.Name != "SKILL" {
		return f.Content(), nil
	}
	switch agent {
	case AgentClaude:
		return f.Content(), nil
	case AgentCursor:
		return cursorRule(), nil
	case AgentCopilot, AgentCodex, AgentGeneric:
		return blockContent(portableBody()), nil
	case AgentAll:
		return "", errors.New("skill print takes one agent, not all")
	}
	return "", fmt.Errorf("unknown agent %q", agent)
}

// Location returns the path a document is installed at, relative to the
// skill directory for claude and to the project root for every other
// agent.
func Location(agent Agent, name string) string {
	f, ok := Lookup(name)
	if !ok {
		return ""
	}
	if f.Name != "SKILL" || agent == AgentClaude {
		return f.Path
	}
	switch agent {
	case AgentCursor:
		return ".cursor/rules/figctl.mdc"
	case AgentCopilot:
		return ".github/copilot-instructions.md"
	case AgentCodex, AgentGeneric:
		return "AGENTS.md"
	}
	return f.Path
}
