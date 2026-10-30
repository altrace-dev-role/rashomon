package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/replay"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The §6.2 rows as acceptance tests, from synthetic transcript trees in
// temporary directories, and the invariants DEDUP-1 and DEDUP-2 they all
// serve. Every tree is imported through the importer, which is what B3a will
// call: files in, per-session summaries in a ledger out.

// gateDoc is a session's transcript of one turn per element of turns, each turn
// a prompt, a test run per outcome ("ok", "fail", "timeout", "notrun") and an
// end. Run ids are unique to the session, turn and place.
func gateDoc(sid string, turns ...[]string) *tdoc { return gateDocIDs(sid, sid, 0, turns...) }

// gateDocIDs is gateDoc with the run ids made from idp instead of the session,
// and every line's time moved by shift.
func gateDocIDs(sid, idp string, shift time.Duration, turns ...[]string) *tdoc {
	d := newTdoc()
	d.sid, d.shift = sid, shift
	for ti, outcomes := range turns {
		d.promptLine(fmt.Sprintf("%s-p%d", sid, ti+1))
		for i, o := range outcomes {
			id := fmt.Sprintf("%s-t%d-%d", idp, ti+1, i+1)
			d.uses(id+"m", tBash(id, "python3 -m unittest"))
			switch o {
			case "ok":
				d.results("", tOK(id))
			case "fail":
				d.results("", tFail(id))
			case "timeout":
				d.results("", tResult{id: id, isError: true, text: "Exit code 124\ntimed out", tur: "Error: Exit code 124"})
			case "notrun":
				d.results("", tResult{id: id, isError: true, text: "Exit code 127\nnot found", tur: "Error: Exit code 127"})
			case "toolerr":
				// An error that names no exit code: the call is known to have
				// failed, and not how, so it is not graded Verified.
				d.results("", tResult{id: id, isError: true, text: "the tool broke", tur: "Error: the tool broke"})
			}
		}
		d.ends(fmt.Sprintf("%s-e%d", sid, ti+1), "done")
	}
	return d
}

// ftree is a directory of transcript files.
type ftree struct {
	t    *testing.T
	root string
}

func newTree(t *testing.T) *ftree { return &ftree{t, t.TempDir()} }

func (tr *ftree) write(rel, text string) string {
	tr.t.Helper()
	p := filepath.Join(tr.root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		tr.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		tr.t.Fatal(err)
	}
	return p
}

func (tr *ftree) files() []string {
	tr.t.Helper()
	fs, err := findTranscripts(tr.root)
	if err != nil {
		tr.t.Fatal(err)
	}
	return fs
}

func newTestImporter(t *testing.T, mod func(*importConfig)) *importer {
	t.Helper()
	cfg := importConfig{Scratch: t.TempDir()}
	if mod != nil {
		mod(&cfg)
	}
	im, err := newImporter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return im
}

// the one family every tree's runs belong to.
func theFamily(t *testing.T, im *importer) (string, *familyTotal) {
	t.Helper()
	tot := im.ledger.Totals()
	if len(tot) != 1 {
		var ks []string
		for k := range tot {
			ks = append(ks, k)
		}
		t.Fatalf("%d families, want 1: %v", len(tot), ks)
	}
	for k, v := range tot {
		return k, v
	}
	return "", nil
}

func importAll(t *testing.T, im *importer, tr *ftree) *importResult {
	t.Helper()
	res, err := im.Import(tr.files())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func outcomeOf(res *importResult, im *importer, sid string) sessionOutcome {
	for _, o := range res.Sessions {
		if o.SessionHMAC == im.ledger.ID(sid) {
			return o
		}
	}
	return sessionOutcome{}
}

// reference is the figures of a session held in one file, to hold the rows
// against.
func reference(t *testing.T, doc *tdoc) familyTotal {
	t.Helper()
	tr := newTree(t)
	tr.write("projects/-repo/ref.jsonl", doc.String())
	im := newTestImporter(t, nil)
	importAll(t, im, tr)
	_, f := theFamily(t, im)
	return *f
}

// TestREPLAY27_AnImportedSessionIsCountedOncePerSessionIDWhateverTheFiles holds
// each row of §6.2's table to DEDUP-1 for the family: the number of sessions
// with a run that matches recorded inputs is the number of distinct session
// ids, whatever the number of scans, files, copies, resumes or subagent files.
func TestREPLAY27_AnImportedSessionIsCountedOncePerSessionIDWhateverTheFiles(t *testing.T) {
	const sid = "11111111-0000-4000-8000-000000000001"
	turn1, turn2 := []string{"ok", "fail"}, []string{"ok", "ok"}

	t.Run("the same tree scanned twice", func(t *testing.T) {
		tr := newTree(t)
		tr.write("projects/-repo/a.jsonl", gateDoc(sid, turn1, turn2).String())
		ref := reference(t, gateDoc(sid, turn1, turn2))
		im := newTestImporter(t, nil)
		r1 := importAll(t, im, tr)
		_, once := theFamily(t, im)
		if once.SessionsVerified != 1 || *once != ref {
			t.Fatalf("one scan: %+v, a single file gives %+v", *once, ref)
		}
		// The second scan finds nothing changed and does nothing; a forced one
		// replays it and replaces the entry. Neither adds.
		r2 := importAll(t, im, tr)
		if outcomeOf(r2, im, sid).Status != statusUnchanged || outcomeOf(r1, im, sid).Status != statusImported {
			t.Errorf("statuses %s then %s", outcomeOf(r1, im, sid).Status, outcomeOf(r2, im, sid).Status)
		}
		im.cfg.Rescan = true
		for range 3 {
			r := importAll(t, im, tr)
			if o := outcomeOf(r, im, sid); o.Status != statusImported || !o.Replaced {
				t.Fatalf("a forced rescan: %+v", o)
			}
		}
		_, many := theFamily(t, im)
		if *many != *once || im.ledger.Len() != 1 {
			t.Errorf("after four scans: %+v in %d entries, after one %+v", *many, im.ledger.Len(), *once)
		}
		// A new importer given the same tree reaches the same ledger.
		fresh := newTestImporter(t, nil)
		importAll(t, fresh, tr)
		if !reflect.DeepEqual(fresh.ledger.Totals(), im.ledger.Totals()) {
			t.Errorf("a fresh import differs from the rescanned one")
		}
	})

	t.Run("a resumed session across two files that repeat turns", func(t *testing.T) {
		ref := reference(t, gateDoc(sid, turn1, turn2))
		tr := newTree(t)
		tr.write("projects/-repo/first.jsonl", gateDoc(sid, turn1).String())
		tr.write("projects/-repo/resumed.jsonl", gateDoc(sid, turn1, turn2).String())
		im := newTestImporter(t, nil)
		res := importAll(t, im, tr)
		_, f := theFamily(t, im)
		if *f != ref || f.SessionsVerified != 1 || f.RunsVerified != 4 {
			t.Errorf("two files: %+v, one file holding both turns gives %+v", *f, ref)
		}
		if o := outcomeOf(res, im, sid); o.Files != 2 {
			t.Errorf("%d files behind the session, want 2", o.Files)
		}
		s := im.ledger.Get(sid)
		if s.Partial || s.TaintReason != "" {
			t.Errorf("a clean resume is partial %v (%v) tainted by %q", s.Partial, s.PartialReasons, s.TaintReason)
		}
	})

	t.Run("the same session copied under two project dirs", func(t *testing.T) {
		ref := reference(t, gateDoc(sid, turn1, turn2))
		tr := newTree(t)
		text := gateDoc(sid, turn1, turn2).String()
		tr.write("projects/-repo-a/s.jsonl", text)
		// A backup taken a moment later holds one more line of bookkeeping; a
		// byte for byte copy would be dropped by its digest and show nothing.
		tr.write("projects/-repo-b/s.jsonl", text+`{"type":"last-prompt","sessionId":"`+sid+`"}`+"\n")
		im := newTestImporter(t, nil)
		res := importAll(t, im, tr)
		_, f := theFamily(t, im)
		if *f != ref {
			t.Errorf("two copies: %+v, one gives %+v", *f, ref)
		}
		if outcomeOf(res, im, sid).Files != 2 || res.Duplicates != 0 {
			t.Errorf("files %d, duplicates %d", outcomeOf(res, im, sid).Files, res.Duplicates)
		}
		// A byte for byte copy is one file.
		tr.write("backup/s.jsonl", text)
		tr.write("backup2/s.jsonl", text)
		im2 := newTestImporter(t, nil)
		res2 := importAll(t, im2, tr)
		_, g := theFamily(t, im2)
		if *g != ref || res2.Duplicates != 2 {
			t.Errorf("with identical copies: %+v, %d duplicate files", *g, res2.Duplicates)
		}
	})

	t.Run("a copy that disagrees on a result", func(t *testing.T) {
		ref := reference(t, gateDoc(sid, turn1, turn2))
		tr := newTree(t)
		tr.write("projects/-repo-a/s.jsonl", gateDoc(sid, turn1, turn2).String())
		// The copy says the failing run passed.
		tr.write("projects/-repo-b/s.jsonl", gateDoc(sid, []string{"ok", "ok"}, turn2).String())
		im := newTestImporter(t, nil)
		importAll(t, im, tr)
		_, f := theFamily(t, im)
		s := im.ledger.Get(sid)
		if f.SessionsVerified != 1 {
			t.Errorf("the session is counted %d times", f.SessionsVerified)
		}
		// The call is unsupported and taints from its place: the run before it
		// matches recorded inputs, it and everything after do not.
		if f.RunsVerified != 1 || f.RunsAny >= ref.RunsAny+1 || f.Greens != 1 || f.Reds != 0 {
			t.Errorf("figures %+v, want only the first run to count", *f)
		}
		if !s.Partial || !reflect.DeepEqual(s.PartialReasons, []string{string(replay.CopiesDisagree)}) || s.TaintReason != string(replay.CopiesDisagree) {
			t.Errorf("partial %v %v taint %q", s.Partial, s.PartialReasons, s.TaintReason)
		}
	})

	t.Run("a session with subagent files", func(t *testing.T) {
		tr := newTree(t)
		tr.write("projects/-repo/"+sid+".jsonl", agentTreeFor(sid, "A1", "Explore").String())
		tr.write("projects/-repo/"+sid+"/subagents/agent-A1.jsonl", subChildFor(sid, "A1", "Explore").String())
		tr.write("projects/-repo/"+sid+"/subagents/agent-A1.meta.json", `{"agentType":"Explore"}`)
		// A subagent file whose session has no file of its own is no session.
		orphan := "99999999-0000-4000-8000-000000000009"
		tr.write("projects/-repo/"+orphan+"/subagents/agent-A2.jsonl", subChildFor(orphan, "A2", "Explore").String())
		im := newTestImporter(t, nil)
		res := importAll(t, im, tr)
		if im.ledger.Len() != 1 || res.Orphans != 1 || im.ledger.Has(orphan) {
			t.Fatalf("%d sessions, %d orphans, orphan imported %v", im.ledger.Len(), res.Orphans, im.ledger.Has(orphan))
		}
		_, f := theFamily(t, im)
		// The main agent's one run matches; the subagent's is in the family
		// (same runner, same place) but was fed on an assumption.
		if f.SessionsVerified != 1 || f.RunsVerified != 1 {
			t.Errorf("figures %+v", *f)
		}
		s := im.ledger.Get(sid)
		fam := s.Families[onlyKey(t, s.Families)]
		if fam.RunsAny != 2 || fam.RunsAssumed != 1 || !contains(s.Flags, flagSubagentResponseAssumed) {
			t.Errorf("runs %d any, %d assumed, flags %v", fam.RunsAny, fam.RunsAssumed, s.Flags)
		}
		if len(s.Agents) != 1 || s.Agents[0].Status != replay.LinkOK || s.Agents[0].Type != "Explore" {
			t.Errorf("agents %+v", s.Agents)
		}
		// With the subagent file gone the session is the same session.
		tr2 := newTree(t)
		tr2.write("projects/-repo/"+sid+".jsonl", agentTreeFor(sid, "A1", "Explore").String())
		im2 := newTestImporter(t, nil)
		importAll(t, im2, tr2)
		_, g := theFamily(t, im2)
		if g.SessionsVerified != f.SessionsVerified {
			t.Errorf("the subagent file changed the session count: %d, %d", f.SessionsVerified, g.SessionsVerified)
		}
	})

	t.Run("a session also present in a fake live store", func(t *testing.T) {
		live := t.TempDir()
		other := "22222222-0000-4000-8000-000000000002"
		if err := os.MkdirAll(filepath.Join(live, "runs", sid), 0o700); err != nil {
			t.Fatal(err)
		}
		tr := newTree(t)
		tr.write("projects/-repo/a.jsonl", gateDoc(sid, turn1, turn2).String())
		tr.write("projects/-repo/b.jsonl", gateDoc(other, turn1).String())
		im := newTestImporter(t, func(c *importConfig) { c.LiveRoot = live })
		res := importAll(t, im, tr)
		if o := outcomeOf(res, im, sid); o.Status != statusLive {
			t.Fatalf("the live session: %+v", o)
		}
		_, f := theFamily(t, im)
		if f.SessionsVerified != 1 || im.ledger.Has(sid) || !im.ledger.Has(other) {
			t.Errorf("figures %+v; live session in the ledger %v", *f, im.ledger.Has(sid))
		}
		// DEDUP-2: nothing in the ledger is in the live store.
		for _, id := range []string{sid, other} {
			liveHas, _ := store.RunExistsAt(live, id)
			if liveHas && im.ledger.Has(id) {
				t.Errorf("session %s is counted by both", id)
			}
		}
	})
}

func onlyKey(t *testing.T, m map[string]*familySummary) string {
	t.Helper()
	if len(m) != 1 {
		t.Fatalf("%d families", len(m))
	}
	for k := range m {
		return k
	}
	return ""
}

// TestREPLAY27b_DedupHoldsOverATreeOfEveryRow: one tree with a session of each
// row, scanned in several orders and times, with and without forced rescans,
// leaves the same ledger, and the family's count is the number of session ids
// not recorded live that have a run matching recorded inputs, which the test
// counts itself.
func TestREPLAY27b_DedupHoldsOverATreeOfEveryRow(t *testing.T) {
	ids := map[string]string{
		"single": "aaaaaaaa-0000-4000-8000-000000000001", "resumed": "aaaaaaaa-0000-4000-8000-000000000002",
		"copied": "aaaaaaaa-0000-4000-8000-000000000003", "agents": "aaaaaaaa-0000-4000-8000-000000000004",
		"live": "aaaaaaaa-0000-4000-8000-000000000005", "unverified": "aaaaaaaa-0000-4000-8000-000000000006",
	}
	tr := newTree(t)
	tr.write("p/single.jsonl", gateDoc(ids["single"], []string{"ok"}).String())
	tr.write("p/res1.jsonl", gateDoc(ids["resumed"], []string{"ok"}).String())
	tr.write("p/res2.jsonl", gateDoc(ids["resumed"], []string{"ok"}, []string{"fail"}).String())
	cp := gateDoc(ids["copied"], []string{"fail", "ok"}).String()
	tr.write("p1/copy.jsonl", cp)
	tr.write("p2/copy.jsonl", cp+"\n")
	tr.write("p/agents.jsonl", agentTreeFor(ids["agents"], "A1", "Explore").String())
	tr.write("p/agents/subagents/agent-A1.jsonl", subChildFor(ids["agents"], "A1", "Explore").String())
	tr.write("p/live.jsonl", gateDoc(ids["live"], []string{"ok"}).String())
	// A session whose first line cannot be read: nothing after it is known.
	bad := gateDoc(ids["unverified"], []string{"ok", "ok"})
	tr.write("p/bad.jsonl", "{\"cut\n"+bad.String())

	live := t.TempDir()
	if err := os.MkdirAll(filepath.Join(live, "runs", ids["live"]), 0o700); err != nil {
		t.Fatal(err)
	}
	want := 4 // single, resumed, copied, agents
	run := func(files []string, rescan bool, im *importer) {
		t.Helper()
		im.cfg.Rescan = rescan
		if _, err := im.Import(files); err != nil {
			t.Fatal(err)
		}
	}
	mk := func() *importer {
		return newTestImporter(t, func(c *importConfig) { c.LiveRoot = live })
	}
	ref := mk()
	run(tr.files(), false, ref)
	_, f := theFamily(t, ref)
	if f.SessionsVerified != want {
		t.Fatalf("%d sessions counted, want %d: the single, the resumed, the copied and the one with subagents", f.SessionsVerified, want)
	}
	if ref.ledger.Has(ids["live"]) || !ref.ledger.Has(ids["unverified"]) {
		t.Errorf("live in ledger %v, unverified in ledger %v", ref.ledger.Has(ids["live"]), ref.ledger.Has(ids["unverified"]))
	}
	if u := ref.ledger.Get(ids["unverified"]).Families; len(u) != 1 || onlyFam(u).SessionsVerified != 0 || onlyFam(u).SessionsAny != 1 {
		t.Errorf("a session with no run that matches contributes %+v", u)
	}
	files := tr.files()
	for round := range 4 {
		im := mk()
		// A different order of the same files each round, and a rescan or two.
		fs := append([]string(nil), files...)
		sort.Slice(fs, func(i, j int) bool { return fmt.Sprint(round, fs[i]) > fmt.Sprint(round, fs[j]) == (round%2 == 0) })
		run(fs, false, im)
		run(fs, true, im)
		run(files, round%2 == 0, im)
		if !reflect.DeepEqual(im.ledger.Totals(), ref.ledger.Totals()) {
			t.Fatalf("round %d: totals differ from the reference", round)
		}
		for sid, s := range ref.ledger.sessions {
			if got := im.ledger.sessions[sid]; got == nil || !reflect.DeepEqual(got.Families, s.Families) || !reflect.DeepEqual(got.Files, s.Files) {
				t.Fatalf("round %d: entry %s differs", round, sid[:8])
			}
		}
	}
	// DEDUP-2 over the ledger: no entry is a live run.
	for _, id := range ids {
		liveHas, _ := store.RunExistsAt(live, id)
		if liveHas && ref.ledger.Has(id) {
			t.Errorf("%s is in the ledger and in the live store", id)
		}
	}
}

func onlyFam(m map[string]*familySummary) *familySummary {
	for _, v := range m {
		return v
	}
	return nil
}

// TestREPLAY28_ARescanReplacesAndALiveSessionIsRemoved: a session whose files
// changed is replaced by its new summary (not added to); a session that has
// since been recorded live is taken out of the ledger, so that the seed and the
// live index never both count it; and the ledger holds the session by
// HMAC(key, sessionId) and by its files' digests, and never the id.
func TestREPLAY28_ARescanReplacesAndALiveSessionIsRemoved(t *testing.T) {
	const sid = "33333333-0000-4000-8000-000000000003"
	tr := newTree(t)
	p := tr.write("projects/-repo/s.jsonl", gateDoc(sid, []string{"ok"}).String())
	live := t.TempDir()
	im := newTestImporter(t, func(c *importConfig) { c.LiveRoot = live })
	importAll(t, im, tr)
	_, f1 := theFamily(t, im)
	first := *f1
	s1 := im.ledger.Get(sid)
	if first.RunsVerified != 1 || len(s1.Files) != 1 || s1.SessionHMAC != im.ledger.ID(sid) {
		t.Fatalf("first import: %+v %+v", first, s1)
	}
	if strings.Contains(fmt.Sprintf("%+v", s1), sid) {
		t.Errorf("the ledger entry holds the session id in the clear")
	}
	// The session grows: the same session, another file.
	if err := os.WriteFile(p, []byte(gateDoc(sid, []string{"ok"}, []string{"ok", "fail"}).String()), 0o644); err != nil {
		t.Fatal(err)
	}
	res := importAll(t, im, tr)
	_, f2 := theFamily(t, im)
	if o := outcomeOf(res, im, sid); o.Status != statusImported || !o.Replaced {
		t.Errorf("the changed session: %+v", o)
	}
	if f2.RunsVerified != 3 || f2.SessionsVerified != 1 || im.ledger.Len() != 1 {
		t.Errorf("replaced, not added to: %+v in %d entries (first import %+v)", *f2, im.ledger.Len(), first)
	}
	if reflect.DeepEqual(im.ledger.Get(sid).Files, s1.Files) {
		t.Errorf("the new entry has the old files' digests")
	}
	// The live recorder records the session. A rescan drops it.
	if err := os.MkdirAll(filepath.Join(live, "runs", sid), 0o700); err != nil {
		t.Fatal(err)
	}
	res = importAll(t, im, tr)
	if o := outcomeOf(res, im, sid); o.Status != statusLive || !o.Removed {
		t.Errorf("after it went live: %+v", o)
	}
	if im.ledger.Has(sid) || len(im.ledger.Totals()) != 0 {
		t.Errorf("a live session still counts: %v", im.ledger.Totals())
	}
	// The ledger's key is the identity's: another key, another HMAC.
	other := newTestImporter(t, func(c *importConfig) {
		c.Identity = replayIdentity{KeyHex: strings.Repeat("ab", 32), InstallID: strings.Repeat("cd", 16)}
	})
	if other.ledger.ID(sid) == im.ledger.ID(sid) {
		t.Errorf("two keys give one HMAC")
	}
}

// TestREPLAY29_AnImportTouchesNothingOutsideItsWorlds extends the replay's
// isolation test to the import: the used-looking home of TestREPLAY7_ holds
// the real store, with a live run, and the import is told so. The live check is
// read-only: the home is byte for byte what it was, nothing reached the
// process's streams, a live root that does not exist is not created, and every
// world the import made is gone.
func TestREPLAY29_AnImportTouchesNothingOutsideItsWorlds(t *testing.T) {
	home := plantHome(t)
	before := snapshotTree(t, home)
	liveRoot := filepath.Join(home, ".local", "state", "rashomon")
	scratch := t.TempDir()
	tr := newTree(t)
	tr.write("projects/-repo/live-session.jsonl", gateDoc("live-session", []string{"ok"}).String())
	tr.write("projects/-repo/other.jsonl", gateDoc("44444444-0000-4000-8000-000000000004", []string{"ok", "fail"}).String())
	im := newTestImporter(t, func(c *importConfig) { c.LiveRoot = liveRoot; c.Scratch = scratch })
	finish := captureStd(t)
	res, err := im.Import(tr.files())
	stdout, stderr := finish()
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("the import reached the process's streams: %q %q", stdout, stderr)
	}
	if d := treeDiff(before, snapshotTree(t, home)); len(d) > 0 {
		t.Fatalf("the import changed the home directory:\n  %s", strings.Join(d, "\n  "))
	}
	if im.ledger.Has("live-session") || !im.ledger.Has("44444444-0000-4000-8000-000000000004") || len(res.Sessions) != 2 {
		t.Errorf("sessions %+v", res.Sessions)
	}
	if left, _ := os.ReadDir(scratch); len(left) != 0 {
		t.Errorf("worlds left behind: %d", len(left))
	}
	// A real store that is not there is not made by looking for a run in it.
	absent := filepath.Join(t.TempDir(), "no-store-here")
	im2 := newTestImporter(t, func(c *importConfig) { c.LiveRoot = absent })
	importAll(t, im2, tr)
	if _, err := os.Stat(absent); !os.IsNotExist(err) {
		t.Errorf("looking for a live run created %s: %v", absent, err)
	}
	// And the world cannot be made on the real store.
	im3 := newTestImporter(t, func(c *importConfig) { c.Scratch = filepath.Join(home, ".local", "state", "rashomon", "scratch") })
	if _, err := im3.Import(tr.files()); err == nil {
		t.Errorf("an import was run with its worlds inside the real store")
	}
	if d := treeDiff(before, snapshotTree(t, home)); len(d) > 0 {
		t.Fatalf("the refused import changed the home directory:\n  %s", strings.Join(d, "\n  "))
	}
	_ = settings.ConfigDir
	_ = install.Events
}

// TestREPLAY30_TheSummaryCountsWhatMatchesRecordedInputs holds the per-session
// fields of §6.3 to a session whose runs are known: counts by class, the
// times of the runs that match, the transitions from the ordered runs with
// the unknown-cause exits breaking the chain, last_observed, and the basis
// that keeps reconstruction apart from completeness.
func TestREPLAY30_TheSummaryCountsWhatMatchesRecordedInputs(t *testing.T) {
	const sid = "55555555-0000-4000-8000-000000000005"
	d := gateDoc(sid, []string{"ok", "fail", "ok", "timeout", "fail", "notrun", "ok", "toolerr"})
	tr := newTree(t)
	tr.write("projects/-repo/s.jsonl", d.String())
	im := newTestImporter(t, nil)
	importAll(t, im, tr)
	s := im.ledger.Get(sid)
	f := s.Families[onlyKey(t, s.Families)]
	// The last run names no exit code: it is a run, and not one that matches.
	if f.SessionsVerified != 1 || f.SessionsAny != 1 || f.RunsAny != 8 || f.RunsVerified != 7 ||
		f.Greens != 3 || f.Reds != 2 || f.Timeouts != 1 || f.NotRun != 1 || f.Other != 0 {
		t.Errorf("counts: %+v", *f)
	}
	// ok, fail, ok, (124), fail, (127), ok, (tool error): green→red,
	// red→green, green→red→green once; the timeout and the exit 127 break the
	// chain, so the red after the first and the green after the second start
	// chains of their own and are no transitions from what went before them.
	if f.Transitions != (transitionCounts{GreenToRed: 1, RedToGreen: 1, GreenRedGreen: 1}) || f.PartialTransitions != 0 {
		t.Errorf("transitions: %+v partial %d", f.Transitions, f.PartialTransitions)
	}
	// Result lines are 3, 5, ... 17: each run's execution is its result's time,
	// and the last run that matches is the seventh.
	if !f.FirstSeen.Equal(tAt(3)) || !f.LastSeen.Equal(tAt(15)) {
		t.Errorf("first seen %v, last seen %v, want %v and %v", f.FirstSeen, f.LastSeen, tAt(3), tAt(15))
	}
	lo := f.LastObserved
	if lo.State != "red" || !lo.At.Equal(tAt(17)) || lo.Verified || !lo.SessionComplete || !s.Complete {
		t.Errorf("last observed: %+v complete %v", lo, s.Complete)
	}
	if f.Kind != "test" || f.Project == "" {
		t.Errorf("kind %q project %q", f.Kind, f.Project)
	}

	// The basis: what the summary can and cannot support.
	b := s.Basis
	if b.Reconstruction != basisMatchesRecordedInputs || b.ExecutedInputsVerified || b.CaptureComplete || b.SessionEnd != "unknown" || b.SafetyGrade() {
		t.Errorf("basis: %+v", b)
	}
	for _, flag := range []string{"rewrite_evidence_absent", "duration_ms_unmeasured", "probe_start_synthesized", "session_end_unknown"} {
		if !contains(s.Flags, flag) {
			t.Errorf("flag %s missing from %v", flag, s.Flags)
		}
	}
	if contains(s.Flags, "probe_end_not_replayed") {
		t.Errorf("the old name of session_end_unknown is back")
	}

	// A session that stops mid-turn is not complete: a red last_observed there
	// says nothing about what the agent did next.
	cut := gateDoc(sid, []string{"ok", "fail"})
	cut.lines = cut.lines[:len(cut.lines)-1]
	tr2 := newTree(t)
	tr2.write("projects/-repo/s.jsonl", cut.String())
	im2 := newTestImporter(t, nil)
	importAll(t, im2, tr2)
	s2 := im2.ledger.Get(sid)
	f2 := s2.Families[onlyKey(t, s2.Families)]
	if s2.Complete || f2.LastObserved.SessionComplete || f2.LastObserved.State != "red" {
		t.Errorf("a cut session: complete %v, last observed %+v", s2.Complete, f2.LastObserved)
	}
}

// TestREPLAY30b_GradingUsesTaintAtTheRunsPosition: a run counts as matching
// recorded inputs only if the events behind it are untainted at its position.
// A line that cannot be read in the middle of the session leaves the runs
// before it counted and the runs after it counted as runs only; a transition
// between runs of two files is partial, not a transition.
func TestREPLAY30b_GradingUsesTaintAtTheRunsPosition(t *testing.T) {
	const sid = "66666666-0000-4000-8000-000000000006"
	d := gateDoc(sid, []string{"ok", "fail", "ok"})
	// Line 5, the second run's result, is cut in half.
	d.lines[4] = d.lines[4][:len(d.lines[4])/2]
	tr := newTree(t)
	tr.write("projects/-repo/s.jsonl", d.String())
	im := newTestImporter(t, nil)
	importAll(t, im, tr)
	s := im.ledger.Get(sid)
	f := s.Families[onlyKey(t, s.Families)]
	// Run 1 is whole. Run 2's result is unreadable, so it has a declaration and
	// no execution and is no run at all; run 3 comes after the unreadable line
	// and is a run that does not match.
	if f.RunsVerified != 1 || f.Greens != 1 || f.RunsAny != 2 || f.SessionsVerified != 1 {
		t.Errorf("counts: %+v", *f)
	}
	if s.TaintReason != "unreadable_line" || s.TaintFrom.Pos.Line != 5 || !s.Partial {
		t.Errorf("taint %q from %+v, partial %v %v", s.TaintReason, s.TaintFrom, s.Partial, s.PartialReasons)
	}
	if f.Transitions != (transitionCounts{}) {
		t.Errorf("transitions across a gap: %+v", f.Transitions)
	}

	// Two files: the green is in the first, the red in the second.
	a := gateDocIDs(sid, "a", 0, []string{"ok"})
	b := gateDocIDs(sid, "b", time.Hour, []string{"fail"})
	tr2 := newTree(t)
	tr2.write("projects/-repo/a.jsonl", a.String())
	tr2.write("projects/-repo/b.jsonl", b.String())
	im2 := newTestImporter(t, nil)
	importAll(t, im2, tr2)
	s2 := im2.ledger.Get(sid)
	f2 := s2.Families[onlyKey(t, s2.Families)]
	t.Logf("two files: taint %q from %+v, partial %v, skipped %v, flags %v", s2.TaintReason, s2.TaintFrom, s2.PartialReasons, s2.Skipped, s2.Flags)
	if f2.RunsVerified != 2 || f2.Transitions != (transitionCounts{}) || f2.PartialTransitions != 1 {
		t.Errorf("a green in one file and a red in the next: %+v", *f2)
	}
	// The same two runs in one file are a transition.
	one := gateDocIDs(sid, "a", 0, []string{"ok", "fail"})
	tr3 := newTree(t)
	tr3.write("projects/-repo/one.jsonl", one.String())
	im3 := newTestImporter(t, nil)
	importAll(t, im3, tr3)
	s3 := im3.ledger.Get(sid)
	f3 := s3.Families[onlyKey(t, s3.Families)]
	if f3.Transitions.GreenToRed != 1 || f3.PartialTransitions != 0 {
		t.Errorf("one file: %+v partial %d", f3.Transitions, f3.PartialTransitions)
	}
}
