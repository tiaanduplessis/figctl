package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiaanduplessis/figctl/internal/figctl"
	"github.com/tiaanduplessis/figctl/internal/skill"
)

// skillPaths pulls the written paths out of a skill install or uninstall
// envelope.
func skillPaths(t *testing.T, r result, command string) []string {
	t.Helper()
	d := data(t, r, command)
	raw, isList := d["paths"].([]any)
	if !isList {
		t.Fatalf("paths is %T: %s", d["paths"], r.stdout)
	}
	paths := make([]string, 0, len(raw))
	for _, p := range raw {
		paths = append(paths, p.(string))
	}
	return paths
}

func TestSkillInstallDryRun(t *testing.T) {
	root := isolate(t)
	r := execute(t, "", "skill", "install", "--agent", "all", "--dry-run")
	ok(t, r)
	d := data(t, r, "skill.install")
	if d["dryRun"] != true || d["agent"] != "all" {
		t.Fatalf("data: %v", d)
	}
	actions, isList := d["actions"].([]any)
	if !isList || len(actions) == 0 {
		t.Fatalf("actions: %v", d["actions"])
	}
	for _, entry := range actions {
		action := entry.(map[string]any)
		if action["bytes"].(float64) <= 0 {
			t.Fatalf("dry run should report byte counts: %v", action)
		}
		if _, err := os.Stat(action["path"].(string)); err == nil {
			t.Fatalf("dry run wrote %v", action["path"])
		}
	}
	if !strings.Contains(strings.Join(hints(t, r), "\n"), "Dry run") {
		t.Fatalf("hints: %v", hints(t, r))
	}
	if _, err := os.Stat(filepath.Join(root, "home", ".claude")); err == nil {
		t.Fatal("dry run created the skill directory")
	}
}

func TestSkillInstallProject(t *testing.T) {
	isolate(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	r := execute(t, "", "skill", "install", "--project")
	ok(t, r)
	paths := skillPaths(t, r, "skill.install")
	// .agents/skills is the canonical location every client reads, and
	// .claude/skills links to it rather than holding a second copy.
	base := filepath.Join(cwd, ".agents", "skills", "figctl")
	want := []string{
		filepath.Join(base, "SKILL.md"),
		filepath.Join(base, "reference", "commands.md"),
		filepath.Join(base, "reference", "output-schemas.md"),
		filepath.Join(base, "reference", "workflows.md"),
		filepath.Join(cwd, ".claude", "skills", "figctl"),
	}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i, path := range want {
		if paths[i] != path {
			t.Fatalf("path %d = %s, want %s", i, paths[i], path)
		}
		// The last entry is the link into the canonical tree, which resolves
		// to a directory rather than a document.
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			if _, err := os.Readlink(path); err != nil {
				t.Fatalf("%s should be a symlink to the canonical skill: %v", path, err)
			}
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if !strings.Contains(string(body), skill.Marker) {
			t.Fatalf("%s does not carry the figctl marker", path)
		}
	}
	if !strings.Contains(string(mustRead(t, want[0])), "allowed-tools: Bash(figctl:*)") {
		t.Fatal("SKILL.md front matter is missing")
	}

	// Installing again is a no-op that writes no path.
	r = execute(t, "", "skill", "install", "--project")
	ok(t, r)
	if paths := skillPaths(t, r, "skill.install"); len(paths) != 0 {
		t.Fatalf("re-install wrote %v", paths)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestSkillInstallUnknownAgent(t *testing.T) {
	isolate(t)
	r := execute(t, "", "skill", "install", "--agent", "gemini")
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("exit = %d, stdout = %s", r.code, r.stdout)
	}
}

func TestSkillPrint(t *testing.T) {
	isolate(t)

	// JSON mode wraps the document in the envelope.
	r := execute(t, "", "skill", "print")
	ok(t, r)
	d := data(t, r, "skill.print")
	if d["file"] != "SKILL" || d["path"] != "SKILL.md" {
		t.Fatalf("data: %v", d)
	}
	content, isString := d["content"].(string)
	if !isString || !strings.HasPrefix(content, "---\nname: figctl") {
		t.Fatalf("content: %q", content)
	}

	// Every other mode writes the document itself, so it can be redirected.
	r = execute(t, "", "skill", "print", "-o", "md")
	ok(t, r)
	if !strings.HasPrefix(r.stdout, "---\nname: figctl") || strings.Contains(r.stdout, `"command": "skill.print"`) {
		t.Fatalf("md output should be the document itself:\n%s", r.stdout[:min(200, len(r.stdout))])
	}

	// The reference documents print by name.
	for _, file := range []string{"commands", "schemas", "workflows"} {
		r = execute(t, "", "skill", "print", "--file", file, "-o", "md")
		ok(t, r)
		if !strings.Contains(r.stdout, "# figctl") || !strings.HasPrefix(r.stdout, skill.Marker) {
			t.Fatalf("%s output: %q", file, r.stdout[:min(80, len(r.stdout))])
		}
	}

	// Copilot reads an instructions file, so its form is the marked block.
	r = execute(t, "", "skill", "print", "--agent", "copilot", "-o", "md")
	ok(t, r)
	if !strings.HasPrefix(r.stdout, skill.BlockBegin) {
		t.Fatalf("copilot output should be a marked block:\n%s", r.stdout[:min(200, len(r.stdout))])
	}

	r = execute(t, "", "skill", "print", "--file", "nope")
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("exit = %d, stdout = %s", r.code, r.stdout)
	}
}

func TestSkillUninstallNeedsConfirmation(t *testing.T) {
	isolate(t)
	ok(t, execute(t, "", "skill", "install", "--project"))

	// stdin is not a terminal in tests, so the confirmation must be
	// explicit.
	r := execute(t, "", "skill", "uninstall", "--project")
	if r.code != figctl.ExitUsage || errorCode(t, r) != "USAGE" {
		t.Fatalf("exit = %d, stdout = %s", r.code, r.stdout)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	skillFile := filepath.Join(cwd, ".claude", "skills", "figctl", "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		t.Fatal("an unconfirmed uninstall must not remove anything")
	}

	// A dry run needs no confirmation because it changes nothing.
	r = execute(t, "", "skill", "uninstall", "--project", "--dry-run")
	ok(t, r)
	if _, err := os.Stat(skillFile); err != nil {
		t.Fatal("a dry run uninstall must not remove anything")
	}

	r = execute(t, "", "skill", "uninstall", "--project", "--yes")
	ok(t, r)
	if len(skillPaths(t, r, "skill.uninstall")) == 0 {
		t.Fatalf("uninstall reported no paths: %s", r.stdout)
	}
	if _, err := os.Stat(skillFile); err == nil {
		t.Fatal("the skill file survived uninstall")
	}
	if _, err := os.Stat(filepath.Join(cwd, ".claude", "skills", "figctl")); err == nil {
		t.Fatal("the empty skill directory should be pruned")
	}
}

func TestSkillInstallBlockTargets(t *testing.T) {
	isolate(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	instructions := filepath.Join(cwd, ".github", "copilot-instructions.md")
	if err := os.MkdirAll(filepath.Dir(instructions), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(instructions, []byte("# House rules\n\nNo emoji.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := execute(t, "", "skill", "install", "--agent", "copilot")
	ok(t, r)
	body := string(mustRead(t, instructions))
	for _, want := range []string{"# House rules", "No emoji.", skill.BlockBegin, skill.BlockEnd} {
		if !strings.Contains(body, want) {
			t.Fatalf("the instructions file is missing %q:\n%s", want, body)
		}
	}
	r = execute(t, "", "skill", "uninstall", "--agent", "copilot", "--yes")
	ok(t, r)
	body = string(mustRead(t, instructions))
	if strings.Contains(body, skill.BlockBegin) || !strings.Contains(body, "No emoji.") {
		t.Fatalf("uninstall did not restore the file:\n%s", body)
	}
}

// TestSkillInstallDoesNotTouchAgentsMd keeps a skill out of the always-on
// instruction file. AGENTS.md holds repository rules that load every turn; a
// skill is a directory loaded on demand, which is the point of shipping one.
func TestSkillInstallDoesNotTouchAgentsMd(t *testing.T) {
	isolate(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(cwd, "AGENTS.md")
	original := "# House rules\n\nNo emoji.\n"
	if err := os.WriteFile(agents, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"codex", "generic", "agents", "all"} {
		r := execute(t, "", "skill", "install", "--agent", agent)
		ok(t, r)
	}
	if body := string(mustRead(t, agents)); body != original {
		t.Fatalf("AGENTS.md was modified:\n%s", body)
	}
}
