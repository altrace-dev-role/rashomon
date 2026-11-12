package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The K7 overlay measures (W-M2.5c, 5c-2) time the hook's own entries with
// the overlay writers in them: the real PreToolUse entry (cmdHook) over an
// Edit payload on a Go file of this repository, and the real PostToolUse
// entry (cmdPost) over the same two files. Both read this repository's index
// above the working directory, in a store root of their own with the
// session-content marker present, and again with it absent (the overlay's
// cost is the difference). Each run is a session of its own, so the 4 MiB
// bound never decides a run. RASHOMON_K7=1 runs them; RASHOMON_K7_RUNS is the
// run count (default 30).

var k7Generated = regexp.MustCompile(`(?m)^// Code generated .* DO NOT EDIT\.$`)

var k7Package = regexp.MustCompile(`(?m)^package \w+\n`)

type k7File struct {
	rel   string
	bytes int
}

// k7Files are the non-generated, non-test Go files under internal/ and cmd/,
// smallest first.
func k7Files(t *testing.T, repo string) []k7File {
	t.Helper()
	var out []k7File
	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(repo, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "testdata" {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if k7Generated.Match(b) {
				return nil
			}
			rel, _ := filepath.Rel(repo, p)
			out = append(out, k7File{filepath.ToSlash(rel), len(b)})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].bytes != out[j].bytes {
			return out[i].bytes < out[j].bytes
		}
		return out[i].rel < out[j].rel
	})
	return out
}

// k7Edit is an Edit tool input on file that rewrites its package line, which
// occurs once, to carry a trailing comment: a change whose splice is exact and
// whose result still parses.
func k7Edit(t *testing.T, repo string, f k7File) json.RawMessage {
	t.Helper()
	abs := filepath.Join(repo, filepath.FromSlash(f.rel))
	src, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	loc := k7Package.FindIndex(src)
	if loc == nil {
		t.Fatalf("%s has no package line", f.rel)
	}
	old := string(src[loc[0]:loc[1]])
	if loc[0] > 0 {
		old = "\n" + old
	}
	if n := strings.Count(string(src), old); n != 1 {
		t.Fatalf("%s: the package line occurs %d times as an old_string", f.rel, n)
	}
	in, err := json.Marshal(map[string]any{"file_path": abs, "old_string": old, "new_string": strings.TrimSuffix(old, "\n") + " // k7\n"})
	if err != nil {
		t.Fatal(err)
	}
	return in
}

// k7Series runs the entry runs times on input, each in a session of its own,
// and returns the wall times. post selects cmdPost.
func k7Series(t *testing.T, root, installID, repo string, in json.RawMessage, runs int, post bool) []time.Duration {
	t.Helper()
	args := []string{install.Marker, installID}
	var out []time.Duration
	for i := 0; i < runs; i++ {
		event := "PreToolUse"
		var resp json.RawMessage
		if post {
			event, resp = "PostToolUse", json.RawMessage(`{"stdout":"ok","stderr":"","interrupted":false}`)
		}
		body := benchJSON(t, benchToolPayload{
			benchHead: benchHead{"k7-" + strconv.Itoa(i), benchTranscript, repo, "default", event},
			PromptID:  "p1", ToolName: "Edit", ToolInput: in, ToolUseID: "toolu_k7_" + strconv.Itoa(i), ToolResponse: resp,
		})
		var stderr bytes.Buffer
		t0 := time.Now()
		var code int
		if post {
			code = cmdPost(args, bytes.NewReader(body), io.Discard, &stderr)
		} else {
			code = cmdHook(args, bytes.NewReader(body), &stderr)
		}
		out = append(out, time.Since(t0))
		if code != exitOK || stderr.Len() != 0 {
			t.Fatalf("run %d: exit %d, stderr %q", i, code, stderr.String())
		}
	}
	return out
}

func k7Overlay(t *testing.T, post bool) {
	if os.Getenv("RASHOMON_K7") == "" {
		t.Skip("RASHOMON_K7 not set")
	}
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if graph.FindIndex(repo) != repo {
		t.Skip("no index at the module root: run rashomon graph index .")
	}
	runs := 30
	if v, err := strconv.Atoi(os.Getenv("RASHOMON_K7_RUNS")); err == nil && v > 0 {
		runs = v
	}
	files := k7Files(t, repo)
	largest, median := files[len(files)-1], files[len(files)/2]
	t.Setenv("CI", "")
	t.Setenv("AI_AGENT", "")
	t.Setenv(hook.EnvHoldout, "off")
	rid, _ := baseline.RepoID(repo)
	entry := map[bool]string{false: "PreToolUse", true: "PostToolUse"}[post]
	for _, c := range []struct {
		label string
		f     k7File
	}{{"largest", largest}, {"median", median}} {
		in := k7Edit(t, repo, c.f)
		var p50, p95 [2]time.Duration
		for i, marker := range []bool{true, false} {
			root, installID := benchStarted(t)
			if marker {
				if err := os.WriteFile(filepath.Join(root, store.FileSessionContent), []byte("1\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			ds := k7Series(t, root, installID, repo, in, runs, post)
			p50[i], p95[i] = percentile(ds, 50), percentile(ds, 95)
			// The numbers mean what they say only if the writers ran or declined as stated.
			_, statErr := os.Stat(graph.SessionLogPath(root, rid))
			if marker {
				f, err := os.Open(graph.SessionLogPath(root, rid))
				if err != nil {
					t.Fatalf("%s %s marker present: no session log: %v", entry, c.label, err)
				}
				_, _, groups, err := graph.LoadSessionLogGroups(graph.NewStore(), f)
				f.Close()
				if err != nil || len(groups) != runs {
					t.Fatalf("%s %s marker present: %d groups from %d runs (%v)", entry, c.label, len(groups), runs, err)
				}
			} else if statErr == nil {
				t.Fatalf("%s %s marker absent: a session log was written", entry, c.label)
			}
			label := "marker present"
			if !marker {
				label = "marker absent"
			}
			line := fmt.Sprintf("K7 %s overlay %s file %s (%d bytes), %s: runs %d p50 %s p95 %s", entry, c.label, c.f.rel, c.f.bytes, label, runs, p50[i], p95[i])
			t.Log(line)
			fmt.Fprintln(os.Stderr, line)
		}
		if !post && c.label == "largest" && p95[0] > 150*time.Millisecond {
			t.Errorf("PreToolUse path with the overlay, largest file %s: p95 %s is over the 150 ms requirement (PLAN 20)", c.f.rel, p95[0])
		}
	}
}

// TestK7_PreToolUsePathOverlay (W-M2.5c, 5c-2): the real PreToolUse entry
// over an Edit of the largest non-generated, non-test Go file under
// internal/ or cmd/ (the gated input) and of the median-size one, the marker
// present and absent; both store holds (DeclareSeq, ReserveOverlay) are
// inside. A p95 over 150 ms for the largest file fails.
func TestK7_PreToolUsePathOverlay(t *testing.T) { k7Overlay(t, false) }

// TestK7_PostToolUsePathOverlay (C5): the same over the real PostToolUse
// entry. It reports p50 and p95 and gates nothing: PLAN has no PostToolUse
// p95.
func TestK7_PostToolUsePathOverlay(t *testing.T) { k7Overlay(t, true) }
