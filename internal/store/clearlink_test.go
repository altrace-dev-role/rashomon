package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// Session ids in the shape Claude Code gives them: plain path segments.
const (
	clrO1 = "0a1b2c3d-0000-4000-8000-00000000000a"
	clrN1 = "0a1b2c3d-0000-4000-8000-00000000000b"
	clrO2 = "0a1b2c3d-0000-4000-8000-00000000000c"
	clrN2 = "0a1b2c3d-0000-4000-8000-00000000000d"
)

func meet(t *testing.T, dir string, side ClearSide, self string, now time.Time) (ClearLink, bool) {
	t.Helper()
	l, ok, err := meetClear(dir, side, self, now)
	if err != nil {
		t.Fatalf("meetClear(%d, %s): %v", side, self, err)
	}
	return l, ok
}

// backdate leaves a file as a hook would have age before now.
func backdate(t *testing.T, path string, now time.Time, age time.Duration) {
	t.Helper()
	at := now.Add(-age)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

// touch plants an arm the way the other side's hook leaves it between
// creating it and listing the directory.
func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func linkContent(t *testing.T, dir, newID string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "link-"+newID))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b), true
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

// TestMeetClear_EndThenStartLinksOnce (W-M4.1): the old session's
// SessionEnd(clear) arms alone and links nothing; the new session's
// SessionStart(clear) a second later finds that one arm and writes the link,
// which names the old session. Every name left in the directory is a kind
// and a session id: none holds the cwd (ruling 2026-11-12 09:46 (b)).
func TestMeetClear_EndThenStartLinksOnce(t *testing.T) {
	dir, now := t.TempDir(), time.Now()
	if l, ok := meet(t, dir, ClearEnd, clrO1, now); ok || l.Wrote {
		t.Fatalf("end alone = %+v, %v; want no link", l, ok)
	}
	l, ok := meet(t, dir, ClearStart, clrN1, now.Add(time.Second))
	if want := (ClearLink{Old: clrO1, New: clrN1, Wrote: true}); !ok || l != want {
		t.Fatalf("start = %+v, %v; want %+v", l, ok, want)
	}
	if got, ok := linkContent(t, dir, clrN1); !ok || got != clrO1 {
		t.Errorf("link-%s = %q, %v; want %q", clrN1, got, ok, clrO1)
	}
	shape := regexp.MustCompile(`^(end|start|link)-[0-9a-f-]{36}$`)
	for _, n := range names(t, dir) {
		if !shape.MatchString(n) {
			t.Errorf("entry %q is not <kind>-<session id>", n)
		}
	}
}

// TestMeetClear_StartThenEndLinksOnce (W-M4.1): the hooks docs do not order
// SessionEnd and SessionStart on /clear, so the reverse order links the same
// pair, written by the end side.
func TestMeetClear_StartThenEndLinksOnce(t *testing.T) {
	dir, now := t.TempDir(), time.Now()
	if l, ok := meet(t, dir, ClearStart, clrN1, now); ok || l.Wrote {
		t.Fatalf("start alone = %+v, %v; want no link", l, ok)
	}
	l, ok := meet(t, dir, ClearEnd, clrO1, now.Add(time.Second))
	if want := (ClearLink{Old: clrO1, New: clrN1, Wrote: true}); !ok || l != want {
		t.Fatalf("end = %+v, %v; want %+v", l, ok, want)
	}
	if got, ok := linkContent(t, dir, clrN1); !ok || got != clrO1 {
		t.Errorf("link-%s = %q, %v; want %q", clrN1, got, ok, clrO1)
	}
}

// TestMeetClear_WindowBracketsAtHalfAndOneAndAHalf: an arm half a window old
// pairs and one a window and a half old does not; neither case sits where
// rounding decides it.
func TestMeetClear_WindowBracketsAtHalfAndOneAndAHalf(t *testing.T) {
	for _, c := range []struct {
		age  time.Duration
		link bool
	}{{ClearWindow / 2, true}, {ClearWindow * 3 / 2, false}} {
		dir, now := t.TempDir(), time.Now()
		meet(t, dir, ClearEnd, clrO1, now)
		backdate(t, filepath.Join(dir, "end-"+clrO1), now, c.age)
		if _, ok := meet(t, dir, ClearStart, clrN1, now); ok != c.link {
			t.Errorf("arm aged %v: linked %v, want %v", c.age, ok, c.link)
		}
	}
}

// TestClearWindow_CoversTheClearHooksTimeout ties the window to its basis:
// plugin/hooks/hooks.json caps rashomon's SessionStart and SessionEnd hooks,
// and when the CLI runs one event's hooks to the end before the other's, the
// gap between the two arms is at most one such hook. The window is two of
// them, and stays separate from ClearKeep's sweep.
func TestClearWindow_CoversTheClearHooksTimeout(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "plugin", "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string  `json:"command"`
				Timeout float64 `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, event := range []string{"SessionStart", "SessionEnd"} {
		for _, group := range doc.Hooks[event] {
			for _, h := range group.Hooks {
				if !strings.Contains(h.Command, "/bin/rashomon") {
					continue
				}
				seen++
				limit := time.Duration(h.Timeout * float64(time.Second))
				if h.Timeout <= 0 || ClearWindow < 2*limit {
					t.Errorf("%s timeout %vs: ClearWindow %v is under two of it", event, h.Timeout, ClearWindow)
				}
			}
		}
	}
	if seen != 2 {
		t.Errorf("found %d rashomon SessionStart/SessionEnd hooks, want 2", seen)
	}
	if ClearKeep <= ClearWindow {
		t.Errorf("ClearKeep %v must exceed ClearWindow %v", ClearKeep, ClearWindow)
	}
}

// TestMeetClear_OverlappingClearsInOneCwdLinkNothing (ruling 2026-11-12
// 09:46 (a)(1)): two sessions in one cwd that clear inside the window leave
// no link in any order a CLI runs them; a wrong link is worse than none.
func TestMeetClear_OverlappingClearsInOneCwdLinkNothing(t *testing.T) {
	type call struct {
		side ClearSide
		id   string
	}
	end1, end2 := call{ClearEnd, clrO1}, call{ClearEnd, clrO2}
	start1, start2 := call{ClearStart, clrN1}, call{ClearStart, clrN2}
	for name, order := range map[string][]call{
		"ends first":         {end1, end2, start1, start2},
		"ends first, swap":   {end2, end1, start2, start1},
		"starts first":       {start1, start2, end1, end2},
		"starts first, swap": {start2, start1, end2, end1},
	} {
		dir, now := t.TempDir(), time.Now()
		for i, c := range order {
			if l, ok := meet(t, dir, c.side, c.id, now.Add(time.Duration(i)*100*time.Millisecond)); ok || l.Wrote {
				t.Errorf("%s: call %d (%d, %s) = %+v, %v; want no link", name, i, c.side, c.id, l, ok)
			}
		}
		for _, n := range names(t, dir) {
			if strings.HasPrefix(n, "link-") {
				t.Errorf("%s: %s written", name, n)
			}
		}
	}
}

// TestMeetClear_SameKindArmBlocksAMispair: terminal 2 has ended O2 and its
// start N2 has armed but not yet listed when terminal 1 ends O1. O1 sees one
// start arm, N2's, and a rule that counted only the other kind would link O1
// to N2. The second end arm, O2's, is the evidence that the pairing is
// ambiguous, so nothing is linked.
func TestMeetClear_SameKindArmBlocksAMispair(t *testing.T) {
	dir, now := t.TempDir(), time.Now()
	meet(t, dir, ClearEnd, clrO2, now)
	touch(t, filepath.Join(dir, "start-"+clrN2))
	if l, ok := meet(t, dir, ClearEnd, clrO1, now); ok || l.Wrote {
		t.Fatalf("O1 = %+v, %v; want no link", l, ok)
	}
	if got, ok := linkContent(t, dir, clrN2); ok {
		t.Fatalf("link-%s = %q written", clrN2, got)
	}
}

// TestMeetClear_MatchedPairDoesNotBlockTheNext: a linked pair's arms are
// matched and leave the count, so a second terminal clearing inside the same
// window after the first has linked still pairs.
func TestMeetClear_MatchedPairDoesNotBlockTheNext(t *testing.T) {
	dir, now := t.TempDir(), time.Now()
	meet(t, dir, ClearEnd, clrO1, now)
	if _, ok := meet(t, dir, ClearStart, clrN1, now); !ok {
		t.Fatal("first pair did not link")
	}
	meet(t, dir, ClearEnd, clrO2, now.Add(time.Second))
	l, ok := meet(t, dir, ClearStart, clrN2, now.Add(time.Second))
	if want := (ClearLink{Old: clrO2, New: clrN2, Wrote: true}); !ok || l != want {
		t.Fatalf("second pair = %+v, %v; want %+v", l, ok, want)
	}
}

// TestMeetClear_ExistingLinkIsNeverOverwritten: a new session links at most
// once. A link that names this pair is confirmed without a second write; one
// that names another old session stands, and this call links nothing.
func TestMeetClear_ExistingLinkIsNeverOverwritten(t *testing.T) {
	for _, c := range []struct {
		content string
		ok      bool
	}{{clrO1, true}, {clrO2, false}} {
		dir, now := t.TempDir(), time.Now()
		if err := os.WriteFile(filepath.Join(dir, "link-"+clrN1), []byte(c.content), 0o600); err != nil {
			t.Fatal(err)
		}
		touch(t, filepath.Join(dir, "start-"+clrN1))
		l, ok := meet(t, dir, ClearEnd, clrO1, now)
		if ok != c.ok || l.Wrote {
			t.Errorf("link holding %s: %+v, %v; want ok %v and no write", c.content, l, ok, c.ok)
		}
		if got, _ := linkContent(t, dir, clrN1); got != c.content {
			t.Errorf("link rewritten to %q, want %q", got, c.content)
		}
	}
}

// TestMeetClear_RacingSidesWriteOnce: the two sides of one /clear run at
// once. Each arms before it lists, so at least one sees the other; exactly
// one writes the link, and no call reports another pair.
func TestMeetClear_RacingSidesWriteOnce(t *testing.T) {
	for i := 0; i < 100; i++ {
		dir, now := t.TempDir(), time.Now()
		var wg sync.WaitGroup
		res := make([]ClearLink, 2)
		oks := make([]bool, 2)
		errs := make([]error, 2)
		for j, c := range []struct {
			side ClearSide
			id   string
		}{{ClearEnd, clrO1}, {ClearStart, clrN1}} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				res[j], oks[j], errs[j] = meetClear(dir, c.side, c.id, now)
			}()
		}
		wg.Wait()
		writes := 0
		for j := range res {
			if errs[j] != nil {
				t.Fatalf("run %d side %d: %v", i, j, errs[j])
			}
			if oks[j] && (res[j].Old != clrO1 || res[j].New != clrN1) {
				t.Fatalf("run %d side %d reported %+v", i, j, res[j])
			}
			if res[j].Wrote {
				writes++
			}
		}
		if writes != 1 {
			t.Fatalf("run %d: %d writers, want 1", i, writes)
		}
		if got, ok := linkContent(t, dir, clrN1); !ok || got != clrO1 {
			t.Fatalf("run %d: link = %q, %v", i, got, ok)
		}
	}
}

// TestMeetClear_VisitSweepsOnlyOldEntriesOfItsOwn: any visit removes arms,
// links and link temporaries older than ClearKeep, and nothing younger or
// not of its own naming.
func TestMeetClear_VisitSweepsOnlyOldEntriesOfItsOwn(t *testing.T) {
	dir, now := t.TempDir(), time.Now()
	old := []string{"end-" + clrO2, "start-" + clrN2, "link-" + clrN2, ".link-" + clrN2 + ".tmp-123"}
	kept := []string{"end-" + clrO1, "notes.txt"}
	for _, n := range append(append([]string{}, old...), kept...) {
		touch(t, filepath.Join(dir, n))
		backdate(t, filepath.Join(dir, n), now, ClearKeep+time.Minute)
	}
	backdate(t, filepath.Join(dir, "end-"+clrO1), now, ClearKeep/2)
	meet(t, dir, ClearStart, clrN1, now)
	have := map[string]bool{}
	for _, n := range names(t, dir) {
		have[n] = true
	}
	for _, n := range old {
		if have[n] {
			t.Errorf("%s older than ClearKeep survived the visit", n)
		}
	}
	for _, n := range kept {
		if !have[n] {
			t.Errorf("%s removed", n)
		}
	}
}

// TestMeetClear_UnsafeIDLinksNothing: an id that is not a plain path segment
// arms nothing and links nothing, rather than a digest no other side can
// name back.
func TestMeetClear_UnsafeIDLinksNothing(t *testing.T) {
	dir, now := t.TempDir(), time.Now()
	touch(t, filepath.Join(dir, "start-"+clrN1))
	for _, id := range []string{"", "..", "../" + clrO1, "a b"} {
		if l, ok := meet(t, dir, ClearEnd, id, now); ok || l.Wrote {
			t.Errorf("id %q = %+v, %v; want no link", id, l, ok)
		}
	}
	if got := names(t, dir); len(got) != 1 {
		t.Errorf("entries = %v; want only the planted start arm", got)
	}
}
