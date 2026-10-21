package acceptance

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/test/tools/hygiene"
)

// H-113 -- no committed fixture carries a secret or an account's shape.
//
// H-112's scrubber only helps a capture someone remembered to scrub. This is
// the gate behind it: on every push CI reads every file this repository keeps
// as a fixture or a captured payload -- everything in a testdata directory,
// every NDJSON and JSON Lines file and every environment file wherever it is,
// and every JSON, text, log and archive file under test/ or experiments/,
// archive members included -- and fails on an environment file, a secret
// shape (the deny list and
// entropy test of internal/secretshape, and any value under a member the deny
// list names secret), an email address, an account-style UUID inside a path,
// a home directory, or an inventory list. A finding names the file, the line
// and the shape, never the value, so the CI log that reports a leak does not
// repeat it.
//
// The entropy test errs wide, and on main it flags one value: the curl
// format strings in nono's real capture. That capture is kept byte for byte
// and the value is recorded in test/tools/hygiene/reviewed.txt by a
// fingerprint that excuses it in that file and nothing else.
//
// Break: skip the .ndjson captures, and a credential planted in a captured
// transcript passes CI.

func TestH113_NoCommittedFixtureCarriesASecretOrAnAccountShape(t *testing.T) {
	scanned, found, err := hygiene.Walk(moduleRoot)
	if err != nil {
		t.Fatal(err)
	}
	// A scan that read nothing would pass here; the nono capture is the one
	// real payload main commits, and it is what the digest rule was measured on.
	for _, want := range []string{"test/fixtures/nono/audit-events.ndjson", "test/mutation/shared-tests.txt"} {
		if !slices.Contains(scanned, want) {
			t.Fatalf("premise: the scan did not read %s; it read %v", want, scanned)
		}
	}
	for _, f := range found {
		t.Errorf("%s -- run go run ./test/tools/scrub -w on it, read the result, and commit that", f)
	}
}

// h113Clean is a tree of scrubbed fixtures: placeholders where the shapes
// were, a digest under the member that names it, an archive of each kind.
func h113Clean(t *testing.T) map[string][]byte {
	t.Helper()
	return map[string][]byte{
		"test/fixtures/demo/payload.json": []byte(`{"session_id":"` + h112P1 + `","transcript_path":"~/.claude/projects/-project/` + h112P1 +
			`.jsonl","cwd":"~/src/app","tool_name":"Bash","tool_input":{"command":"go test ./..."},"leaf_hash":"` + h112Leaf + `"}` + "\n"),
		"test/fixtures/demo/transcript.jsonl": []byte(`{"type":"system","subtype":"init","tools":4,"skills":2,"session_id":"` + h112P1 + `"}` + "\n" +
			`{"type":"assistant","message":{"content":[{"type":"text","text":"done; mailed user-1@example.invalid from $TMPDIR/out"}]}}` + "\n"),
		"test/fixtures/demo/trail.ndjson":    []byte(`{"sequence":0,"prev_chain":null,"chain_hash":"` + h112Chain + `"}` + "\n"),
		"test/fixtures/demo/notes.txt":       []byte("a PreToolUse capture, scrubbed; the remote is git@github.com:org/repo.git\n"),
		"internal/demo/testdata/case.golden": []byte("report: 1 call, 0 hosts\n"),
		"test/fixtures/demo/bundle.tar.gz":   h113TarGz(t, "inner.json", `{"ok":true}`),
		"test/fixtures/demo/bundle.zip":      h113Zip(t, "inner.txt", "fine\n"),
	}
}

func h113Write(t *testing.T, files map[string][]byte) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func h113TarGz(t *testing.T, name, body string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func h113Zip(t *testing.T, name, body string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestH113_APlantedCredentialFailsTheScanAndACleanTreePasses(t *testing.T) {
	clean := h113Clean(t)
	scanned, found, err := hygiene.Walk(h113Write(t, clean))
	if err != nil {
		t.Fatal(err)
	}
	if len(scanned) != len(clean) {
		t.Errorf("the clean tree's %d fixtures were read as %d: %v", len(clean), len(scanned), scanned)
	}
	for _, f := range found {
		t.Errorf("clean tree: %s", f)
	}

	aws := "AKIA" + "IOSFODNN7EXAMPLE"
	for _, tc := range []struct {
		name, file string
		body       []byte
		at         string // the finding's path:line
		planted    string // never printed
	}{
		{"credential in a .json payload", "test/fixtures/demo/payload.json",
			[]byte(`{"tool_input":{"command":"gh auth login --with-token ` + h112Token + `"}}`), "test/fixtures/demo/payload.json:1", h112Token},
		{"credential on line 3 of a pretty-printed .json", "test/fixtures/demo/payload.json",
			[]byte("{\n  \"tool_name\": \"Bash\",\n  \"note\": \"" + h112Token + "\"\n}\n"), "test/fixtures/demo/payload.json:3", h112Token},
		{"credential in an .ndjson capture", "test/fixtures/demo/trail.ndjson",
			[]byte(`{"sequence":0}` + "\n" + `{"sequence":1,"note":"` + h112Stray + `"}` + "\n"), "test/fixtures/demo/trail.ndjson:2", h112Stray},
		{"credential in a .jsonl transcript", "test/fixtures/demo/transcript.jsonl",
			[]byte(`{"message":{"content":[{"type":"text","text":"use ` + h112Bearer + ` as the key"}]}}` + "\n"), "test/fixtures/demo/transcript.jsonl:1", h112Bearer},
		{"credential in a .txt", "test/fixtures/demo/notes.txt",
			[]byte("first line\naws configure set aws_access_key_id " + aws + "\n"), "test/fixtures/demo/notes.txt:2", aws},
		{"credential in a testdata file", "internal/demo/testdata/case.golden",
			[]byte("Authorization: Bearer " + h112Bearer + "\n"), "internal/demo/testdata/case.golden:1", h112Bearer},
		{"credential in a tar.gz member", "test/fixtures/demo/bundle.tar.gz",
			h113TarGz(t, "inner.json", `{"key":"`+h112Token+`"}`), "test/fixtures/demo/bundle.tar.gz!inner.json:1", h112Token},
		{"credential in a zip member", "test/fixtures/demo/bundle.zip",
			h113Zip(t, "inner.txt", "token "+h112Token+"\n"), "test/fixtures/demo/bundle.zip!inner.txt:1", h112Token},
		{"a value under a member the deny list names secret", "test/fixtures/demo/payload.json",
			[]byte(`{"env":{"DB_PASSWORD":"hunter2"}}`), "test/fixtures/demo/payload.json:1", "hunter2"},
		{"an email address", "test/fixtures/demo/transcript.jsonl",
			[]byte(`{"text":"mail alice.liddell@acme-corp.io"}` + "\n"), "test/fixtures/demo/transcript.jsonl:1", "alice.liddell"},
		{"an account-style UUID inside a path", "test/fixtures/demo/payload.json",
			[]byte(`{"transcript_path":"~/.claude/projects/-project/` + h112Session + `.jsonl"}`), "test/fixtures/demo/payload.json:1", h112Session},
		{"an inventory list", "test/fixtures/demo/transcript.jsonl",
			[]byte(`{"type":"system","subtype":"init","skills":["acme-brand"]}` + "\n"), "test/fixtures/demo/transcript.jsonl:1", "acme"},
		{"an inventory list inside a string", "test/fixtures/demo/transcript.jsonl",
			[]byte(`{"content":"{\"plugins\":[\"acme-legal\"]}"}` + "\n"), "test/fixtures/demo/transcript.jsonl:1", "acme"},
		{"a home directory", "test/fixtures/demo/payload.json",
			[]byte(`{"cwd":"/home/alice/src/app"}`), "test/fixtures/demo/payload.json:1", "alice"},
		{"a project directory named after a home", "test/fixtures/demo/payload.json",
			[]byte(`{"transcript_path":"~/.claude/projects/-home-alice-src-app/x.jsonl"}`), "test/fixtures/demo/payload.json:1", "alice"},
		{"credential in a capture outside test/", "experiments/demo/payload.json",
			[]byte(`{"tool_input":{"command":"gh auth login --with-token ` + h112Token + `"}}`), "experiments/demo/payload.json:1", h112Token},
		{"credential in an .ndjson capture anywhere in the tree", "docs/demo/trail.ndjson",
			[]byte(`{"sequence":1,"note":"` + h112Stray + `"}` + "\n"), "docs/demo/trail.ndjson:1", h112Stray},
		{"an environment file", "experiments/demo/SessionStart.env",
			[]byte("HOME=/srv/app\nSHELL=/bin/sh\n"), "experiments/demo/SessionStart.env:0", "HOME=/srv/app"},
		{"an environment file inside an archive", "test/fixtures/demo/bundle.tar.gz",
			h113TarGz(t, "hook.env", "HOME=/srv/app\n"), "test/fixtures/demo/bundle.tar.gz!hook.env:0", "HOME=/srv/app"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := h113Clean(t)
			files[tc.file] = tc.body
			_, found, err := hygiene.Walk(h113Write(t, files))
			if err != nil {
				t.Fatal(err)
			}
			at := false
			for _, f := range found {
				at = at || strings.HasPrefix(f.String(), tc.at+":")
				if strings.Contains(f.String(), tc.planted) {
					t.Errorf("a finding prints what it found: %s", f)
				}
			}
			if !at {
				t.Errorf("nothing found at %s; found %v", tc.at, found)
			}
		})
	}
}

// TestH113_ADigestPassesOnlyWhereItsFileNamesIt: a 64-hex run is a digest
// only when the file names it one -- under a member called a hash, or
// repeating a value that was -- in lowercase, at the digest's length. The
// same run anywhere else is what a token looks like.
func TestH113_ADigestPassesOnlyWhereItsFileNamesIt(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		clean      bool
	}{
		{"under a member named for a hash", `{"leaf_hash":"` + h112Leaf + `"}`, true},
		{"repeating a value named a hash", `{"chain_hash":"` + h112Chain + `"}` + "\n" + `{"prev_chain":"` + h112Chain + `"}`, true},
		{"named nowhere in the file", `{"prev_chain":"` + h112Chain + `"}`, false},
		{"in upper case", `{"leaf_hash":"` + strings.ToUpper(h112Leaf) + `"}`, false},
		{"one hex digit too long", `{"leaf_hash":"` + h112Leaf + `0"}`, false},
	} {
		found := hygiene.Scan("trail.ndjson", []byte(tc.body+"\n"))
		if clean := len(found) == 0; clean != tc.clean {
			t.Errorf("%s: clean = %v, want %v (%v)", tc.name, clean, tc.clean, found)
		}
	}
}

// TestH113_AReviewedEntryExcusesOneValueInOneFile: the reviewed list is how
// a real capture keeps a token the entropy test flags without being
// rewritten, and so it is the gate's one way round. An entry excuses the
// value a person read, in the file they read it in: the same file with a
// different value fails again, as does the same value in another file, and
// an entry that excuses nothing is a finding of its own.
func TestH113_AReviewedEntryExcusesOneValueInOneFile(t *testing.T) {
	const flagged = `{"command":"curl -w \"ok1=%{http_code}\\n\" https://example.com/"}`
	const capture = "test/fixtures/demo/trail.ndjson"
	first := hygiene.Scan(capture, []byte(flagged+"\n"))
	if len(first) != 1 || first[0].Fingerprint == "" {
		t.Fatalf("premise: the format string is one finding with a fingerprint, got %v", first)
	}
	entry := capture + " " + first[0].Fingerprint + " a curl format string, read and judged\n"

	for _, tc := range []struct {
		name  string
		files map[string]string
		clean bool
	}{
		{"the value the entry names", map[string]string{capture: flagged}, true},
		{"a different value in the same file", map[string]string{capture: strings.Replace(flagged, "ok1", "ok2", 1)}, false},
		{"the same value in another file", map[string]string{"test/fixtures/demo/other.ndjson": flagged, capture: `{"ok":true}`}, false},
		{"the value beside a credential", map[string]string{capture: flagged + "\n" + `{"note":"` + h112Token + `"}`}, false},
	} {
		files := h113Clean(t)
		for name, body := range tc.files {
			files[name] = []byte(body + "\n")
		}
		files[hygiene.Reviewed] = []byte(entry)
		_, found, err := hygiene.Walk(h113Write(t, files))
		if err != nil {
			t.Fatal(err)
		}
		if clean := len(found) == 0; clean != tc.clean {
			t.Errorf("%s: clean = %v, want %v (%v)", tc.name, clean, tc.clean, found)
		}
		if tc.name == "the same value in another file" && !slices.ContainsFunc(found, func(f hygiene.Finding) bool {
			return f.Path == hygiene.Reviewed && strings.Contains(f.What, "excuses nothing")
		}) {
			t.Errorf("%s: the entry that now excuses nothing is not reported: %v", tc.name, found)
		}
	}
}
