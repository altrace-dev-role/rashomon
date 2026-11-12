package main

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

func seenConflict(kind, rule, path string, resolved int64, seqs ...int64) verify.ConflictSeen {
	return verify.ConflictSeen{Row: verify.ConflictRow{Kind: kind, Unit: "g:r#unit:go:" + path, UnitPath: path, RuleID: rule,
		Subject: "sym:go:" + path}, Seqs: seqs, ResolvedSeq: resolved}
}

// runStopWith is the Stop of a generated session (a final message that claims
// a pass over failed calls, so recap.Line's sentences speak) with conflicts
// as the seam returns them: the exit code and the stdout and stderr.
func runStopWith(t *testing.T, conflicts []verify.ConflictSeen) (int, string, string) {
	t.Helper()
	root, cfg := t.TempDir(), t.TempDir()
	benchSetenv(t, root, cfg)
	installID := benchInstall(t, root, cfg)
	if _, err := payload.WriteSession(root, 10, payload.Prompts); err != nil {
		t.Fatal(err)
	}
	old := conflictsSeen
	conflictsSeen = func(*store.Run, int64) []verify.ConflictSeen { return conflicts }
	t.Cleanup(func() { conflictsSeen = old })
	args, in := benchRecapInput(t, installID, false)
	var stdout, stderr bytes.Buffer
	code := cmdRecap(args, bytes.NewReader(in), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func systemMessage(t *testing.T, out string) string {
	t.Helper()
	var env struct {
		SystemMessage string `json:"systemMessage"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout %q: %v", out, err)
	}
	return env.SystemMessage
}

// TestStop_ConflictNeverBlocks (PLAN, R2): through the Stop entry, a session
// with an unresolved conflict exits 0 and prints an envelope with the keys of
// the conflict-free Stop's, and the conflict is in its text.
func TestStop_ConflictNeverBlocks(t *testing.T) {
	keys := func(out string) []string {
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(out), &m); err != nil {
			t.Fatalf("stdout %q: %v", out, err)
		}
		var k []string
		for key := range m {
			k = append(k, key)
		}
		slices.Sort(k)
		return k
	}
	codeA, outA, errA := runStopWith(t, nil)
	codeB, outB, errB := runStopWith(t, []verify.ConflictSeen{seenConflict("boundary_cross", "no-ui-db", "internal/ui", 0, 3)})
	if codeA != 0 || codeB != 0 || errA != "" || errB != "" {
		t.Fatalf("exit %d and %d, stderr %q and %q, want 0, 0 and none", codeA, codeB, errA, errB)
	}
	if a, b := keys(outA), keys(outB); !slices.Equal(a, b) {
		t.Errorf("envelope keys %v with a conflict, %v without", b, a)
	}
	if m := systemMessage(t, outB); !strings.Contains(m, "spec rule no-ui-db:") || !strings.Contains(m, "unit 'internal/ui' adds") {
		t.Errorf("conflicted Stop text %q does not name the conflict", m)
	}
}

// platformView is a view every composer speaks for: what sits before the
// conflict lines (a rule, an unbound spec, an owed package), the conflicts of
// in through EvaluateStop, and a claim of done with no gate to back it.
func platformView(in []verify.ConflictSeen) verify.StopView {
	v := verify.EvaluateStop(&store.Run{}, nil, nil, verify.Point{Kind: verify.PointStop, SessionID: "s1"}, time.Unix(1, 0), verify.StopInput{Conflicts: in})
	v.Summary.Row, v.Turn = verify.RowNoGate, verify.TurnCounts{Writes: 1, PathsWritten: 1, LastWriteSeq: 5}
	v.Scan = verify.ClaimScan{Claims: []verify.ClaimVerdict{{Class: verify.ClaimDone, Strict: true, Verdict: verify.Verdict(verify.Absent)}}}
	v.Violations = []verify.RuleVerdict{{Rule: verify.SlotID("spec:no-rm-glob"), Kind: "forbid_command", Verdict: verify.Violation, Seqs: []int64{3}}}
	v.Unbound = []string{"rashomon.yaml"}
	v.Obligations = append(v.Obligations, verify.Obligation{Kind: verify.PlatformImpact, State: verify.ObligationOpen})
	v.Platform.Families = []verify.PlatformFamily{{Package: "p", Dir: "internal/verify", ModuleDir: "."}}
	return v
}

// composerTexts is each composer's text for in: the three that take a view,
// and the Stop entry's, whose LineWith argument is composed in recapTurn.
func composerTexts(t *testing.T, in []verify.ConflictSeen) map[string]string {
	t.Helper()
	v := platformView(in)
	im := recap.Impact{Findings: []recap.FindingRef{{State: recap.FindingResolved, ResolvedBy: 41}}}
	done, ok := recap.DoneFlag(v, "sess-1", false)
	if !ok {
		t.Fatal("the done flag did not fire on the fixture")
	}
	_, entry, _ := runStopWith(t, in)
	return map[string]string{
		"ValueLine": recap.ValueLine(v, im, "sess-1", false),
		"DoneFlag":  done,
		"RuleLine":  recap.RuleLine(v, "sess-1", false),
		"recapTurn": systemMessage(t, entry),
	}
}

// TestStop_ConflictLinesInEveryComposer (B3): each composer prints the
// conflict line, after the platform line where it prints one, before the
// clause and the pointer.
func TestStop_ConflictLinesInEveryComposer(t *testing.T) {
	in := []verify.ConflictSeen{seenConflict("boundary_cross", "no-ui-db", "internal/ui", 0, 3)}
	const conflict, platform, clause, pointer = "unit 'internal/ui' adds ", "platform: go test", "AutoSteer:", "→ rashomon report"
	texts := composerTexts(t, in)
	for name, got := range texts {
		at := strings.Index(got, conflict)
		if at < 0 {
			t.Errorf("%s: %q does not hold the conflict line", name, got)
			continue
		}
		order := []int{strings.Index(got, platform), at, strings.Index(got, clause), strings.Index(got, pointer)}
		order = slices.DeleteFunc(order, func(i int) bool { return i < 0 })
		if !slices.IsSorted(order) {
			t.Errorf("%s: the lines are out of order: %q", name, got)
		}
	}
	if got := texts["ValueLine"]; !strings.Contains(got, platform) || !strings.Contains(got, clause) {
		t.Errorf("the ValueLine fixture lacks the platform line or the clause: %q", got)
	}
}

// TestStop_WithheldConflictKindSaysNothing (director 16:23, the M2 not-live
// pin): a duplicate_abstraction member, alone and beside live ones, owes no
// Obligation, no StopView.Conflicts entry and no word in any composer's text,
// and the counting line counts only the live kind. An arity member likewise.
func TestStop_WithheldConflictKindSaysNothing(t *testing.T) {
	live := func(n string) verify.ConflictSeen { return seenConflict("boundary_cross", "r", "internal/"+n, 0, 3) }
	for _, kind := range []string{"duplicate_abstraction", "arity"} {
		withheld := seenConflict(kind, "r", "internal/dup", 0, 4)
		alone := platformView([]verify.ConflictSeen{withheld})
		if len(alone.Conflicts) != 0 || slices.ContainsFunc(alone.Obligations, func(o verify.Obligation) bool { return o.Unit != "" }) {
			t.Errorf("%s alone: conflicts %+v obligations %+v, want none", kind, alone.Conflicts, alone.Obligations)
		}
		for name, got := range composerTexts(t, []verify.ConflictSeen{withheld}) {
			if strings.Contains(got, "duplicate") || strings.Contains(got, "abstraction") || strings.Contains(got, "conflict") {
				t.Errorf("%s alone, %s: %q speaks of it", kind, name, got)
			}
		}
		beside := []verify.ConflictSeen{live("a"), withheld, live("b"), live("c")}
		if v := platformView(beside); len(v.Conflicts) != 3 {
			t.Errorf("%s beside three live: view %+v, want those three", kind, v.Conflicts)
		}
		for name, got := range composerTexts(t, beside) {
			if strings.Contains(got, "duplicate") || strings.Contains(got, "abstraction") {
				t.Errorf("%s beside live, %s: %q speaks of it", kind, name, got)
			}
			if name != "RuleLine" && !strings.Contains(got, "and 1 more unresolved conflict") {
				t.Errorf("%s beside three live, %s: %q, want the counting line to count the live kind only (1 more)", kind, name, got)
			}
		}
	}
}

// TestStop_ConflictKindsArePartitioned: every engine conflict kind is in
// exactly one of verify.ConflictRowKinds, verify.WithheldConflictKinds, or
// arity (the session graph's); each holds only engine kinds; recap's phrase
// table has the row kinds' keys. A seventh engine kind fails here until
// someone places it.
func TestStop_ConflictKindsArePartitioned(t *testing.T) {
	engineKinds := map[string]bool{}
	for _, k := range engine.ConflictKinds {
		engineKinds[string(k)] = true
		n := 0
		for _, set := range [][]string{verify.ConflictRowKinds, verify.WithheldConflictKinds, {string(engine.ConflictArity)}} {
			if slices.Contains(set, string(k)) {
				n++
			}
		}
		if n != 1 {
			t.Errorf("engine kind %s is in %d of the row, withheld and arity sets, want exactly one", k, n)
		}
	}
	for _, set := range [][]string{verify.ConflictRowKinds, verify.WithheldConflictKinds} {
		for _, k := range set {
			if !engineKinds[k] {
				t.Errorf("%s is not an engine conflict kind", k)
			}
		}
	}
	if got := recap.ConflictPhraseKinds(); !slices.Equal(got, verify.ConflictRowKinds) {
		t.Errorf("phrase table keys %v, want %v", got, verify.ConflictRowKinds)
	}
}

// callsIn is the positions of the calls in body to a function or method named
// one of names, in source order.
func callsIn(body *ast.BlockStmt, names ...string) []token.Pos {
	var out []token.Pos
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee := ""
		switch c := call.Fun.(type) {
		case *ast.Ident:
			callee = c.Name
		case *ast.SelectorExpr:
			callee = c.Sel.Name
		}
		if slices.Contains(names, callee) {
			out = append(out, call.Pos())
		}
		return true
	})
	return out
}

// uncovered is each position of platform that no position of conflict follows.
func uncovered(platform, conflict []token.Pos) []token.Pos {
	var out []token.Pos
	for _, at := range platform {
		if !slices.ContainsFunc(conflict, func(after token.Pos) bool { return after > at }) {
			out = append(out, at)
		}
	}
	return out
}

// TestStop_EveryPlatformComposerCarriesConflicts (B3): every function of
// internal/recap and cmd/rashomon, non-test, whose body calls platformLines or
// PlatformLines also calls conflictLines or ConflictLines after it in source
// order, so a fifth composer that forgets them fails. The wrappers that
// define the names are not composers.
func TestStop_EveryPlatformComposerCarriesConflicts(t *testing.T) {
	var found []string
	for _, dir := range []string{"../../internal/recap", "."} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("glob %s: %v %d", dir, err, len(files))
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, f, src, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range file.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				fname := fn.Name.String()
				if slices.Contains([]string{"platformLines", "PlatformLines", "conflictLines", "ConflictLines"}, fname) {
					continue
				}
				platform := callsIn(fn.Body, "platformLines", "PlatformLines")
				conflict := callsIn(fn.Body, "conflictLines", "ConflictLines")
				if len(platform) == 0 {
					continue
				}
				found = append(found, filepath.Base(f)+":"+fname)
				for _, at := range uncovered(platform, conflict) {
					t.Errorf("%s:%s calls the platform lines at %s and no conflict lines after", filepath.Base(f), fname, fset.Position(at))
				}
			}
		}
	}
	slices.Sort(found)
	t.Logf("composers found: %v", found)
	if len(found) < 3 {
		t.Errorf("found %v, want ValueLine, DoneFlag and recapTurn at least", found)
	}
}
