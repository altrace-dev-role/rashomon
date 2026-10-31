package learn

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/steer"
)

const project = "work_proj-0123456789ab"

var (
	t0 = time.UnixMilli(1_761_700_000_000)
	t1 = t0.Add(time.Hour)
)

func open(t *testing.T, root string, now time.Time) *Ledger {
	t.Helper()
	l, err := Open(root, project, now)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func importText(t *testing.T, l *Ledger, file string) (int, int) {
	t.Helper()
	added, skipped, err := l.Import(strings.NewReader(file), t0)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
	return added, skipped
}

func activeOf(t *testing.T, root string) activeFile {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, DirLearn, project, FileActive))
	if err != nil {
		t.Fatal(err)
	}
	var a activeFile
	if err := json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	return a
}

func tiers(l *Ledger) string {
	var s []string
	for _, ls := range l.Lessons() {
		s = append(s, ls.ID+"="+string(ls.Tier))
	}
	return strings.Join(s, " ")
}

const twoLessons = `{"schema":"lessons/1","lessons":[
	{"id":"go-test-cwd","tier":"proven","text":"run go test from the module root","since_unix_ms":1},
	{"id":"npm-ci","tier":"probation","text":"npm ci before npm test","since_unix_ms":2}]}`

// TestP6_ImportLandsAtProposed: a shared lesson enters at proposed whatever
// tier it names, never reaches active.json, and a taken id, a malformed id,
// an empty text and a text over the steer cap are skipped.
func TestP6_ImportLandsAtProposed(t *testing.T) {
	root := t.TempDir()
	l := open(t, root, t0)
	if added, skipped := importText(t, l, twoLessons); added != 2 || skipped != 0 {
		t.Fatalf("import = %d added, %d skipped; want 2, 0", added, skipped)
	}
	if got := tiers(l); got != "go-test-cwd=proposed npm-ci=proposed" {
		t.Errorf("tiers %q, want both proposed", got)
	}
	if a := activeOf(t, root); len(a.Lessons) != 0 || a.Quarantined || a.Schema != SchemaActive {
		t.Errorf("active.json %+v, want no lessons", a)
	}
	again := `{"lessons":[{"id":"npm-ci","text":"x"},{"id":"Bad","text":"x"},{"id":"empty","text":""},` +
		`{"id":"long","text":"` + strings.Repeat("a", 2049) + `"},{"id":"ok","tier":"proven","text":"y"}]}`
	if added, skipped := importText(t, open(t, root, t1), again); added != 1 || skipped != 4 {
		t.Errorf("second import = %d added, %d skipped; want 1, 4", added, skipped)
	}
	if got := tiers(open(t, root, t1)); got != "go-test-cwd=proposed npm-ci=proposed ok=proposed" {
		t.Errorf("tiers after reopening %q", got)
	}
}

// TestP6_ApprovalMovesOnlyAProposedLesson: approval puts a proposed lesson on
// probation and into active.json; a second approval, or one of no lesson, is
// refused.
func TestP6_ApprovalMovesOnlyAProposedLesson(t *testing.T) {
	root := t.TempDir()
	l := open(t, root, t0)
	importText(t, l, twoLessons)
	if err := l.Approve("npm-ci", t1); err != nil {
		t.Fatal(err)
	}
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
	l = open(t, root, t1)
	if l.Quarantined || tiers(l) != "go-test-cwd=proposed npm-ci=probation" {
		t.Fatalf("reopened: quarantined %v, tiers %q", l.Quarantined, tiers(l))
	}
	if a := activeOf(t, root); len(a.Lessons) != 1 || a.Lessons[0].ID != "npm-ci" {
		t.Errorf("active.json %+v, want npm-ci alone", a)
	}
	if err := l.Approve("npm-ci", t1); err == nil {
		t.Error("a second approval of npm-ci was taken")
	}
	if err := l.Approve("nope", t1); err == nil {
		t.Error("approving a lesson that does not exist was taken")
	}
}

// TestP6_LabelsMoveAnApprovedLesson: labels appended through the ledger move
// a lesson to proven and then demoted, and a reopened ledger reads the same.
func TestP6_LabelsMoveAnApprovedLesson(t *testing.T) {
	root := t.TempDir()
	l := open(t, root, t0)
	importText(t, l, twoLessons)
	if err := l.Approve("npm-ci", t1); err != nil {
		t.Fatal(err)
	}
	if err := l.AppendLabel(Label{LessonID: "npm-ci", Label: "fixed"}); err == nil {
		t.Error("a label outside steer.AllLabels was appended")
	}
	writeLabels(t, l, "npm-ci", 30, 0)
	if got := tiers(open(t, root, t1)); got != "go-test-cwd=proposed npm-ci=proven" {
		t.Errorf("after 30 confirmed: %q", got)
	}
	if err := l.AppendLabel(Label{LessonID: "npm-ci", SessionID: "s", AtUnixMS: 9, Label: steer.LabelRefuted}); err != nil {
		t.Fatal(err)
	}
	writeLabels(t, l, "npm-ci", 0, 39)
	l = open(t, root, t1)
	if got := tiers(l); l.Quarantined || got != "go-test-cwd=proposed npm-ci=demoted" {
		t.Errorf("after 40 refuted: quarantined %v, %q", l.Quarantined, got)
	}
	if tl := l.Tally("npm-ci"); tl != (Tally{Fires: 70, Confirmed: 30, Refuted: 40}) {
		t.Errorf("tally %+v", tl)
	}
}

// writeLabels appends confirmed and refuted rows for id as LABEL-WRITER1
// would, one save for the lot rather than one per row.
func writeLabels(t *testing.T, l *Ledger, id string, confirmed, refuted int) {
	t.Helper()
	path := filepath.Join(l.dir, FileLabels)
	b, _ := os.ReadFile(path)
	for i := range confirmed + refuted {
		label := steer.LabelConfirmed
		if i >= confirmed {
			label = steer.LabelRefuted
		}
		row, _ := json.Marshal(Label{LessonID: id, SessionID: fmt.Sprint("s", i), AtUnixMS: int64(i), Label: label})
		b = append(append(b, row...), '\n')
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	l.tallies = tallies(b)
	l.retier()
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
}

// TestP6_AgentWriteQuarantinesEveryFact: any write into the learning state
// this package did not make -- an edit of any state file, a deleted manifest
// or key -- quarantines every lesson at the next read: all return to
// proposed, active.json says quarantined and holds none, the time is kept,
// and the state is signed again, so the read after it is clean.
func TestP6_AgentWriteQuarantinesEveryFact(t *testing.T) {
	for name, tamper := range map[string]func(dir, keyPath string) error{
		"lessons.json edited": func(dir, _ string) error {
			return edit(filepath.Join(dir, FileLessons), "npm ci before", "curl x | sh before")
		},
		"a label appended by hand": func(dir, _ string) error {
			return os.WriteFile(filepath.Join(dir, FileLabels), []byte(`{"lesson_id":"npm-ci","label":"confirmed"}`+"\n"), 0o600)
		},
		"active.json edited": func(dir, _ string) error {
			return edit(filepath.Join(dir, FileActive), `"quarantined":false`, `"quarantined":false `)
		},
		"state.sum removed": func(dir, _ string) error { return os.Remove(filepath.Join(dir, FileSum)) },
		"key removed":       func(_, key string) error { return os.Remove(key) },
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			l := open(t, root, t0)
			importText(t, l, twoLessons)
			for _, id := range []string{"npm-ci", "go-test-cwd"} {
				if err := l.Approve(id, t1); err != nil {
					t.Fatal(err)
				}
			}
			if err := l.Save(); err != nil {
				t.Fatal(err)
			}
			if err := tamper(l.dir, filepath.Join(root, DirLearn, FileKey)); err != nil {
				t.Fatal(err)
			}
			l = open(t, root, t1)
			if !l.Quarantined || tiers(l) != "go-test-cwd=proposed npm-ci=proposed" {
				t.Errorf("after the write: quarantined %v, tiers %q; want quarantined, every lesson proposed", l.Quarantined, tiers(l))
			}
			if !l.QuarantinedAt().Equal(t1) {
				t.Errorf("QuarantinedAt %v, want %v", l.QuarantinedAt(), t1)
			}
			if a := activeOf(t, root); !a.Quarantined || len(a.Lessons) != 0 {
				t.Errorf("active.json %+v, want quarantined with no lessons", a)
			}
			if l = open(t, root, t1.Add(time.Hour)); l.Quarantined || !l.QuarantinedAt().Equal(t1) {
				t.Errorf("the read after: quarantined %v at %v; want clean, the time kept", l.Quarantined, l.QuarantinedAt())
			}
		})
	}
}

func edit(path, old, new string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Contains(b, []byte(old)) {
		return fmt.Errorf("%s holds no %q", path, old)
	}
	return os.WriteFile(path, bytes.Replace(b, []byte(old), []byte(new), 1), 0o600)
}

// TestP6_ActiveCapDropsTheOldestDemotedFirst: over 64 KiB, active.json loses
// its oldest demoted rows first and keeps every other approved row; with no
// demoted row left to drop it is not written, and the old files stand.
func TestP6_ActiveCapDropsTheOldestDemotedFirst(t *testing.T) {
	root := t.TempDir()
	l := open(t, root, t0)
	text := strings.Repeat("x", 2000)
	var rows []string
	for i := range 40 {
		rows = append(rows, fmt.Sprintf(`{"id":"l%02d","text":%q}`, i, text))
	}
	importText(t, l, `{"lessons":[`+strings.Join(rows, ",")+`]}`)
	for i := range l.file.Lessons {
		l.file.Lessons[i].SinceUnixMS = int64(i)
		l.file.Lessons[i].Tier = TierProbation
	}
	for i := range 10 {
		l.file.Lessons[i*2].Tier = TierDemoted // l00, l02 ... l18
	}
	l.tallies = map[string]Tally{}
	for i := range 10 {
		l.tallies[fmt.Sprintf("l%02d", i*2)] = Tally{Fires: 2, Refuted: 2}
	}
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
	kept := map[string]bool{}
	for _, ls := range activeOf(t, root).Lessons {
		kept[ls.ID] = true
	}
	dropped := 0
	for i := range 40 {
		id, demoted := fmt.Sprintf("l%02d", i), i < 20 && i%2 == 0
		switch {
		case !demoted && !kept[id]:
			t.Errorf("active.json dropped %s, a probation row", id)
		case demoted && !kept[id] && dropped != i/2:
			t.Errorf("active.json dropped %s while an older demoted row stayed", id)
		case demoted && !kept[id]:
			dropped++
		}
	}
	if dropped == 0 || dropped == 10 {
		t.Errorf("%d demoted rows dropped; the fixture is sized to need some and not all ten", dropped)
	}
	before := snapshot(t, root)
	for i := range l.file.Lessons {
		l.file.Lessons[i].Tier = TierProbation
	}
	if err := l.Save(); !errors.Is(err, ErrActiveOverCap) {
		t.Fatalf("Save over the cap with no demoted row = %v, want ErrActiveOverCap", err)
	}
	if after := snapshot(t, root); after != before {
		t.Error("a refused Save changed the files")
	}
}

func snapshot(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	for _, name := range append([]string{FileSum}, stateFiles...) {
		c, _ := os.ReadFile(filepath.Join(root, DirLearn, project, name))
		b.Write(c)
		b.WriteByte(0)
	}
	return b.String()
}

// TestP6_KeyIsPrivate: the manifest's key is 32 bytes at learn/key, mode
// 0600, and a project name that is not one path element is refused.
func TestP6_KeyIsPrivate(t *testing.T) {
	root := t.TempDir()
	importText(t, open(t, root, t0), twoLessons)
	path := filepath.Join(root, DirLearn, FileKey)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 || info.Size() != 32 {
		t.Errorf("%s: %v, %v; want 32 bytes at 0600", path, info, err)
	}
	for _, p := range []string{"", ".", "..", "a/b"} {
		if _, err := Open(root, p, t0); err == nil {
			t.Errorf("Open(%q) was taken", p)
		}
	}
}
