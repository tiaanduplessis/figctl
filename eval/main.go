// Command eval runs the figctl agent evaluation harness: a set of tasks
// executed by a real coding agent against a real Figma file, scored on
// the number of figctl calls, wall time, and verifiable checks on the
// files the agent produced.
//
// It needs a Figma token, a Figma file, and the claude binary, so it
// cannot run in CI. See eval/README.md.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Task is one evaluation task, loaded from eval/tasks/*.yaml.
type Task struct {
	Name           string  `yaml:"name"`
	Description    string  `yaml:"description"`
	Node           string  `yaml:"node"`
	Prompt         string  `yaml:"prompt"`
	TimeoutSeconds int     `yaml:"timeoutSeconds"`
	Checks         []Check `yaml:"checks"`

	path string
}

// Check is one verifiable assertion about the result of a task.
type Check struct {
	Type    string   `yaml:"type"`
	Path    string   `yaml:"path"`
	Values  []string `yaml:"values"`
	Pattern string   `yaml:"pattern"`
	Value   int      `yaml:"value"`
	Reason  string   `yaml:"reason"`
}

// CheckResult records whether one check passed.
type CheckResult struct {
	Check  string `json:"check"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

// Result is the score of one task run.
type Result struct {
	Task        string        `json:"task"`
	Workdir     string        `json:"workdir"`
	Transcript  string        `json:"transcript"`
	ToolCalls   int           `json:"toolCalls"`
	FigctlCalls int           `json:"figctlCalls"`
	Commands    []string      `json:"commands"`
	SecondsUsed float64       `json:"secondsUsed"`
	AgentError  string        `json:"agentError,omitempty"`
	Checks      []CheckResult `json:"checks"`
	Passed      bool          `json:"passed"`
}

func main() {
	tasksDir := flag.String("tasks", "eval/tasks", "directory of task definitions")
	outDir := flag.String("out", "eval/results", "directory results are written to")
	only := flag.String("task", "", "run only the task with this name")
	agentBin := flag.String("agent", "claude", "agent binary to run the prompt with")
	agentArgs := flag.String("agent-args", defaultAgentArgs, "arguments passed to the agent before the prompt")
	keep := flag.Bool("keep", false, "keep the working directory of every task")
	flag.Parse()

	if err := run(*tasksDir, *outDir, *only, *agentBin, strings.Fields(*agentArgs), *keep); err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}
}

// defaultAgentArgs runs Claude Code non-interactively and streams the
// transcript as JSON lines so tool calls can be counted.
const defaultAgentArgs = "-p --output-format stream-json --verbose --permission-mode acceptEdits"

// requiredEnv names the variables a run needs.
var requiredEnv = []string{"FIGCTL_EVAL_URL"}

func run(tasksDir, outDir, only, agentBin string, agentArgs []string, keep bool) error {
	for _, name := range requiredEnv {
		if os.Getenv(name) == "" {
			return fmt.Errorf("%s is not set; see eval/README.md", name)
		}
	}
	if _, err := exec.LookPath(agentBin); err != nil {
		return fmt.Errorf("agent binary %q not found in PATH: %w", agentBin, err)
	}
	if _, err := exec.LookPath("figctl"); err != nil {
		return fmt.Errorf("figctl not found in PATH; run make build and put bin/figctl on PATH: %w", err)
	}

	tasks, err := loadTasks(tasksDir, only)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		return fmt.Errorf("no tasks in %s", tasksDir)
	}

	stamp := time.Now().UTC().Format("20060102-150405")
	runDir := filepath.Join(outDir, stamp)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}

	results := make([]Result, 0, len(tasks))
	for _, task := range tasks {
		fmt.Printf("== %s: %s\n", task.Name, task.Description)
		result, err := runTask(task, runDir, agentBin, agentArgs, keep)
		if err != nil {
			return err
		}
		results = append(results, result)
		report(result)
	}

	summary := filepath.Join(runDir, "results.json")
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(summary, append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("\nwrote %s\n", summary)

	failed := 0
	for _, r := range results {
		if !r.Passed {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d tasks failed", failed, len(results))
	}
	return nil
}

func loadTasks(dir, only string) ([]Task, error) {
	entries, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)
	var tasks []Task
	for _, path := range entries {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var task Task
		if err := yaml.Unmarshal(data, &task); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		task.path = path
		if task.Name == "" {
			return nil, fmt.Errorf("%s: the task has no name", path)
		}
		if only != "" && task.Name != only {
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// runTask runs one task in a fresh working directory and scores it.
func runTask(task Task, runDir, agentBin string, agentArgs []string, keep bool) (Result, error) {
	workdir, err := os.MkdirTemp("", "figctl-eval-"+task.Name+"-")
	if err != nil {
		return Result{}, err
	}
	if !keep {
		defer func() { _ = os.RemoveAll(workdir) }()
	}
	transcript := filepath.Join(runDir, task.Name+".jsonl")

	timeout := time.Duration(task.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	prompt := expand(task.Prompt)
	args := append(append([]string{}, agentArgs...), prompt)
	cmd := exec.CommandContext(ctx, agentBin, args...)
	cmd.Dir = workdir
	cmd.Stderr = os.Stderr
	out, err := os.Create(transcript)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = out.Close() }()
	cmd.Stdout = out

	started := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(started)
	if closeErr := out.Close(); closeErr != nil {
		return Result{}, closeErr
	}

	result := Result{
		Task:        task.Name,
		Workdir:     workdir,
		Transcript:  transcript,
		SecondsUsed: elapsed.Round(time.Millisecond).Seconds(),
	}
	if runErr != nil {
		result.AgentError = runErr.Error()
	}
	tools, commands, err := countToolCalls(transcript)
	if err != nil {
		return Result{}, err
	}
	result.ToolCalls = tools
	result.Commands = commands
	for _, c := range commands {
		if isFigctl(c) {
			result.FigctlCalls++
		}
	}
	result.Checks = check(task, workdir, result)
	result.Passed = runErr == nil
	for _, c := range result.Checks {
		if !c.Passed {
			result.Passed = false
		}
	}
	return result, nil
}

// expand substitutes ${VAR} from the environment in a prompt.
func expand(s string) string {
	return os.Expand(s, os.Getenv)
}

// isFigctl reports whether a shell command invokes figctl.
func isFigctl(command string) bool {
	for _, part := range strings.FieldsFunc(command, func(r rune) bool {
		return r == '|' || r == ';' || r == '&' || r == '\n'
	}) {
		fields := strings.Fields(part)
		for _, f := range fields {
			if f == "figctl" || strings.HasSuffix(f, "/figctl") {
				return true
			}
			if strings.Contains(f, "=") {
				continue
			}
			break
		}
	}
	return false
}

// streamEvent is the subset of a stream-json transcript line the harness
// reads: the assistant messages and the tool calls inside them.
type streamEvent struct {
	Type    string `json:"type"`
	Message struct {
		Content []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	} `json:"message"`
}

// countToolCalls reads a stream-json transcript and returns the number of
// tool calls and the shell commands among them.
func countToolCalls(path string) (int, []string, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = file.Close() }()

	tools := 0
	var commands []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var event streamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		for _, block := range event.Message.Content {
			if block.Type != "tool_use" {
				continue
			}
			tools++
			var input struct {
				Command string `json:"command"`
			}
			if err := json.Unmarshal(block.Input, &input); err == nil && input.Command != "" {
				commands = append(commands, input.Command)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, nil, err
	}
	return tools, commands, nil
}

// check runs the verifiable assertions of a task against its working
// directory and its run statistics.
func check(task Task, workdir string, result Result) []CheckResult {
	out := make([]CheckResult, 0, len(task.Checks))
	for _, c := range task.Checks {
		out = append(out, runCheck(c, workdir, result))
	}
	return out
}

func runCheck(c Check, workdir string, result Result) CheckResult {
	label := c.Type
	if c.Path != "" {
		label += " " + c.Path
	}
	fail := func(format string, args ...any) CheckResult {
		detail := fmt.Sprintf(format, args...)
		if c.Reason != "" {
			detail += " (" + c.Reason + ")"
		}
		return CheckResult{Check: label, Detail: detail}
	}
	switch c.Type {
	case "file_exists":
		if _, err := os.Stat(filepath.Join(workdir, c.Path)); err != nil {
			return fail("missing")
		}
		return CheckResult{Check: label, Passed: true}
	case "glob_exists":
		matches, _ := filepath.Glob(filepath.Join(workdir, c.Path))
		if len(matches) == 0 {
			return fail("no file matches")
		}
		return CheckResult{Check: label, Passed: true, Detail: fmt.Sprintf("%d file(s)", len(matches))}
	case "contains_any":
		bodies, err := readMatches(workdir, c.Path)
		if err != nil {
			return fail("%v", err)
		}
		for _, body := range bodies {
			for _, want := range c.Values {
				if strings.Contains(body, want) {
					return CheckResult{Check: label, Passed: true, Detail: want}
				}
			}
		}
		return fail("none of %v found", c.Values)
	case "matches", "not_matches":
		re, err := regexp.Compile(c.Pattern)
		if err != nil {
			return fail("bad pattern: %v", err)
		}
		bodies, err := readMatches(workdir, c.Path)
		if err != nil {
			return fail("%v", err)
		}
		found := false
		for _, body := range bodies {
			if re.MatchString(body) {
				found = true
				break
			}
		}
		if c.Type == "matches" && !found {
			return fail("pattern %s not found", c.Pattern)
		}
		if c.Type == "not_matches" && found {
			return fail("pattern %s found", c.Pattern)
		}
		return CheckResult{Check: label, Passed: true}
	case "max_figctl_calls":
		if result.FigctlCalls > c.Value {
			return fail("%d figctl calls, want at most %d", result.FigctlCalls, c.Value)
		}
		return CheckResult{Check: label, Passed: true, Detail: fmt.Sprintf("%d call(s)", result.FigctlCalls)}
	case "max_tool_calls":
		if result.ToolCalls > c.Value {
			return fail("%d tool calls, want at most %d", result.ToolCalls, c.Value)
		}
		return CheckResult{Check: label, Passed: true, Detail: fmt.Sprintf("%d call(s)", result.ToolCalls)}
	}
	return fail("unknown check type")
}

// readMatches reads every file matching a path or glob under the working
// directory.
func readMatches(workdir, pattern string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(workdir, pattern))
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no file matches %s", pattern)
	}
	bodies := make([]string, 0, len(matches))
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		bodies = append(bodies, string(data))
	}
	return bodies, nil
}

func report(r Result) {
	status := "PASS"
	if !r.Passed {
		status = "FAIL"
	}
	fmt.Printf("%s  %s  %.1fs  %d tool calls, %d figctl calls\n", status, r.Task, r.SecondsUsed, r.ToolCalls, r.FigctlCalls)
	if r.AgentError != "" {
		fmt.Printf("     agent: %s\n", r.AgentError)
	}
	for _, c := range r.Checks {
		mark := "ok  "
		if !c.Passed {
			mark = "fail"
		}
		line := fmt.Sprintf("     %s %s", mark, c.Check)
		if c.Detail != "" {
			line += ": " + c.Detail
		}
		fmt.Println(line)
	}
	fmt.Printf("     transcript: %s\n", r.Transcript)
}
