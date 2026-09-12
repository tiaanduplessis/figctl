package skill

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// dirs returns a temporary home and project directory for one install.
func dirs(t *testing.T) (home, project string) {
	t.Helper()
	root := t.TempDir()
	home = filepath.Join(root, "home")
	project = filepath.Join(root, "project")
	for _, dir := range []string{home, project} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home, project
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// expectedPaths is the file each agent writes, relative to home for the
// user level claude skill and to the project directory otherwise.
func expectedPaths(agent Agent, home, project string) []string {
	switch agent {
	case AgentAgents, AgentCodex, AgentGeneric:
		base := filepath.Join(home, ".agents", "skills", "figctl")
		return []string{
			filepath.Join(base, "SKILL.md"),
			filepath.Join(base, "reference", "commands.md"),
			filepath.Join(base, "reference", "output-schemas.md"),
			filepath.Join(base, "reference", "workflows.md"),
		}
	case AgentClaude:
		base := filepath.Join(home, ".agents", "skills", "figctl")
		return []string{
			filepath.Join(base, "SKILL.md"),
			filepath.Join(base, "reference", "commands.md"),
			filepath.Join(base, "reference", "output-schemas.md"),
			filepath.Join(base, "reference", "workflows.md"),
			filepath.Join(home, ".claude", "skills", "figctl"),
		}
	case AgentCursor:
		return []string{filepath.Join(project, ".cursor", "rules", "figctl.mdc")}
	case AgentCopilot:
		return []string{filepath.Join(project, ".github", "copilot-instructions.md")}
	}
	return nil
}

func TestInstallAndReinstallEveryAgent(t *testing.T) {
	for _, agent := range []Agent{AgentAgents, AgentClaude, AgentCursor, AgentCopilot, AgentCodex, AgentGeneric} {
		t.Run(string(agent), func(t *testing.T) {
			home, project := dirs(t)
			opts := Options{Agent: agent, Home: home, Dir: project}

			actions, err := Install(opts)
			if err != nil {
				t.Fatalf("install: %v", err)
			}
			want := expectedPaths(agent, home, project)
			if len(actions) != len(want) {
				t.Fatalf("got %d actions, want %d: %+v", len(actions), len(want), actions)
			}
			for i, action := range actions {
				if action.Path != want[i] {
					t.Fatalf("action %d path = %s, want %s", i, action.Path, want[i])
				}
				if !action.Written() || action.Bytes == 0 {
					t.Fatalf("action %d should report a write with bytes: %+v", i, action)
				}
				info, err := os.Lstat(action.Path)
				if err != nil {
					t.Fatalf("stat %s: %v", action.Path, err)
				}
				if runtime.GOOS != "windows" {
					// A link carries the mode of the link itself, which the
					// platform sets; only real files have to stay at 0644.
					if perm := info.Mode().Perm(); info.Mode()&os.ModeSymlink == 0 && perm|0o644 != 0o644 {
						t.Fatalf("%s mode = %o, want no bits beyond 0644", action.Path, perm)
					}
					if dir, err := os.Stat(filepath.Dir(action.Path)); err != nil {
						t.Fatal(err)
					} else if perm := dir.Mode().Perm(); perm|0o755 != 0o755 {
						t.Fatalf("%s mode = %o, want no bits beyond 0755", filepath.Dir(action.Path), perm)
					}
				}
			}

			// Every install carries the marker, so a re-install can tell
			// its own output from a hand written file.
			before := map[string]string{}
			for _, path := range want {
				// A link resolves to the canonical directory rather than to a
				// document, so there is no body to mark.
				if info, err := os.Stat(path); err == nil && info.IsDir() {
					continue
				}
				before[path] = read(t, path)
				if !strings.Contains(before[path], Marker) && !strings.Contains(before[path], BlockBegin) {
					t.Fatalf("%s carries neither the marker nor the block delimiters", path)
				}
			}

			// Installing again changes nothing.
			again, err := Install(opts)
			if err != nil {
				t.Fatalf("re-install: %v", err)
			}
			for _, action := range again {
				if action.Op != OpUnchanged {
					t.Fatalf("re-install should be a no-op, got %+v", action)
				}
			}
			for path, content := range before {
				if read(t, path) != content {
					t.Fatalf("%s changed on re-install", path)
				}
			}

			// Uninstall removes what install wrote.
			removed, err := Uninstall(opts)
			if err != nil {
				t.Fatalf("uninstall: %v", err)
			}
			for _, action := range removed {
				if !action.Written() {
					t.Fatalf("uninstall should report a change, got %+v", action)
				}
			}
			for _, path := range want {
				if exists(path) {
					t.Fatalf("%s still exists after uninstall", path)
				}
			}

			// Uninstalling again is a no-op rather than an error.
			twice, err := Uninstall(opts)
			if err != nil {
				t.Fatalf("second uninstall: %v", err)
			}
			for _, action := range twice {
				if action.Op != OpAbsent {
					t.Fatalf("second uninstall should find nothing, got %+v", action)
				}
			}
		})
	}
}

func TestInstallProjectScopesTheSkill(t *testing.T) {
	home, project := dirs(t)
	opts := Options{Agent: AgentClaude, Home: home, Dir: project, Project: true}
	actions, err := Install(opts)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	for _, action := range actions {
		if !strings.HasPrefix(action.Path, project) {
			t.Fatalf("path %s is not under %s", action.Path, project)
		}
	}
	if exists(filepath.Join(home, ".claude")) || exists(filepath.Join(home, ".agents")) {
		t.Fatal("--project should not write into the home directory")
	}
}

// TestClaudeLinkResolvesToTheCanonicalSkill guards against a dangling link:
// the Claude target is a symlink into .agents/skills, so that tree has to be
// written even when only Claude was asked for.
func TestClaudeLinkResolvesToTheCanonicalSkill(t *testing.T) {
	home, project := dirs(t)
	if _, err := Install(Options{Agent: AgentClaude, Home: home, Dir: project}); err != nil {
		t.Fatalf("install: %v", err)
	}
	link := filepath.Join(home, ".claude", "skills", "figctl")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("%s should be a symlink: %v", link, err)
	}
	if filepath.IsAbs(target) {
		t.Fatalf("the link must be relative so it survives a move, got %s", target)
	}
	body, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
	if err != nil {
		t.Fatalf("the link does not resolve to the skill: %v", err)
	}
	if !strings.Contains(string(body), "name: figctl") {
		t.Fatal("the linked SKILL.md is not the figctl skill")
	}
}

func TestInstallAllCoversEveryTargetOnce(t *testing.T) {
	home, project := dirs(t)
	actions, err := Install(Options{Agent: AgentAll, Home: home, Dir: project})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	seen := map[string]int{}
	for _, action := range actions {
		seen[action.Path]++
	}
	for path, count := range seen {
		if count != 1 {
			t.Fatalf("%s was written %d times", path, count)
		}
	}
	for _, agent := range Agents {
		for _, path := range expectedPaths(agent, home, project) {
			if !exists(path) {
				t.Fatalf("%s (%s) was not written", path, agent)
			}
		}
	}
}

func TestMarkedBlockPreservesSurroundingContent(t *testing.T) {
	home, project := dirs(t)
	path := filepath.Join(project, ".github", "copilot-instructions.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "# Project rules\n\nUse tabs.\n\n<!-- BEGIN other-tool -->\nkeep me\n<!-- END other-tool -->\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := Options{Agent: AgentCopilot, Home: home, Dir: project}

	actions, err := Install(opts)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if actions[0].Op != OpAppendBlock {
		t.Fatalf("op = %s, want %s", actions[0].Op, OpAppendBlock)
	}
	content := read(t, path)
	for _, want := range []string{"# Project rules", "Use tabs.", "keep me", BlockBegin, BlockEnd, "## Workflow"} {
		if !strings.Contains(content, want) {
			t.Fatalf("content is missing %q:\n%s", want, content)
		}
	}

	// A second install replaces the block instead of appending another.
	if _, err := Install(opts); err != nil {
		t.Fatalf("re-install: %v", err)
	}
	if n := strings.Count(read(t, path), BlockBegin); n != 1 {
		t.Fatalf("found %d figctl blocks, want 1", n)
	}

	// A stale block is replaced in place, keeping what is around it.
	stale := strings.Replace(read(t, path), "## Workflow", "## Stale", 1)
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	actions, err = Install(opts)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if actions[0].Op != OpReplaceBlock {
		t.Fatalf("op = %s, want %s", actions[0].Op, OpReplaceBlock)
	}
	content = read(t, path)
	if strings.Contains(content, "## Stale") || !strings.Contains(content, "keep me") {
		t.Fatalf("stale block was not replaced cleanly:\n%s", content)
	}

	// Uninstall takes the block back out and leaves the rest.
	if _, err := Uninstall(opts); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	content = read(t, path)
	if strings.Contains(content, BlockBegin) {
		t.Fatalf("block survived uninstall:\n%s", content)
	}
	for _, want := range []string{"# Project rules", "Use tabs.", "keep me", "<!-- BEGIN other-tool -->"} {
		if !strings.Contains(content, want) {
			t.Fatalf("uninstall lost %q:\n%s", want, content)
		}
	}
}

func TestUninstallRemovesAFileItCreatedWholesale(t *testing.T) {
	home, project := dirs(t)
	opts := Options{Agent: AgentCopilot, Home: home, Dir: project}
	if _, err := Install(opts); err != nil {
		t.Fatalf("install: %v", err)
	}
	path := filepath.Join(project, ".github", "copilot-instructions.md")
	if _, err := Uninstall(opts); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if exists(path) {
		t.Fatal("a file that held nothing but the figctl block should be removed")
	}
}

func TestForeignFileIsNotClobbered(t *testing.T) {
	home, project := dirs(t)
	path := filepath.Join(project, ".cursor", "rules", "figctl.mdc")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "---\ndescription: mine\n---\n\nhand written\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := Options{Agent: AgentCursor, Home: home, Dir: project}

	actions, err := Install(opts)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if actions[0].Op != OpSkip || actions[0].Reason == "" {
		t.Fatalf("install should skip a foreign file, got %+v", actions[0])
	}
	if read(t, path) != original {
		t.Fatal("a foreign file was overwritten")
	}

	// Uninstall leaves it alone for the same reason.
	removed, err := Uninstall(opts)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if removed[0].Op != OpSkip {
		t.Fatalf("uninstall should skip a foreign file, got %+v", removed[0])
	}
	if !exists(path) {
		t.Fatal("a foreign file was removed")
	}

	// --force is the way through.
	forced, err := Install(Options{Agent: AgentCursor, Home: home, Dir: project, Force: true})
	if err != nil {
		t.Fatalf("forced install: %v", err)
	}
	if forced[0].Op != OpReplace {
		t.Fatalf("op = %s, want %s", forced[0].Op, OpReplace)
	}
	if !strings.Contains(read(t, path), Marker) {
		t.Fatal("forced install should write the figctl rule")
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	home, project := dirs(t)
	actions, err := Install(Options{Agent: AgentAll, Home: home, Dir: project, DryRun: true})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("dry run should still plan actions")
	}
	for _, action := range actions {
		if action.Bytes == 0 {
			t.Fatalf("dry run should report byte counts: %+v", action)
		}
		if exists(action.Path) {
			t.Fatalf("dry run wrote %s", action.Path)
		}
	}
	if exists(filepath.Join(home, ".claude")) || exists(filepath.Join(project, ".cursor")) {
		t.Fatal("dry run created directories")
	}

	// A dry run uninstall is equally inert.
	if _, err := Install(Options{Agent: AgentAll, Home: home, Dir: project}); err != nil {
		t.Fatalf("install: %v", err)
	}
	planned, err := Uninstall(Options{Agent: AgentAll, Home: home, Dir: project, DryRun: true})
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	for _, action := range planned {
		if action.Written() && !exists(action.Path) {
			t.Fatalf("dry run removed %s", action.Path)
		}
	}
}

func TestSafeJoinRefusesToEscapeTheRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills", "figctl")
	for _, rel := range []string{
		"../escape.md",
		"reference/../../escape.md",
		"/etc/passwd",
		"",
		"a/../../b",
	} {
		if got, err := safeJoin(root, rel); err == nil {
			t.Fatalf("safeJoin(%q) = %q, want an error", rel, got)
		}
	}
	got, err := safeJoin(root, "reference/commands.md")
	if err != nil {
		t.Fatalf("safeJoin: %v", err)
	}
	if want := filepath.Join(root, "reference", "commands.md"); got != want {
		t.Fatalf("safeJoin = %q, want %q", got, want)
	}
}

func TestEverySkillPathStaysInsideTheRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "figctl")
	for _, f := range Files() {
		path, err := safeJoin(root, f.Path)
		if err != nil {
			t.Fatalf("%s: %v", f.Path, err)
		}
		if !strings.HasPrefix(path, root+string(filepath.Separator)) {
			t.Fatalf("%s resolves to %s, outside %s", f.Path, path, root)
		}
	}
}

func TestSkillFrontMatter(t *testing.T) {
	content, err := Content("SKILL")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"---\nname: figctl\n",
		"description: ",
		"allowed-tools: Bash(figctl:*)\n",
		Marker,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("SKILL.md is missing %q", want)
		}
	}
	if !strings.HasPrefix(content, "---\n") {
		t.Fatal("SKILL.md must start with its front matter")
	}
	if lines := strings.Count(content, "\n"); lines > 210 {
		t.Fatalf("SKILL.md is %d lines; it is loaded into context and should stay near 200", lines)
	}
	if strings.Contains(Body(), "---\nname: figctl") {
		t.Fatal("Body should strip the front matter")
	}
}

func TestRenderAndLocationPerAgent(t *testing.T) {
	claude, err := Render(AgentClaude, "SKILL")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(claude, "---\nname: figctl") {
		t.Fatal("the claude skill keeps its front matter")
	}
	if !strings.Contains(claude, "reference/commands.md") {
		t.Fatal("the claude skill routes to its reference files")
	}

	cursor, err := Render(AgentCursor, "SKILL")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"---\ndescription: ", "alwaysApply: false\n", "figctl skill print --file commands"} {
		if !strings.Contains(cursor, want) {
			t.Fatalf("the cursor rule is missing %q", want)
		}
	}
	if strings.Contains(cursor, "reference/commands.md") {
		t.Fatal("the cursor rule must not route to files that are not installed")
	}

	// Copilot reads an instructions file rather than a skill directory, so
	// its document is the marked block.
	block, err := Render(AgentCopilot, "SKILL")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(block, BlockBegin) || !strings.Contains(block, BlockEnd) {
		t.Fatal("the copilot document is a marked block")
	}

	// Codex reads .agents/skills, so it gets the skill itself.
	codex, err := Render(AgentCodex, "SKILL")
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(codex, BlockBegin) {
		t.Fatal("the codex document is the skill file, not a marked block")
	}

	// Reference documents are the same for every agent.
	for _, agent := range []Agent{AgentClaude, AgentCursor, AgentCodex} {
		got, err := Render(agent, "workflows")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, "# figctl workflows") {
			t.Fatalf("%s workflows document is wrong", agent)
		}
	}

	if _, err := Render(AgentAll, "SKILL"); err == nil {
		t.Fatal("printing for every agent at once should be an error")
	}
	if _, err := Render(AgentClaude, "nope"); err == nil {
		t.Fatal("an unknown document should be an error")
	}

	if got := Location(AgentClaude, "SKILL"); got != "SKILL.md" {
		t.Fatalf("claude location = %s", got)
	}
	if got := Location(AgentCursor, "SKILL"); got != ".cursor/rules/figctl.mdc" {
		t.Fatalf("cursor location = %s", got)
	}
	if got := Location(AgentCopilot, "SKILL"); got != ".github/copilot-instructions.md" {
		t.Fatalf("copilot location = %s", got)
	}
	if got := Location(AgentGeneric, "SKILL"); got != "SKILL.md" {
		t.Fatalf("generic location = %s", got)
	}
	if got := Location(AgentCodex, "workflows"); got != "reference/workflows.md" {
		t.Fatalf("reference location = %s", got)
	}
}

func TestParseAgent(t *testing.T) {
	for _, name := range AgentNames() {
		if _, err := ParseAgent(name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := ParseAgent("CLAUDE"); err != nil {
		t.Fatalf("agent names are case insensitive: %v", err)
	}
	if _, err := ParseAgent("gemini"); err == nil {
		t.Fatal("an unknown agent should be an error")
	}
}

func TestDocumentsCarryNoPlaceholders(t *testing.T) {
	for _, f := range Files() {
		content := f.Content()
		if content == "" {
			t.Fatalf("%s is empty", f.Path)
		}
		for _, bad := range []string{"TODO", "FIXME", "placeholder", "Lorem ipsum"} {
			if strings.Contains(content, bad) {
				t.Errorf("%s contains %q", f.Path, bad)
			}
		}
	}
}
