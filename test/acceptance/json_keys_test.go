package acceptance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// docKeyPaths lists every key path in a JSON document, an array's elements
// written as [] and their keys merged, sorted and one per line: the
// document's shape with none of its values.
func docKeyPaths(t *testing.T, raw []byte) []byte {
	t.Helper()
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, raw)
	}
	seen := map[string]bool{}
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch v := v.(type) {
		case map[string]any:
			for k, child := range v {
				p := k
				if prefix != "" {
					p = prefix + "." + k
				}
				seen[p] = true
				walk(p, child)
			}
		case []any:
			for _, child := range v {
				walk(prefix+"[]", child)
			}
		}
	}
	walk("", doc)
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return []byte(strings.Join(paths, "\n") + "\n")
}

// checkKeySnapshot compares a document's key paths with a committed
// snapshot, rewriting it under -update.
//
// The snapshot holds paths, not values, so it carries no copy of the
// document's own schema_version to compare against: a sibling
// .schema_version file holds the number the committed snapshot was taken at,
// and -update refuses to drop a key path while that number is unchanged, for
// the same reason report_golden_test's guard does with report_version.
func checkKeySnapshot(t *testing.T, name string, raw []byte) {
	t.Helper()
	got := docKeyPaths(t, raw)
	path := filepath.Join("testdata", name)
	versionPath := path + ".schema_version"
	newVersion := versionOf(raw, "schema_version")
	if *updateGoldens {
		if before, err := os.ReadFile(path); err == nil {
			oldVersion := -1
			if v, err := os.ReadFile(versionPath); err == nil {
				if n, err := strconv.Atoi(strings.TrimSpace(string(v))); err == nil {
					oldVersion = n
				}
			}
			if removed, blocked := guardDropsAKey(before, got, oldVersion, newVersion); blocked {
				t.Fatalf("-update would drop %v from %s with schema_version still %d; bump it first", removed, path, newVersion)
			}
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		if newVersion >= 0 {
			if err := os.WriteFile(versionPath, []byte(strconv.Itoa(newVersion)+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v: run with -update to write it", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("the key paths differ from %s (rewrite it with -update only for a deliberate change, and bump schema_version):\n--- got\n%s\n--- want\n%s",
			path, got, want)
	}
}

// TestRubricNAMING3_DigestJSONKeysAreTheSnapshot pins the digest document's
// key paths, so a renamed or dropped key is a diff to a committed file rather
// than a change no test sees.
func TestRubricNAMING3_DigestJSONKeysAreTheSnapshot(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	e.mustHook(p.build(t))
	e.mustPost(failurePayload(t, p.ToolUseID, "Exit code 1", false, 30))
	p2 := defaultPayload()
	p2.ToolUseID = "toolu_2"
	e.mustHook(p2.build(t))

	res := e.digestRaw("--session", testSession, "--prompt", p.PromptID)
	if res.exitCode != 0 {
		t.Fatalf("digest: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	checkKeySnapshot(t, "digest-keypaths.txt", []byte(res.stdout))
}

// TestRubricNAMING3_SpendJSONKeysAreTheSnapshot pins spend --json's key
// paths over a window holding a priced response and a recorded turn with a
// failed call.
func TestRubricNAMING3_SpendJSONKeysAreTheSnapshot(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	transcript := filepath.Join(e.configDir, "projects", "-work-project", testSession+".jsonl")
	p := defaultPayload()
	p.TranscriptPath = transcript
	e.mustHook(p.build(t))
	e.mustPost(failurePayload(t, p.ToolUseID, "Exit code 1", false, 30))

	at := time.Now().Add(-time.Minute)
	writeSessionTranscript(t, e.configDir, testSession,
		promptLine(t, testSession, p.PromptID, at),
		usageLine(t, testSession, "msg_1", at.Add(time.Second), 1_000, toolBlock(p.ToolUseID)),
		usageLine(t, testSession, "msg_2", at.Add(2*time.Second), 2_000, textBlock("Ran the command as requested.")))

	res := e.run("", nil, "spend", "--json")
	if res.exitCode != 0 {
		t.Fatalf("spend --json: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	checkKeySnapshot(t, "spend-keypaths.txt", []byte(res.stdout))
}
