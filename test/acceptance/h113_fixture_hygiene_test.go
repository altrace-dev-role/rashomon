package acceptance

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path"
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
// The session fixtures of internal/graph/session carry rashomon's own steer
// line ids, dedupe keys and SHA-256 sums, which the entropy test reads as
// tokens; h113Typed excuses those by kind of file, member and class of value.
//
// Break: skip the .ndjson captures, and a credential planted in a captured
// transcript passes CI.

// h113Contracts are the files whose secret-shape findings H-113 excuses, by
// exact path, each with its reason (D156): hand-written contracts that a test
// of their own reads, not captures, whose Go identifiers and the prose naming
// them the entropy test reads as secret shapes. An allow list, not a skip:
// the file is still scanned, only that shape is excused (an email address,
// an account UUID, a home directory or an inventory in it still fails), and
// an entry the scan did not read fails as stale.
var h113Contracts = map[string]string{
	"test/prd/truth_table.json":                                       "the PRD implementation truth table (card TRUTH-TABLE1): Go identifiers and prose, checked by test/prd's TestPRD1 tests",
	"test/acceptance/testdata/measure1/claude.py":                     "the hand-written claude stub of MEASURE1's driver tests (not a capture): Python identifiers, keyword arguments and prose naming Claude Code's hook fields, read by TestMS1; scrub would redact its code",
	"test/acceptance/testdata/measure1/go.py":                         "the hand-written go stub of MEASURE1's driver tests (not a capture): its go env prints a cgo flags line of Go's own shape, with a work directory named for the process, that G1a's normaliser must fold; scrub would redact its code",
	"test/live/autosteer/t1/pins.txt":                                 "the T1 pre-registration pins (PLAN section 7, frozen before the first run): the commit shas of the pinned tree and binaries and the SHA-256 of the baseline pgraph log, sourced by run.sh and the judges; a scrubbed pin pins nothing",
	"test/live/autosteer/t1b/pins.txt":                                "the T1b pre-registration pins (README T1b section): the T1 shas and digests under the corrected harness with the owner's cap adjustment, sourced by run.sh and the t1b judges; a scrubbed pin pins nothing",
	"test/live/autosteer/t1c/pins.txt":                                "the T1c pre-registration pins (the file's header, deltas 1 to 6, and delta 7 appended before the G runs at the new pin): T1b's shas and digests with the G arm pinned to the 0e630250 engine freeze, then to the engine M1 closes on, sourced by run.sh and the t1c judges; a scrubbed pin pins nothing",
	"internal/platform/testdata/snapshot/.pgraph/pgraph.digests":      "the sidecar the real pgraph wrote for the snapshot fixture (W-M1.2): the SHA-256 of each Go file and of the sibling sidecars is the vouch chain TestReadSnapshot_ReadsWhatPgraphWrote reads whole, so a scrubbed digest is a refused fixture, not a cleaner one",
	"internal/platform/testdata/snapshot/.pgraph/pgraph.symbols":      "the sidecar the real pgraph wrote for the snapshot fixture (W-M1.2): the SHA-256 of each Go file and of the sibling sidecars is the vouch chain TestReadSnapshot_ReadsWhatPgraphWrote reads whole, so a scrubbed digest is a refused fixture, not a cleaner one",
	"internal/platform/testdata/snapshot/.pgraph/pgraph.walk":         "the sidecar the real pgraph wrote for the snapshot fixture (W-M1.2): the SHA-256 of each Go file and of the sibling sidecars is the vouch chain TestReadSnapshot_ReadsWhatPgraphWrote reads whole, so a scrubbed digest is a refused fixture, not a cleaner one",
	"internal/graph/lang/gopg/testdata/shapes/.pgraph/pgraph.digests": "the sidecar the real pgraph wrote for the shapes fixture (W-M1.3): the SHA-256 of each Go file and of the sibling sidecars is the vouch chain TestGopg_RecordsForFixture reads whole, so a scrubbed digest is a refused fixture, not a cleaner one",
	"internal/graph/lang/gopg/testdata/shapes/.pgraph/pgraph.symbols": "the sidecar the real pgraph wrote for the shapes fixture (W-M1.3): the SHA-256 of each Go file and of the sibling sidecars is the vouch chain TestGopg_RecordsForFixture reads whole, so a scrubbed digest is a refused fixture, not a cleaner one",
	"internal/graph/lang/gopg/testdata/shapes/.pgraph/pgraph.walk":    "the sidecar the real pgraph wrote for the shapes fixture (W-M1.3): the SHA-256 of each Go file and of the sibling sidecars is the vouch chain TestGopg_RecordsForFixture reads whole, so a scrubbed digest is a refused fixture, not a cleaner one",
}

// h113Sessions holds internal/graph/session's fixtures: stores cut from
// recorded eval runs (PROVENANCE.txt), the goldens folded from them, and the
// SHA-256 sums that pin the stores.
const h113Sessions = "internal/graph/session/testdata/sessions/"

// h113Typed excuses the secret shapes a store format puts in those fixtures,
// by place: a kind of file, as a path.Match pattern; the JSON member the
// value is read under, "" for a line of text; the class of value allowed
// there (hygiene.Finding.Class), never empty, since a row that took any value
// would excuse a token under its member; and the reason. A steer line's
// id is 16 bytes of crypto/rand in hex (steer.newID), a token by design, so
// the entropy test cannot tell it from a credential and only its place can.
// As h113Contracts, an allow list and not a skip: only a secret shape is
// excused, the same value under another member, of another class or in
// another kind of file still fails, and a row that excuses nothing fails as
// stale.
var h113Typed = []h113Row{
	{h113Sessions + "stores/*/runs/*/steer.ndjson", "id", "32 lowercase hex digits",
		"steer.Line.ID, a steer line's id: 16 bytes of crypto/rand in hex (steer.newID)"},
	{h113Sessions + "stores/*/runs/*/steer.ndjson", "key", "a gate id",
		"steer.Line.Key, the finding's dedupe key, which for a gate's finding is the gate id: learned:<kind>:<runner>@<location> or inferred:test@<digits>"},
	{h113Sessions + "golden/fold-v1/*.json", "id", "32 lowercase hex digits",
		"a steer line's id as the fold keeps it: a delivered line's LineRef.ID, and the through Mark's ID when the last event is a line"},
	{h113Sessions + "golden/fold-v1/*.json", "key", "a gate id",
		"a steer line's dedupe key as the fold keeps it: a delivered line's LineRef.Key, a gate id as in the steer file"},
	{h113Sessions + "SHA256SUMS.txt", "", "a checksum line",
		"the SHA-256 of each store file beside its path, as shasum -a 256 writes them; PROVENANCE.txt pins this file by its own sum"},
}

// h113Row is one allow-list row of h113Typed.
type h113Row struct{ kind, member, class, why string }

// h113TypedRow is the index of the row of rows that excuses f, or -1. A row
// with no class excuses nothing, so it is reported stale and never takes any
// value under its member.
func h113TypedRow(rows []h113Row, f hygiene.Finding) int {
	if f.What != "a secret shape" {
		return -1
	}
	for i, r := range rows {
		if ok, _ := path.Match(r.kind, f.Path); ok && f.Member == r.member && r.class != "" && f.Class == r.class {
			return i
		}
	}
	return -1
}

// h113Problems is what H-113 fails on, given what the scan read and found:
// each finding neither allow list excuses, and each entry of either list
// that excuses nothing.
func h113Problems(scanned []string, found []hygiene.Finding) []string {
	var problems []string
	for name := range h113Contracts {
		if !slices.Contains(scanned, name) {
			problems = append(problems, fmt.Sprintf("h113Contracts names %s, which the scan did not read: a stale entry", name))
		}
	}
	used := make([]bool, len(h113Typed))
	for _, f := range found {
		if _, ok := h113Contracts[f.Path]; ok && f.What == "a secret shape" {
			continue
		}
		if i := h113TypedRow(h113Typed, f); i >= 0 {
			used[i] = true
			continue
		}
		problems = append(problems, fmt.Sprintf("%s -- run go run ./test/tools/scrub -w on it, read the result, and commit that", f))
	}
	for i, r := range h113Typed {
		if !used[i] {
			problems = append(problems, fmt.Sprintf("h113Typed row %s, member %q, class %q excuses nothing: a stale row", r.kind, r.member, r.class))
		}
	}
	return problems
}

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
	for _, p := range h113Problems(scanned, found) {
		t.Error(p)
	}
}

// TestH113_ATypedRowExcusesItsMemberAndClassInItsKindOfFile: a typed row
// excuses a secret shape under its member, of its class, in its kind of file,
// and nothing else: the value under another member fails, beside itself
// under its own member too; a token or a longer hex run under the member
// fails; the value in another kind of file fails; another shape under key
// fails; a gate id under key passes, in a steer file and in a golden, and a
// token under key or inside a gate id's form fails; a row with no class
// excuses nothing; and a row nothing matches is reported as stale.
func TestH113_ATypedRowExcusesItsMemberAndClassInItsKindOfFile(t *testing.T) {
	id := h112Stray[:32] // 32 lowercase hex digits, a steer line id's class
	run := h113Sessions + "stores/r/runs/" + h112P1 + "/"
	sums := h113Sessions + "SHA256SUMS.txt"
	golden := h113Sessions + "golden/fold-v1/r.json"
	// A learned gate id from the steer fixtures, which the scan flags in the
	// user:token@ form; split so no source line holds that form whole.
	learned := "learned:build:" + "31f678efefe9" + "@" + "7af2a8b4a6a2"
	for _, tc := range []struct {
		name, file, body string
		excused          bool
	}{
		{"a steer line's id", run + "steer.ndjson", `{"id":"` + id + `"}`, true},
		{"a checksum line", sums, h112Leaf + "  stores/r/runs/" + h112P1 + "/steer.ndjson", true},
		{"the id under another member", run + "steer.ndjson", `{"note":"` + id + `"}`, false},
		{"the id under its member and another", run + "steer.ndjson", `{"id":"` + id + `","note":"` + id + `"}`, false},
		{"a token under the member", run + "steer.ndjson", `{"id":"` + h112Bearer + `"}`, false},
		{"64 hex digits under the member", run + "steer.ndjson", `{"id":"` + h112Stray + `"}`, false},
		{"the id in another kind of file", run + "records.ndjson", `{"id":"` + id + `"}`, false},
		{"an email address under key", run + "steer.ndjson", `{"key":"mail alice.liddell@acme-corp.io"}`, false},
		{"a checksum line with one space", sums, h112Leaf + " stores/r/runs/" + h112P1 + "/steer.ndjson", false},
		{"a token under key in a steer file", run + "steer.ndjson", `{"key":"` + h112Bearer + `"}`, false},
		{"a token under key in a golden", golden, `{"key":"` + h112Bearer + `"}`, false},
		{"a learned gate id under key in a steer file", run + "steer.ndjson", `{"key":"` + learned + `"}`, true},
		{"a learned gate id under key in a golden", golden, `{"key":"` + learned + `"}`, true},
		{"the inferred gate id under key in a steer file", run + "steer.ndjson", `{"key":"inferred:test` + `@285"}`, true},
		{"a token as a learned gate id's runner", run + "steer.ndjson", `{"key":"learned:build:` + h112Bearer + `@` + learned[len(learned)-12:] + `"}`, false},
		{"a learned gate id of a kind no gate has", run + "steer.ndjson", `{"key":"learned:token` + learned[len("learned:build"):] + `"}`, false},
		{"a token after the inferred gate id's at sign", run + "steer.ndjson", `{"key":"inferred:test` + `@` + h112Bearer + `"}`, false},
	} {
		found := hygiene.Scan(tc.file, []byte(tc.body+"\n"))
		if len(found) == 0 {
			t.Errorf("%s: premise: the scan finds nothing", tc.name)
			continue
		}
		excused := true
		for _, f := range found {
			excused = excused && h113TypedRow(h113Typed, f) >= 0
		}
		if excused != tc.excused {
			t.Errorf("%s: excused = %v, want %v (%v)", tc.name, excused, tc.excused, found)
		}
	}

	// A row with no class excuses nothing, not even a token under its member.
	noClass := []h113Row{{h113Sessions + "stores/*/runs/*/steer.ndjson", "key", "", "a row with no class"}}
	tokens := hygiene.Scan(run+"steer.ndjson", []byte(`{"key":"`+h112Bearer+`"}`+"\n"))
	if len(tokens) == 0 {
		t.Error("a row with no class: premise: the scan finds nothing")
	}
	for _, f := range tokens {
		if h113TypedRow(noClass, f) >= 0 {
			t.Errorf("a row with no class excuses %v", f)
		}
	}

	var contracts []string
	for name := range h113Contracts {
		contracts = append(contracts, name)
	}
	// The steer id uses row 0 alone, so every other row is stale and row 0
	// is not.
	found := hygiene.Scan(run+"steer.ndjson", []byte(`{"id":"`+id+`"}`+"\n"))
	problems := h113Problems(contracts, found)
	if len(problems) != len(h113Typed)-1 {
		t.Errorf("one row used: %d problems, want the %d unused rows as stale: %v", len(problems), len(h113Typed)-1, problems)
	}
	r := h113Typed[0]
	usedRow := fmt.Sprintf("h113Typed row %s, member %q, class %q excuses nothing: a stale row", r.kind, r.member, r.class)
	for _, p := range problems {
		if p == usedRow || !strings.HasSuffix(p, "excuses nothing: a stale row") {
			t.Errorf("not an unused row's stale report: %s", p)
		}
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
		{"account and organisation ids as values, as a config file holds them", "test/fixtures/demo/config.json",
			[]byte(`{"oauthAccount":{"accountUuid":"` + h112Session + `","organizationUuid":"` + h112Session + `"}}`), "test/fixtures/demo/config.json:1", h112Session},
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
