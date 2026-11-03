package learn

import (
	"bytes"
	"cmp"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The learner's files under the store root: DirLearn/FileKey, one per install,
// and DirLearn/<project>/ holding the three state files and their manifest.
const (
	DirLearn    = "learn"
	FileKey     = "key"
	FileLessons = "lessons.json"
	FileLabels  = "labels.ndjson"
	FileActive  = "active.json"
	FileSum     = "state.sum"
	// ActiveMaxBytes caps active.json, the file delivery reads.
	ActiveMaxBytes = 64 << 10
	SchemaLessons  = "lessons/1"
	SchemaActive   = "active/1"
)

// stateFiles are the files the manifest covers, in the order it signs them.
var stateFiles = []string{FileLessons, FileLabels, FileActive}

// Owns reports whether a regular file of this name, directly under DirLearn
// (inProject false) or in a project's directory there (inProject true), is
// one this package writes, its replacement in flight included: purge's test.
func Owns(name string, inProject bool) bool {
	files := []string{FileKey}
	if inProject {
		files = append(slices.Clone(stateFiles), FileSum)
	}
	for _, f := range files {
		if name == f || strings.HasPrefix(name, "."+f+".tmp-") {
			return true
		}
	}
	return false
}

// ErrActiveOverCap is Save's refusal: active.json would pass ActiveMaxBytes
// with every demoted row dropped. Nothing is written; the old files stand.
// ErrReadOnly is Save or AppendLabel on a ledger OpenReadOnly returned: it
// never writes.
var ErrReadOnly = errors.New("learn: the ledger was opened read-only; nothing was written")

var ErrActiveOverCap = errors.New("learn: active.json would pass 64 KiB with every demoted row dropped; nothing was written")

// Lesson is one row of lessons.json and of active.json.
type Lesson struct {
	ID          string `json:"id"`
	Tier        Tier   `json:"tier"`
	Text        string `json:"text"`
	SinceUnixMS int64  `json:"since_unix_ms"`
	// ApprovedUnixMS is when a person last approved the lesson at a terminal,
	// 0 for never. A quarantine sends the tier back to proposed and keeps it,
	// so "approved now" reads the tier as well.
	ApprovedUnixMS int64 `json:"approved_unix_ms,omitempty"`
}

// AwaitsApproval reports whether a person has yet to approve the lesson for
// this build: it is proposed, or it holds an approved tier with no stamp,
// which a build before the stamp wrote and Pick never sends (#156 D2).
func (ls Lesson) AwaitsApproval() bool {
	return ls.Tier == TierProposed || ls.ApprovedUnixMS == 0
}

// Label is one row of labels.ndjson: one fire of a lesson and what it came
// to. Its fields are LABEL-WRITER1's contract, pinned by a test.
type Label struct {
	LessonID  string      `json:"lesson_id"`
	SessionID string      `json:"session_id"`
	AtUnixMS  int64       `json:"at_unix_ms"`
	Label     steer.Label `json:"label"`
}

type lessonsFile struct {
	Schema              string   `json:"schema"`
	QuarantinedAtUnixMS int64    `json:"quarantined_at_unix_ms,omitempty"`
	Lessons             []Lesson `json:"lessons"`
}

type activeFile struct {
	Schema      string   `json:"schema"`
	Quarantined bool     `json:"quarantined"`
	Lessons     []Lesson `json:"lessons"`
}

// Ledger is one project's lessons as read, with their tallies.
//
// state.sum is tamper evidence for writes the record never saw, not a
// security boundary: it is an HMAC under DirLearn/FileKey (mode 0600), and an
// agent running with the user's rights can read that key.
type Ledger struct {
	root, dir string
	file      lessonsFile
	tallies   map[string]Tally
	// Quarantined is true when this Open found the state written by
	// something other than this package and quarantined it.
	Quarantined bool
	// readOnly is set by OpenReadOnly: Save refuses, so a ledger a report
	// holds cannot be written even by a caller that mutates it.
	readOnly bool
}

// Open reads project's ledger under root and checks its manifest. A state
// that does not match it, or has none, is quarantined at once and written
// back so (fail closed): every lesson returns to proposed, to be approved
// again, and active.json holds none. No state at all is an empty ledger.
func Open(root, project string, now time.Time) (*Ledger, error) {
	return openAt(root, project, now, true)
}

// OpenReadOnly is Open that never writes: a state that does not check is
// read as quarantined in memory (Quarantined true, every lesson proposed,
// QuarantinedAt zero, since no quarantine was recorded) and left on disk as
// it is. A report reads the ledger this way (card REPORT1b): a question must
// not change the learning state, and the next learn verb that opens it still
// finds the mismatch and records the quarantine itself.
func OpenReadOnly(root, project string) (*Ledger, error) {
	return openAt(root, project, time.Time{}, false)
}

// openAt is Open and OpenReadOnly: write is whether a mismatch is quarantined
// on disk.
func openAt(root, project string, now time.Time, write bool) (*Ledger, error) {
	if err := projectElement(project); err != nil {
		return nil, err
	}
	l := &Ledger{root: root, dir: filepath.Join(root, DirLearn, project), tallies: map[string]Tally{}, readOnly: !write}
	files, sum, err := l.readState()
	if err != nil || (files == nil && sum == nil) {
		l.file.Schema = SchemaLessons
		return l, err
	}
	key, err := os.ReadFile(filepath.Join(root, DirLearn, FileKey))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	l.Quarantined = len(key) == 0 || !hmac.Equal(sum, []byte(sign(key, files)))
	if b := files[FileLessons]; b != nil && decode(b, &l.file) != nil && !l.Quarantined {
		return nil, fmt.Errorf("learn: %s does not parse", FileLessons)
	}
	l.file.Schema = SchemaLessons
	l.tallies = tallies(files[FileLabels])
	if !l.Quarantined {
		l.retier()
		return l, nil
	}
	for i := range l.file.Lessons {
		l.file.Lessons[i].Tier = TierProposed
	}
	if !write {
		return l, nil
	}
	l.file.QuarantinedAtUnixMS = now.UnixMilli()
	return l, l.Save()
}

// projectElement refuses a project name that is not one path element.
func projectElement(project string) error {
	if project == "" || project == "." || project == ".." || strings.ContainsAny(project, `/\`) {
		return fmt.Errorf("learn: project %q is not one path element", project)
	}
	return nil
}

// readState reads the three state files (nil where absent) and the manifest.
// Both nil is no state.
func (l *Ledger) readState() (map[string][]byte, []byte, error) {
	var files map[string][]byte
	for _, name := range stateFiles {
		b, err := os.ReadFile(filepath.Join(l.dir, name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		if files == nil {
			files = map[string][]byte{}
		}
		if b == nil {
			b = []byte{}
		}
		files[name] = b
	}
	sum, err := os.ReadFile(filepath.Join(l.dir, FileSum))
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	if files == nil && sum == nil {
		return nil, nil, err
	}
	if files == nil {
		files = map[string][]byte{}
	}
	return files, bytes.TrimSpace(sum), err
}

// decode is the learner's JSON read of its own files read whole and of an
// import file. Only the learn verb calls it: recap pending, a hook command,
// links this package but reads active.json through decodeActive, under its
// cap, so RES-5 allows the two by name and a hook-path caller of Open would
// show up as a new one.
func decode(b []byte, v any) error { return json.Unmarshal(b, v) }

// sign is the manifest over files read whole (Open's and Save's reads):
// signDigests over each one's SHA-256.
func sign(key []byte, files map[string][]byte) string {
	digests := make([]string, len(stateFiles))
	for i, name := range stateFiles {
		digests[i] = "-"
		if b, ok := files[name]; ok {
			digests[i] = digestHex(b)
		}
	}
	return signDigests(key, digests)
}

// signDigests is the manifest: an HMAC over each state file's name and
// digest, in stateFiles' order, "-" for one that is absent. Open and Save
// reach it through sign and ReadActive with the digests it streamed, so the
// two readers check one manifest.
func signDigests(key []byte, digests []string) string {
	m := hmac.New(sha256.New, key)
	for i, name := range stateFiles {
		fmt.Fprintf(m, "%s\x00%s\n", name, digests[i])
	}
	return hex.EncodeToString(m.Sum(nil))
}

// digestHex is b's SHA-256 in hex.
func digestHex(b []byte) string {
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}

// tallies counts labels.ndjson by lesson; a line that does not parse is
// not a fire.
func tallies(b []byte) map[string]Tally {
	out := map[string]Tally{}
	for line := range bytes.SplitSeq(b, []byte("\n")) {
		var lb Label
		if decode(line, &lb) != nil || lb.LessonID == "" {
			continue
		}
		t := out[lb.LessonID]
		t.add(lb.Label)
		out[lb.LessonID] = t
	}
	return out
}

// retier moves every lesson by its tally.
func (l *Ledger) retier() {
	for i, ls := range l.file.Lessons {
		l.file.Lessons[i].Tier = ls.Tier.Next(l.tallies[ls.ID])
	}
}

// Lessons returns the lessons in their total order, Since then ID.
func (l *Ledger) Lessons() []Lesson {
	out := slices.Clone(l.file.Lessons)
	slices.SortFunc(out, byAge)
	return out
}

// Tally returns a lesson's labels.
func (l *Ledger) Tally(id string) Tally { return l.tallies[id] }

// QuarantinedAt is when the ledger was last quarantined; zero for never.
func (l *Ledger) QuarantinedAt() time.Time {
	if l.file.QuarantinedAtUnixMS == 0 {
		return time.Time{}
	}
	return time.UnixMilli(l.file.QuarantinedAtUnixMS)
}

func byAge(a, b Lesson) int {
	return cmp.Or(cmp.Compare(a.SinceUnixMS, b.SinceUnixMS), strings.Compare(a.ID, b.ID))
}

// Import adds the lessons of a shared file (the lessons.json shape) at
// proposed, whatever tier each names (PF-HD5: a shared lesson is approved
// here or not at all). A row whose id is taken, malformed or whose text is
// empty or over store.MaxSteerTextBytes is skipped. A row whose id is
// secret-shaped (the screens a lesson id runs when it renders: a deny-list
// shape, a high-entropy run) refuses the whole file with slot.ErrSecretShaped
// and adds nothing: an id is stored and printed by the verbs, so an import
// never carries a token in as one, and the error names the row, not its
// value (Q4). A host- or address-shaped id (slot.RefuseHosts) refuses the
// whole file the same way: an id is model-visible and unquoted, and the
// error names the row, not the id. The caller saves.
func (l *Ledger) Import(r io.Reader, now time.Time) (added, skipped int, err error) {
	b, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err != nil {
		return 0, 0, err
	}
	var in lessonsFile
	if err := decode(b, &in); err != nil {
		return 0, 0, fmt.Errorf("learn: the import file is not a lessons file: %w", err)
	}
	for i, ls := range in.Lessons {
		if idForm(ls.ID) && errors.Is(slot.LessonID(ls.ID).Validate(), slot.ErrSecretShaped) {
			return 0, 0, fmt.Errorf("learn: import row %d: the lesson id is secret-shaped, so nothing was imported: %w", i+1, slot.ErrSecretShaped)
		}
		if idForm(ls.ID) && slot.RefuseHosts(ls.ID) != nil {
			return 0, 0, fmt.Errorf("learn: import row %d: the lesson id is shaped like a host or an address, so nothing was imported", i+1)
		}
	}
	for _, ls := range in.Lessons {
		if !idForm(ls.ID) || ls.Text == "" || len(ls.Text) > store.MaxSteerTextBytes || l.index(ls.ID) >= 0 {
			skipped++
			continue
		}
		l.file.Lessons = append(l.file.Lessons, Lesson{ID: ls.ID, Tier: TierProposed, Text: ls.Text, SinceUnixMS: now.UnixMilli()})
		added++
	}
	return added, skipped, nil
}

// idForm is the slot id charset (verify.SlotID): ^[a-z0-9][a-z0-9._:@-]{0,63}$.
func idForm(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for i, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || i > 0 && strings.ContainsRune("._:@-", c)) {
			return false
		}
	}
	return true
}

func (l *Ledger) index(id string) int {
	return slices.IndexFunc(l.file.Lessons, func(ls Lesson) bool { return ls.ID == id })
}

// Approve moves a lesson that awaits approval to the tier its tally gives an
// approved one and stamps now as its approval time: a proposed lesson, or
// one a build before the stamp approved. The caller has had a person type
// the one-time code; this does not ask. The caller saves.
func (l *Ledger) Approve(id string, now time.Time) error {
	i := l.index(id)
	if i < 0 {
		return fmt.Errorf("no lesson %q", id)
	}
	if !l.file.Lessons[i].AwaitsApproval() {
		return fmt.Errorf("lesson %q is %s, not proposed; nothing to approve", id, l.file.Lessons[i].Tier)
	}
	l.file.Lessons[i].Tier = TierProbation.Next(l.tallies[id])
	l.file.Lessons[i].ApprovedUnixMS = now.UnixMilli()
	l.Quarantined = false
	return nil
}

// Withdraw sends an approved lesson back to proposed and clears its stamp,
// so it leaves active.json and reaches the agent again only once a person
// approves it (#156 D3). The caller has had a person type the one-time
// code; the caller saves.
func (l *Ledger) Withdraw(id string) error {
	i := l.index(id)
	if i < 0 {
		return fmt.Errorf("no lesson %q", id)
	}
	if l.file.Lessons[i].Tier == TierProposed {
		return fmt.Errorf("lesson %q is proposed; nothing to withdraw", id)
	}
	l.file.Lessons[i].Tier = TierProposed
	l.file.Lessons[i].ApprovedUnixMS = 0
	return nil
}

// AppendLabel appends one label row and saves: LABEL-WRITER1's entry.
func (l *Ledger) AppendLabel(lb Label) error {
	// Before labels.ndjson is opened: the row would land unsigned, Save would
	// then refuse, and the next Open would quarantine every lesson.
	if l.readOnly {
		return ErrReadOnly
	}
	if l.index(lb.LessonID) < 0 || !slices.Contains(steer.AllLabels(), lb.Label) {
		return fmt.Errorf("learn: label %q for lesson %q is not one this ledger takes", lb.Label, lb.LessonID)
	}
	b, err := json.Marshal(lb)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(l.dir, FileLabels), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	t := l.tallies[lb.LessonID]
	t.add(lb.Label)
	l.tallies[lb.LessonID] = t
	l.retier()
	return l.Save()
}

// Save writes lessons.json and active.json, each whole or not at all, and
// signs the three state files. active.json past ActiveMaxBytes drops its
// oldest demoted rows first; still over, nothing is written. A ledger opened
// with OpenReadOnly returns ErrReadOnly and writes nothing.
func (l *Ledger) Save() error {
	if l.readOnly {
		return ErrReadOnly
	}
	active, err := l.activeBytes()
	if err != nil {
		return err
	}
	slices.SortFunc(l.file.Lessons, byAge)
	lessons, err := json.Marshal(l.file)
	if err != nil {
		return err
	}
	key, err := ensureKey(filepath.Join(l.root, DirLearn, FileKey))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return err
	}
	for name, b := range map[string][]byte{FileLessons: append(lessons, '\n'), FileActive: active} {
		if err := store.ReplaceFile(filepath.Join(l.dir, name), b); err != nil {
			return err
		}
	}
	files, _, err := l.readState()
	if err != nil {
		return err
	}
	return store.ReplaceFile(filepath.Join(l.dir, FileSum), []byte(sign(key, files)+"\n"))
}

// activeBytes is active.json: every approved lesson, in order, under the cap.
func (l *Ledger) activeBytes() ([]byte, error) {
	rows := []Lesson{}
	for _, ls := range l.Lessons() {
		if ls.Tier != TierProposed {
			rows = append(rows, ls)
		}
	}
	for {
		b, err := json.Marshal(activeFile{Schema: SchemaActive, Quarantined: l.Quarantined, Lessons: rows})
		if err != nil || len(b)+1 <= ActiveMaxBytes {
			return append(b, '\n'), err
		}
		i := slices.IndexFunc(rows, func(ls Lesson) bool { return ls.Tier == TierDemoted })
		if i < 0 {
			return nil, ErrActiveOverCap
		}
		rows = slices.Delete(rows, i, i+1)
	}
}

// ensureKey reads the learner's key, creating it (32 random bytes, 0600) on
// first use.
func ensureKey(path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil && len(b) == 32 {
		return b, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := store.ReplaceFile(path, key); err != nil {
		return nil, err
	}
	return key, nil
}
