package gather

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

const (
	kProject = "/work/app"
	kBase    = int64(1_761_700_000_000)
)

// kg is a gate of a checkpoint fixture: name, required, state.
type kg struct {
	name  string
	req   bool
	state string
}

// kc is a checkpoint fixture.
type kc struct {
	session, project string
	at, seq          int64
	trust, digest    string
	omitted          int
	gates            []kg
}

func (c kc) line() string {
	project := c.project
	if project == "" {
		project = kProject
	}
	gates := make([]string, len(c.gates))
	for i, g := range c.gates {
		gates[i] = fmt.Sprintf(`{"gate":%q,"required":%t,"state":%q}`, g.name, g.req, g.state)
	}
	trust, digest := "null", "null"
	if c.trust != "" {
		trust = fmt.Sprintf("%q", c.trust)
	}
	if c.digest != "" {
		digest = fmt.Sprintf("%q", c.digest)
	}
	return fmt.Sprintf(`{"type":"verify","schema_version":%d,"recorded_at_unix_ms":%d,"session_id":%q,"project_root":%q,"point_seq":%d,`+
		`"spec_digest":%s,"spec_trust":%s,"record_green":false,"gates":[%s],"gates_omitted":%d,"integrity":{"unresolved":[]},"integrity_omitted":0}`,
		store.EventSchemaVersion, c.at, c.session, project, c.seq, digest, trust, strings.Join(gates, ","), c.omitted)
}

// kStore is an empty store.
func kStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// seed writes a run holding the checkpoints, its records file last written
// at the last one's time.
func seed(t *testing.T, st *store.Store, session string, cps ...kc) {
	t.Helper()
	dir := st.RunDir(session)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	var body []byte
	for _, c := range cps {
		c.session = session
		body = append(body, c.line()+"\n"...)
	}
	path := filepath.Join(dir, store.FileRecords)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	at := time.UnixMilli(cps[len(cps)-1].at)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

func approvedLedger(t *testing.T, st *store.Store, approvedAt time.Time, ids ...string) {
	t.Helper()
	dir := strings.TrimSuffix(baseline.FileName(kProject), baseline.FileExt)
	l, err := learn.Open(st.Root(), dir, approvedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.Import(strings.NewReader(`{"schema":"lessons/1","lessons":[
	{"id":"go-test-cwd","tier":"proven","text":"run go test from the module root","since_unix_ms":1},
	{"id":"npm-ci","tier":"probation","text":"npm ci before npm test","since_unix_ms":2},
	{"id":"proposed-one","tier":"proposed","text":"use make","since_unix_ms":3}]}`), approvedAt); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if err := l.Approve(id, approvedAt); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
}

func bindSpec(t *testing.T, st *store.Store, bs ...spec.Binding) {
	t.Helper()
	key := make([]byte, spec.TrustKeyBytes)
	for i := range key {
		key[i] = byte(i + 1)
	}
	data, err := spec.EncodeBindings(&spec.Bindings{Bindings: bs}, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.Root(), spec.TrustFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.Root(), spec.TrustKeyFile), key, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeIdx(t *testing.T, st *store.Store, entries ...verify.IndexEntry) {
	t.Helper()
	if _, err := verify.WriteIndex(verify.IndexPath(st.Root(), kProject), &verify.ProjectIndex{ProjectKey: kProject, Entries: entries}); err != nil {
		t.Fatal(err)
	}
}

func tree(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(p string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %d %x\n", p, info.ModTime().UnixNano(), data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

var kNow = time.UnixMilli(kBase + 10*3_600_000)

// TestRP2_RetainedReadsTheFourStores (card REPORT1b R1): an index with two
// required families and one not, one binding for this project and one for
// another, and a ledger with a proven, a probation and a proposed lesson. The
// retained block names two families, this project's spec as bound with its
// file digest and time, the last Stop's trust with the compiled digest, and
// the two approved lessons, and nothing of the other project. A binding file
// whose MAC fails lists no spec; no family established is the prior. Break:
// read another project's binding, or print a count for the prior.
func TestRP2_RetainedReadsTheFourStores(t *testing.T) {
	st := kStore(t)
	seed(t, st, "s1", kc{at: kBase + 1000, seq: 5, trust: "trusted", digest: "sha256:5e6f708182930000", gates: []kg{{"learned:go-test", true, "green"}}})
	writeIdx(t, st,
		verify.IndexEntry{Family: "a1", Kind: "test", Sessions: 3, FirstSeen: 1, LastSeen: kBase},
		verify.IndexEntry{Family: "b2", Kind: "build", Sessions: 2, FirstSeen: 1, LastSeen: kBase},
		verify.IndexEntry{Family: "c3", Kind: "lint", Sessions: 1, FirstSeen: 1, LastSeen: kBase})
	bindSpec(t, st,
		spec.Binding{ProjectRoot: kProject, SpecPath: kProject + "/.claude/rashomon/spec.json", SpecDigest: "sha256:ab12cd34ef56aaaa", BoundAtMS: kBase + 500},
		spec.Binding{ProjectRoot: "/work/other", SpecPath: "/work/other/spec.json", SpecDigest: "sha256:ffff", BoundAtMS: 1})
	approvedLedger(t, st, time.UnixMilli(kBase-1000), "go-test-cwd", "npm-ci")

	k := knowledgeOf(st, nil, "s1", kNow)
	if k == nil {
		t.Fatal("no knowledge for a session with a checkpoint")
	}
	r := k.Retained
	if r.Prior || r.RequiredFamilies != 2 {
		t.Errorf("families: prior %v, required %d; want 2 required", r.Prior, r.RequiredFamilies)
	}
	if len(r.Specs) != 1 || r.Specs[0].Path != ".claude/rashomon/spec.json" || r.Specs[0].FileDigest != "sha256:ab12cd34ef56aaaa" || r.Specs[0].BoundAtMS != kBase+500 {
		t.Errorf("specs %+v; want this project's one binding, path relative to the project", r.Specs)
	}
	if st := r.SpecAtLastStop; st == nil || st.Trust != "trusted" || st.Compiled != "sha256:5e6f708182930000" || st.PointSeq != 5 {
		t.Errorf("spec at the last Stop %+v", st)
	}
	if len(r.Lessons) != 2 || r.Lessons[0].ID != "go-test-cwd" || r.Lessons[1].ID != "npm-ci" || r.Lessons[0].Tier != "probation" || r.LedgerUnchecked {
		t.Errorf("lessons %+v unchecked %v; want the two approved", r.Lessons, r.LedgerUnchecked)
	}
	var text strings.Builder
	rep := &report.Report{Sessions: []report.Session{{SessionID: "s1", Knowledge: k}}}
	_ = report.Text(&text, rep)
	for _, want := range []string{"2 gate families required (index)", "spec .claude/rashomon/spec.json bound in spec-trust.json (file sha256:ab12cd34ef56, bound ",
		"read trusted at the last Stop (compiled sha256:5e6f70818293)", "2 lessons approved: go-test-cwd probation, npm-ci probation (learn)"} {
		if !strings.Contains(strings.Join(strings.Fields(text.String()), " "), want) {
			t.Errorf("the text lacks %q:\n%s", want, text.String())
		}
	}
	if strings.Contains(text.String(), "/work/other") {
		t.Errorf("another project's binding is named:\n%s", text.String())
	}

	// A binding file that does not carry the store's MAC lists no spec.
	if err := os.WriteFile(filepath.Join(st.Root(), spec.TrustFile), []byte(`{"bindings":[],"mac":"00"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if k := knowledgeOf(st, nil, "s1", kNow); len(k.Retained.Specs) != 0 || !k.Retained.BindingsUnsigned {
		t.Errorf("unsigned bindings: specs %+v unsigned %v; want none, unsigned", k.Retained.Specs, k.Retained.BindingsUnsigned)
	}

	// No family established: the fail-closed prior, never "0 gate families".
	st2 := kStore(t)
	seed(t, st2, "s1", kc{at: kBase + 1000, seq: 5})
	writeIdx(t, st2, verify.IndexEntry{Family: "c3", Kind: "lint", Sessions: 1, FirstSeen: 1, LastSeen: kBase})
	k2 := knowledgeOf(st2, nil, "s1", kNow)
	text.Reset()
	_ = report.Text(&text, &report.Report{Sessions: []report.Session{{SessionID: "s1", Knowledge: k2}}})
	if !k2.Retained.Prior || !strings.Contains(text.String(), "no family established; every gate required (index)") || strings.Contains(text.String(), "0 gate famil") {
		t.Errorf("the prior: %+v\n%s", k2.Retained, text.String())
	}
}

// TestRP2b_DiscoveredFromConsecutiveCheckpoints (card REPORT1b R1): three
// checkpoints with the spec untrusted, trusted, trusted and a gate red, red,
// green name "spec trusted at the Stop after #2" and the gate's one change at
// #3. A Required flip with the state unchanged prints nothing; a fourth
// checkpoint that cut gates turns the gate line into the one count line.
// Break: compare a checkpoint with itself, compare Required, or drop the cut
// check.
func TestRP2b_DiscoveredFromConsecutiveCheckpoints(t *testing.T) {
	st := kStore(t)
	seed(t, st, "s1",
		kc{at: kBase + 1000, seq: 1, trust: "untrusted", digest: "sha256:aaaaaaaaaaaa1", gates: []kg{{"learned:go-test", true, "red"}, {"npm", true, "green"}}},
		kc{at: kBase + 2000, seq: 2, trust: "trusted", digest: "sha256:bbbbbbbbbbbb2", gates: []kg{{"learned:go-test", true, "red"}, {"npm", false, "green"}}},
		kc{at: kBase + 3000, seq: 3, trust: "trusted", digest: "sha256:bbbbbbbbbbbb2", gates: []kg{{"learned:go-test", true, "green"}, {"npm", true, "green"}}})
	k := knowledgeOf(st, nil, "s1", kNow)
	if len(k.Discovered) != 1 || !strings.HasPrefix(k.Discovered[0].What, "spec trusted at the Stop after #2 (compiled sha256:bbbbbbbbbbbb2)") {
		t.Errorf("discovered %+v; want the spec trusted at the Stop after #2", k.Discovered)
	}
	if len(k.GateStates) != 1 || k.GateStates[0].Gate != "learned:go-test" || k.GateStates[0].Changes != 1 ||
		k.GateStates[0].LastFrom != "red" || k.GateStates[0].LastTo != "green" || k.GateStates[0].LastSeq != 3 {
		t.Errorf("gate states %+v; want learned:go-test 1 change, red to green at #3, and nothing for npm's Required flip", k.GateStates)
	}
	st = kStore(t)
	seed(t, st, "s1",
		kc{at: kBase + 1000, seq: 1, gates: []kg{{"go", true, "red"}}},
		kc{at: kBase + 2000, seq: 2, gates: []kg{{"go", true, "green"}}},
		kc{at: kBase + 3000, seq: 3, omitted: 2, gates: []kg{{"go", true, "red"}}})
	k = knowledgeOf(st, nil, "s1", kNow)
	var text strings.Builder
	_ = report.Text(&text, &report.Report{Sessions: []report.Session{{SessionID: "s1", Knowledge: k}}})
	if k.GatesCut != 2 || len(k.GateStates) != 0 || !strings.Contains(text.String(), "gate states at the Stops: not compared; a checkpoint cut up to 2 gates (gates_omitted)") {
		t.Errorf("a cut: %+v\n%s", k, text.String())
	}
}

// TestRP2c_InvalidatedAgainstTheBaseline (card REPORT1b R1): the baseline
// trusted at compiled digest A and this session's first Stop untrusted at B
// is an invalidation, A to B, at the first Stop. Trusted at A then trusted at
// B is "spec changed, now trusted" under discovered. A later session of the
// project, with checkpoints after this one's last, is not the baseline. A
// quarantine stamped inside the span prints; one before it does not. Break:
// read held-to-held as invalidated, take the newest other session as the
// baseline, or drop the span check.
func TestRP2c_InvalidatedAgainstTheBaseline(t *testing.T) {
	st := kStore(t)
	seed(t, st, "base", kc{at: kBase - 5000, seq: 9, trust: "trusted", digest: "sha256:AAAAAAAAAAAA", gates: []kg{{"go", true, "green"}}})
	seed(t, st, "s1", kc{at: kBase + 1000, seq: 1, trust: "untrusted", digest: "sha256:BBBBBBBBBBBB", gates: []kg{{"go", true, "red"}}})
	seed(t, st, "later", kc{at: kBase + 9000, seq: 4, trust: "trusted", digest: "sha256:CCCCCCCCCCCC", gates: []kg{{"go", true, "green"}}})
	k := knowledgeOf(st, nil, "s1", kNow)
	if k.Baseline == nil || k.Baseline.SessionID != "base" || k.Baseline.PointSeq != 9 {
		t.Fatalf("baseline %+v; want session base at #9, not the later session", k.Baseline)
	}
	if len(k.Invalidated) != 1 || !strings.Contains(k.Invalidated[0].What, "spec trust, trusted to untrusted at the Stop after #1 (compiled sha256:AAAAAAAAAAAA to sha256:BBBBBBBBBBBB)") {
		t.Errorf("invalidated %+v", k.Invalidated)
	}
	if len(k.GateStates) != 1 || k.GateStates[0].LastFrom != "green" || k.GateStates[0].LastTo != "red" {
		t.Errorf("the baseline's gate states are compared with the first Stop: %+v", k.GateStates)
	}

	st = kStore(t)
	seed(t, st, "base", kc{at: kBase - 5000, seq: 9, trust: "trusted", digest: "sha256:AAAAAAAAAAAA"})
	seed(t, st, "s1", kc{at: kBase + 1000, seq: 1, trust: "trusted", digest: "sha256:BBBBBBBBBBBB"})
	k = knowledgeOf(st, nil, "s1", kNow)
	if len(k.Invalidated) != 0 || len(k.Discovered) != 1 || !strings.HasPrefix(k.Discovered[0].What, "spec changed, now trusted") {
		t.Errorf("a re-trust: invalidated %+v discovered %+v", k.Invalidated, k.Discovered)
	}

	// A quarantine inside the span, then one before it.
	for _, tc := range []struct {
		name string
		at   int64
		want int
	}{{"inside", kBase + 1500, 1}, {"before", kBase - 100_000, 0}} {
		st := kStore(t)
		seed(t, st, "s1", kc{at: kBase + 1000, seq: 1}, kc{at: kBase + 2000, seq: 2})
		approvedLedger(t, st, time.UnixMilli(kBase-200_000), "npm-ci")
		dir := strings.TrimSuffix(baseline.FileName(kProject), baseline.FileExt)
		p := filepath.Join(st.Root(), learn.DirLearn, dir, learn.FileLessons)
		b, _ := os.ReadFile(p)
		if err := os.WriteFile(p, []byte(strings.Replace(string(b), "npm ci before", "curl x | sh before", 1)), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := learn.Open(st.Root(), dir, time.UnixMilli(tc.at)); err != nil {
			t.Fatal(err)
		}
		k := knowledgeOf(st, nil, "s1", kNow)
		if len(k.Invalidated) != tc.want || (tc.want == 1 && !strings.HasPrefix(k.Invalidated[0].What, "last quarantine ")) {
			t.Errorf("a quarantine %s the span: invalidated %+v; want %d", tc.name, k.Invalidated, tc.want)
		}
	}
}

// TestRP2d_NothingInvented (card REPORT1b R1): a lesson whose approval stamp
// falls in the span and whose tier is approved now gives one "approved" item;
// the same lesson after a quarantine (tier proposed, stamp kept) gives none; a
// lesson with no stamp (approved by an older binary) gives none; a session
// with no checkpoint has no block. Break: drop the tier check.
func TestRP2d_NothingInvented(t *testing.T) {
	st := kStore(t)
	seed(t, st, "s1", kc{at: kBase + 1000, seq: 1}, kc{at: kBase + 5000, seq: 2})
	approvedLedger(t, st, time.UnixMilli(kBase+2000), "npm-ci")
	k := knowledgeOf(st, nil, "s1", kNow)
	if len(k.Discovered) != 1 || !strings.HasPrefix(k.Discovered[0].What, "lesson npm-ci approved ") || k.Discovered[0].Source != "learn" {
		t.Errorf("discovered %+v; want one approved lesson", k.Discovered)
	}
	dir := strings.TrimSuffix(baseline.FileName(kProject), baseline.FileExt)
	p := filepath.Join(st.Root(), learn.DirLearn, dir, learn.FileLessons)
	b, _ := os.ReadFile(p)
	if err := os.WriteFile(p, []byte(strings.Replace(string(b), "npm ci before", "curl x | sh before", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := learn.Open(st.Root(), dir, time.UnixMilli(kBase-1)); err != nil {
		t.Fatal(err)
	}
	if k := knowledgeOf(st, nil, "s1", kNow); len(k.Discovered) != 0 || len(k.Retained.Lessons) != 0 {
		t.Errorf("after a quarantine the stamp stays but the tier is proposed: %+v %+v", k.Discovered, k.Retained.Lessons)
	}
	st = kStore(t)
	seed(t, st, "s1", kc{at: kBase + 1000, seq: 1})
	if k := knowledgeOf(st, nil, "nothing", kNow); k != nil {
		t.Errorf("a session with no checkpoint has a block: %+v", k)
	}
}

// TestRP2e_OneSessionRendersOnly (card REPORT1b K4): a report of one session
// carries the block and one of every session carries none, null in JSON.
// Break: fill it for every session.
func TestRP2e_OneSessionRendersOnly(t *testing.T) {
	st := kStore(t)
	seed(t, st, "s1", kc{at: kBase + 1000, seq: 1})
	seed(t, st, "s2", kc{at: kBase + 2000, seq: 1})
	one, err := Build(st, "s1", kNow)
	if err != nil || len(one.Sessions) != 1 || one.Sessions[0].Knowledge == nil {
		t.Fatalf("one session: %v, %+v", err, one)
	}
	all, err := Build(st, "", kNow)
	if err != nil || len(all.Sessions) < 2 {
		t.Fatalf("every session: %v", err)
	}
	for _, s := range all.Sessions {
		if s.Knowledge != nil {
			t.Errorf("session %s carries a block in a report of every session", s.SessionID)
		}
	}
}

// TestRP2f_EveryItemNamesItsSource (card REPORT1b R1): every discovered and
// invalidated item names a store, a checkpoint "#<seq>" or the baseline, and
// the block's retained clauses name their store.
func TestRP2f_EveryItemNamesItsSource(t *testing.T) {
	st := kStore(t)
	seed(t, st, "base", kc{at: kBase - 5000, seq: 9, trust: "trusted", digest: "sha256:AAAAAAAAAAAA"})
	seed(t, st, "s1", kc{at: kBase + 1000, seq: 1, trust: "untrusted"}, kc{at: kBase + 2000, seq: 2, trust: "trusted", digest: "sha256:CCCCCCCCCCCC"})
	writeIdx(t, st, verify.IndexEntry{Family: "a1b2c3d4e5", Kind: "test", Sessions: 1, FirstSeen: kBase + 1500, LastSeen: kBase + 1500})
	approvedLedger(t, st, time.UnixMilli(kBase+1200), "go-test-cwd")
	k := knowledgeOf(st, nil, "s1", kNow)
	if len(k.Discovered)+len(k.Invalidated) < 4 {
		t.Fatalf("premise: want the trust change, the family and the lesson, got %+v %+v", k.Discovered, k.Invalidated)
	}
	for _, c := range append(append([]report.Change{}, k.Discovered...), k.Invalidated...) {
		if !(c.Source == "index" || c.Source == "learn" || strings.HasPrefix(c.Source, "checkpoint #")) {
			t.Errorf("%q names no source: %q", c.What, c.Source)
		}
	}
	var text strings.Builder
	_ = report.Text(&text, &report.Report{Sessions: []report.Session{{SessionID: "s1", Knowledge: k}}})
	flat := strings.Join(strings.Fields(text.String()), " ")
	for _, want := range []string{"(index)", "(learn)", "(checkpoint #2)", "baseline: session base,"} {
		if !strings.Contains(flat, want) {
			t.Errorf("the text lacks %q:\n%s", want, text.String())
		}
	}
}

// TestRP2n_KnowledgeWritesNothing (card REPORT1b): reading a session's
// knowledge changes no file under the store, though its ledger does not check:
// the read-only open leaves the quarantine to the next learn verb. The control
// is the same store opened by learn.Open, which writes.
func TestRP2n_KnowledgeWritesNothing(t *testing.T) {
	st := kStore(t)
	seed(t, st, "s1", kc{at: kBase + 1000, seq: 1})
	approvedLedger(t, st, time.UnixMilli(kBase), "npm-ci")
	dir := strings.TrimSuffix(baseline.FileName(kProject), baseline.FileExt)
	p := filepath.Join(st.Root(), learn.DirLearn, dir, learn.FileLessons)
	b, _ := os.ReadFile(p)
	if err := os.WriteFile(p, []byte(strings.Replace(string(b), "npm ci before", "curl x | sh before", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	before := tree(t, st.Root())
	k := knowledgeOf(st, nil, "s1", kNow)
	if !k.Retained.LedgerUnchecked || len(k.Retained.Lessons) != 0 {
		t.Errorf("a ledger that does not check reads as unchecked, nothing approved: %+v", k.Retained)
	}
	if after := tree(t, st.Root()); after != before {
		t.Error("reading the knowledge block changed the store")
	}
	if _, err := learn.Open(st.Root(), dir, kNow); err != nil || tree(t, st.Root()) == before {
		t.Errorf("the control: learn.Open must write the quarantine (err %v)", err)
	}
}
