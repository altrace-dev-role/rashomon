package acceptance

// PW1b-ON-230 1b: the plugin runs the platform graph's writer. Claude Code
// spawns ${CLAUDE_PLUGIN_ROOT}/bin/pgraph from the plugin's hooks.json, in the
// background, at session start and when Claude finishes a reply (Stop);
// the rashomon binary still starts no process (H-17, TRU-40). The graph is
// read at turn boundaries (the Stop, the next prompt), so one run per turn
// keeps it as current as its readers see it, and a run per tool call would
// add a process to every call against INV-10's per-call bound for no reader
// (PF-PL6: a debounced cadence). The risk held down: an entry on a per-call
// event, an entry that names a launcher the plugin does not ship, an entry
// that blocks the reply it follows, a launcher that prints on stdout (a
// SessionStart hook's stdout goes to the model), and a writer the person was
// never told about.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
)

// pgraphHook is the one hook object the plugin runs pgraph with.
var pgraphHook = map[string]any{
	"type": "command", "command": "${CLAUDE_PLUGIN_ROOT}/bin/pgraph",
	"args": []any{"sync", "-hook"}, "timeout": float64(90), "async": true,
}

// TestPW1b_ThePluginRunsTheWriterInTheBackground: hooks.json runs pgraph
// sync -hook asynchronously from exactly two groups, SessionStart and Stop,
// neither with a matcher; the launcher it names is shipped and executable,
// the build makes a binary for every platform the recorder has, and the
// built binaries are not committable. Break: drop async, add it to
// PostToolUse, or ship no launcher, and this fails.
func TestPW1b_ThePluginRunsTheWriterInTheBackground(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(moduleRoot, "plugin", "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Matcher *string          `json:"matcher"`
			Hooks   []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for event, groups := range doc.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				if h["command"] != pgraphHook["command"] {
					continue
				}
				hb, _ := json.Marshal(h)
				wb, _ := json.Marshal(pgraphHook)
				if string(hb) != string(wb) || len(g.Hooks) != 1 {
					t.Errorf("%s: pgraph's group is %s, want %s alone in its group", event, hb, wb)
				}
				m := "(none)"
				if g.Matcher != nil {
					m = *g.Matcher
				}
				found[event] = m
			}
		}
	}
	if want := map[string]string{"SessionStart": "(none)", "Stop": "(none)"}; !mapsEqual(found, want) {
		t.Errorf("pgraph runs from %v, want %v", found, want)
	}
	launcher := filepath.Join(moduleRoot, "plugin", "bin", "pgraph")
	if fi, err := os.Stat(launcher); err != nil || fi.Mode()&0o111 == 0 {
		t.Errorf("plugin/bin/pgraph is not an executable launcher: %v", err)
	}
	build, err := os.ReadFile(filepath.Join(moduleRoot, "scripts", "build-plugin.sh"))
	if err != nil || !strings.Contains(string(build), `-o "../plugin/bin/pgraph-$os-$arch" ./cmd/pgraph`) {
		t.Errorf("scripts/build-plugin.sh builds no pgraph per platform")
	}
	ignore, err := os.ReadFile(filepath.Join(moduleRoot, ".gitignore"))
	if err != nil || !slices.Contains(strings.Split(string(ignore), "\n"), "/plugin/bin/pgraph-*") {
		t.Errorf(".gitignore leaves the built pgraph binaries committable")
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// TestPW1b_ThePluginLauncherRunsTheHookSilently: the shipped launcher, with
// a pgraph built for this machine beside it, runs the hook on a Claude Code
// payload: a Go repository gets its graph, stdout stays empty and the exit
// is 0; with no binary for this machine it exits 0 with one stderr line.
// Break: let the launcher echo, or exit nonzero without a binary.
func TestPW1b_ThePluginLauncherRunsTheHookSilently(t *testing.T) {
	plugin := t.TempDir()
	src, err := os.ReadFile(filepath.Join(moduleRoot, "plugin", "bin", "pgraph"))
	if err != nil {
		t.Fatalf("the plugin ships no pgraph launcher: %v", err)
	}
	launcher := filepath.Join(plugin, "pgraph")
	if err := os.WriteFile(launcher, src, 0o755); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	for name, body := range h156Files {
		p := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	payload := `{"hook_event_name":"PostToolUse","tool_name":"Edit","cwd":` + quoteJSON(filepath.Join(repo, "a")) + `}`
	runLauncher := func() (int, string, string) {
		cmd := exec.Command(launcher, "sync", "-hook")
		cmd.Stdin = strings.NewReader(payload)
		cmd.Env = append(os.Environ(), "CLAUDE_PROJECT_DIR=")
		var out, errb strings.Builder
		cmd.Stdout, cmd.Stderr = &out, &errb
		err := cmd.Run()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return code, out.String(), errb.String()
	}
	if code, out, errs := runLauncher(); code != 0 || out != "" || strings.Count(errs, "\n") != 1 || !strings.HasPrefix(errs, "pgraph: ") {
		t.Errorf("no binary for this machine: exit %d, stdout %q, stderr %q; want 0, silence, one line", code, out, errs)
	}
	bin, err := os.ReadFile(pgraphBinary(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugin, "pgraph-"+runtime.GOOS+"-"+runtime.GOARCH), bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := runLauncher(); code != 0 || out != "" || errs != "" {
		t.Errorf("the hook: exit %d, stdout %q, stderr %q; want 0 and silence", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(repo, ".pgraph", "pgraph.log")); err != nil {
		t.Errorf("the hook wrote no graph: %v", err)
	}
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// TestPW1b_TheWriterIsDisclosedWhereThePersonConsents: enabling the plugin
// is the plugin route's consent (PLUGIN-CONSENT1), so its description says
// pgraph runs and what it writes in the repository, and the full screen has
// the paragraph that says when it runs, what it reads, that the go tool runs
// offline, and how to forget it. Break: ship the entry without the words, or
// keep the words of a per-call entry.
func TestPW1b_TheWriterIsDisclosedWhereThePersonConsents(t *testing.T) {
	for _, w := range []string{"pgraph", ".pgraph", "in the background", "go.mod"} {
		if !strings.Contains(disclose.PluginDescription, w) {
			t.Errorf("the plugin description does not say %q", w)
		}
	}
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for _, p := range inv.Paragraphs {
		if strings.Contains(p.Text, ".pgraph") {
			text = p.Text
		}
	}
	for _, w := range []string{"in the background", "finishes a reply", "session start", "go tool offline",
		".gitignore", "Deleting .pgraph forgets", "watch route runs no such hook"} {
		if !strings.Contains(text, w) {
			t.Errorf("the full screen's pgraph paragraph does not say %q:\n%s", w, text)
		}
	}
	for _, w := range []string{"Write, Edit", "after each edit"} {
		if strings.Contains(text, w) || strings.Contains(disclose.PluginDescription, w) {
			t.Errorf("the pgraph disclosure still says %q, from the per-call entry", w)
		}
	}
}
