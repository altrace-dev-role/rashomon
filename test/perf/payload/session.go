package payload

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The identity a generated store is given, fixed where a fresh store's is
// random. On go1.27, decoding a store allocates by the content of its strings
// (encoding/json/v2 keeps a 256-slot string cache), and a record's digests are
// keyed by the install key: with a random one, BenchmarkRecap/10 moved by up
// to 9% and BenchmarkBuild/188 by up to 6% between processes, noise a gate at
// +10% cannot carry. Pinned, the spread is under 1%.
const (
	KeyHex    = "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a"
	InstallID = "a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5"
)

// CWD is the working directory a generated session's records name.
const CWD = cwd

// FinalMessage is a generated session's final assistant message. It claims a
// pass and names no failure, so a digest sets the last turn's failed calls
// against it.
const FinalMessage = "Done. Open now checks the key length before it returns the key, the new " +
	"test covers a short key, and go test ./internal/store/... -count=1 passes. Ready for review."

// Prompts is how many prompts the benchmarks split a generated session's
// calls over: the last turn, which a digest reads, is a quarter of them.
const Prompts = 4

// durationMS is how long each generated call ran: its execution and post
// coverage record are written that long after its declaration.
const durationMS = 1200

// Pin gives the store root dir the fixed identity before anything opens a
// store there, so store.Open reads KeyHex and InstallID rather than minting
// random ones.
func Pin(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	files := map[string]string{
		"install.key":  KeyHex + "\n",
		"install.json": fmt.Sprintf(`{"install_id":%q,"created_at_unix_ms":1791380383430,"schema_version":%d}`+"\n", InstallID, store.SchemaVersion),
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// Turn is what WriteSession put in the session's last turn, the turn a digest
// built with no prompt id reads.
type Turn struct {
	PromptID   string
	Calls      int
	Executions int
	Failed     int
	RefusedID  string
	Bytes      int64 // the whole session: records.ndjson plus coverage.ndjson
}

func (t *Turn) add(c call, id string) {
	t.Calls++
	switch {
	case c.refused:
		t.RefusedID = id
	case c.failed:
		t.Executions++
		t.Failed++
	default:
		t.Executions++
	}
}

// PromptID is the p-th prompt's id, UUID-shaped as Claude Code's are.
func PromptID(p int) string {
	return fmt.Sprintf("72fea482-07bb-4309-8de3-%012d", p+1)
}

// call is one generated call: its tool, its input and how it ended.
type call struct {
	tool, input     string
	failed, refused bool
}

// mix is the call mix a generated session cycles through: Bash, Read and
// Edit, with a test run that fails, an edit, and the same run passing.
var mix = []call{
	{tool: "Bash", input: `{"command":"git status --short && git diff --stat","description":"Show working tree status"}`},
	{tool: "Read", input: `{"file_path":"/Users/dev/src/project/internal/store/store.go"}`},
	{tool: "Bash", input: `{"command":"go test ./internal/store/... -count=1","description":"Run the store tests"}`, failed: true},
	{tool: "Edit", input: `{"file_path":"/Users/dev/src/project/internal/store/store.go","old_string":"return key, nil","new_string":"return key, checkKey(key)","replace_all":false}`},
	{tool: "Bash", input: `{"command":"go test ./internal/store/... -count=1","description":"Run the store tests"}`},
	{tool: "Read", input: `{"file_path":"/Users/dev/src/project/internal/store/store_test.go"}`},
	{tool: "Edit", input: `{"file_path":"/Users/dev/src/project/internal/store/store_test.go","old_string":"want: nil","new_string":"want: errShortKey","replace_all":false}`},
	{tool: "Bash", input: `{"command":"grep -rn 'func Open' internal/store","description":"Find Open"}`},
}

// callAt is call i of a session of calls. The last two are a build that fails
// and a call the user refused, so the last turn holds both findings; the build
// is not a test run, so it pairs with no other run as test bending.
func callAt(i, calls int) call {
	switch i {
	case calls - 2:
		return call{tool: "Bash", input: `{"command":"go build ./...","description":"Build everything"}`, failed: true}
	case calls - 1:
		return call{tool: "Bash", input: `{"command":"rm -rf /Users/dev/src/project/build","description":"Remove the build directory"}`, refused: true}
	}
	return mix[i%len(mix)]
}

// WriteSession pins a store at dir and writes into it a session of calls tool
// calls, split evenly over prompts prompts, as the hooks would have written
// them: one start coverage record, then per call a declaration, a terminal and
// an execution in seq order, with a call and a post coverage record. It writes
// the files directly, because through the append API each record costs two
// fsyncs, about an hour for 20,000 calls. calls must be at least twice
// prompts, so the last turn holds the last two calls. Before it returns it
// reads the session back with the store's own reader.
func WriteSession(dir string, calls, prompts int) (Turn, error) {
	if prompts < 1 || calls < 2*prompts {
		return Turn{}, fmt.Errorf("payload: a session of %d calls over %d prompts: need at least two calls a prompt", calls, prompts)
	}
	if err := Pin(dir); err != nil {
		return Turn{}, err
	}
	st, err := store.Open(dir)
	if err != nil {
		return Turn{}, err
	}
	start := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC).UnixMilli()
	w := &writer{key: st.Key(), cov: store.Coverage{
		Type: store.TypeCoverage, SchemaVersion: store.SessionSchemaVersion, SessionID: Session,
		InstallID: st.InstallID(), State: store.StateVerified, HookEntry: store.EntryPresentSettings,
		Probe: store.ProbeFresh, CWD: cwd,
	}}
	w.coverage(store.PhaseStart, start-1000)

	turn := Turn{PromptID: PromptID(prompts - 1)}
	for i := 0; i < calls; i++ {
		c := callAt(i, calls)
		id, prompt := fmt.Sprintf("toolu_01%022d", i), PromptID(i*prompts/calls)
		w.call(c, id, prompt, start+int64(i)*2000)
		if prompt == turn.PromptID {
			turn.add(c, id)
		}
	}
	if err := w.flush(st); err != nil {
		return Turn{}, err
	}
	turn.Bytes = int64(w.recs.Len() + w.covs.Len())
	if err := check(st, calls); err != nil {
		return Turn{}, err
	}
	return turn, nil
}

// writer holds a generated session's records and coverage as they are
// written, each line as the store's own writer marshals it. err is the first
// line that would not marshal.
type writer struct {
	key        []byte
	recs, covs bytes.Buffer
	cov        store.Coverage
	seq        int64
	err        error
}

func (w *writer) write(buf *bytes.Buffer, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		if w.err == nil {
			w.err = err
		}
		return
	}
	buf.Write(data)
	buf.WriteByte('\n')
}

func (w *writer) coverage(phase string, at int64) {
	w.cov.Phase, w.cov.RecordedAtMS = phase, at
	w.write(&w.covs, w.cov)
}

// call writes c as the two hooks would: the declaration, its terminal and a
// call coverage record, then, unless c was refused, its execution and a post
// coverage record.
func (w *writer) call(c call, id, prompt string, at int64) {
	w.seq++
	d := store.Declaration{
		Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion, Seq: w.seq, RecordedAtMS: at,
		ToolUseID: id, SessionID: Session, PromptID: &prompt, TranscriptPath: transcript,
		PermissionMode: "default", ToolName: c.tool,
	}
	in := json.RawMessage(c.input)
	d.Shape = shape.Derive(d.ToolName, in, w.key)
	d.Hosts, d.SSHHosts = shape.Hosts(d.ToolName, in)
	d.CWDDigest, d.RulesVersion = shape.CWDDigest(w.key, CWD), shape.RulesVersion
	if label := shape.Label(d.ToolName, in); label != "" {
		d.FileLabel = &label
	}
	w.write(&w.recs, d)
	w.seq++
	termSeq := w.seq
	w.write(&w.recs, store.Terminal{
		Type: store.TypeTerminal, SchemaVersion: store.SchemaVersion, Seq: &termSeq, RecordedAtMS: at + 5,
		ToolUseID: id, SessionID: Session, Outcome: store.OutcomeOK,
	})
	w.coverage(store.PhaseCall, at+5)
	if c.refused {
		return
	}
	// The post path derives the executed shape from the same input under the
	// same key, so it is the declaration's.
	w.seq++
	execSeq, duration := w.seq, int64(durationMS)
	x := store.Execution{
		Type: store.TypeExecution, SchemaVersion: store.SchemaVersion, Seq: &execSeq, RecordedAtMS: at + duration,
		ToolUseID: id, SessionID: Session, ToolName: c.tool, Outcome: store.ExecOK, DurationMS: &duration,
		ExecutedDigest: d.Shape.Digest, StatusMasked: d.Shape.StatusMasked, RunnerDigest: d.Shape.RunnerDigest,
		RulesVersion: d.RulesVersion,
	}
	if c.failed {
		code, interrupt := 1, false
		x.Outcome, x.ExitCode, x.IsInterrupt = store.ExecFailed, &code, &interrupt
	}
	w.write(&w.recs, x)
	w.coverage(store.PhasePost, at+duration)
}

// flush writes the session's files into its run directory.
func (w *writer) flush(st *store.Store) error {
	if w.err != nil {
		return w.err
	}
	dir := st.RunDir(Session)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	files := map[string][]byte{
		store.FileRecords:  w.recs.Bytes(),
		store.FileCoverage: w.covs.Bytes(),
		store.FileSeq:      []byte(strconv.FormatInt(w.seq, 10) + "\n"),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// check reads the session back with the store's reader: every line taken, a
// declaration and a terminal per call, an execution for every call but the
// refused one, and two coverage records per call, the start's included.
func check(st *store.Store, calls int) error {
	run, err := st.ReadRun(Session)
	if err != nil {
		return err
	}
	if run.Skipped != 0 || len(run.Declarations) != calls || len(run.Terminals) != calls ||
		len(run.Executions) != calls-1 || len(run.Coverage) != 2*calls {
		return fmt.Errorf("payload: generated session reads back as %d declarations, %d terminals, %d executions, %d coverage, %d skipped; want %d, %d, %d, %d, 0",
			len(run.Declarations), len(run.Terminals), len(run.Executions), len(run.Coverage), run.Skipped,
			calls, calls, calls-1, 2*calls)
	}
	return nil
}
