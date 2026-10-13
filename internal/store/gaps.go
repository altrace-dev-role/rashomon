package store

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/host"
)

// window is the half-open range of record instants a forget removes, in Unix
// milliseconds. An open side is held as the extreme of the range, so one
// comparison serves a bounded side and an unbounded one alike.
type window struct {
	fromMS, toMS int64
}

func newWindow(from, to *time.Time) window {
	w := window{fromMS: math.MinInt64, toMS: math.MaxInt64}
	if from != nil {
		w.fromMS = from.UnixMilli()
	}
	if to != nil {
		w.toMS = to.UnixMilli()
	}
	return w
}

func (w window) holds(ms int64) bool { return ms >= w.fromMS && ms < w.toMS }

func (w window) openBelow() bool { return w.fromMS == math.MinInt64 }
func (w window) openAbove() bool { return w.toMS == math.MaxInt64 }

// ForgetWindow removes every declaration, execution and terminal recorded in
// the half-open window [from, to), leaving a gap record per affected run.
// Coverage records stay: they describe whether the run could be trusted, which
// remains true of it after its content has been forgotten.
//
// A nil bound is unbounded on that side, which is what the two flags are:
// --since names the lower bound and removes everything after it, --before names
// the upper bound and removes everything before it. They are the same operation
// with opposite ends left open, and one code path so that neither end can
// acquire a rule the other does not have.
//
// The records sharing a tool_use_id leave together -- declaration, execution
// and terminal. Removing by timestamp alone can split a set that straddles the
// instant, and a declaration left without its terminal reads exactly like a
// handler that was killed.
func (s *Store) ForgetWindow(from, to *time.Time, now time.Time) ([]Gap, error) {
	w := newWindow(from, to)
	return s.forgetEach(func(dir string) (*Gap, error) { return s.forgetRun(dir, w, now) })
}

// forgetEach applies one forget to every run, oldest first, and goes on past
// a run it fails on: the gaps it returns are every gap written, and the error
// joins one per failed run, naming its directory. Stopping at the first
// failure, as it used to, left every later run unforgotten behind one damaged
// file, and the caller was told only about the damage. A failed run is left
// as it was, or with its gap written and its rewrite unfinished -- the
// overstatement forgetRun's own comment accepts.
func (s *Store) forgetEach(forget func(dir string) (*Gap, error)) ([]Gap, error) {
	names, err := s.Runs()
	if err != nil {
		return nil, err
	}
	var (
		gaps []Gap
		errs []error
	)
	for _, name := range names {
		g, err := forget(filepath.Join(s.root, dirRuns, name))
		if g != nil {
			gaps = append(gaps, *g)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("run %s: %w", name, err))
		}
	}
	if len(errs) == 0 {
		return gaps, nil
	}
	return gaps, fmt.Errorf("%w: %w", ErrForgetIncomplete, errors.Join(errs...))
}

// ErrForgetIncomplete marks a forget that ran over every run and left some it
// could not rewrite: the gaps returned with it were written, and the caller
// may say so. Any other error from a forget came before it changed a run --
// for forget --host, possibly before its store-level record landed -- and
// nothing may be reported as forgotten.
var ErrForgetIncomplete = errors.New("store: forget left runs it could not rewrite")

// lineHead is what forget needs to know about a record to decide its fate.
type lineHead struct {
	RecordedAtMS int64  `json:"recorded_at_unix_ms"`
	ToolUseID    string `json:"tool_use_id"`
	SessionID    string `json:"session_id"`
}

func (s *Store) forgetRun(dir string, w window, now time.Time) (*Gap, error) {
	recordsPath := filepath.Join(dir, FileRecords)
	spillPath := filepath.Join(dir, FileSpill)

	// The ordered stream's lock is held for the whole operation so that no
	// handler appends between the plan and the rewrite.
	records, err := os.OpenFile(recordsPath, os.O_RDWR, fileMode)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if records != nil {
		defer records.Close() //nolint:errcheck // Sync reports the write failure
		unlock, err := lockCounted(records, lockBudget)
		if err != nil {
			return nil, err
		}
		defer unlock()
	}

	recLines, err := readLines(records)
	if err != nil {
		return nil, err
	}
	spillLines, err := readLinesAt(spillPath)
	if err != nil {
		return nil, err
	}

	// Plan: every tool_use_id with any record in the window, from either file.
	doomed := map[string]bool{}
	sessionID := ""
	for _, line := range append(append([][]byte{}, recLines...), spillLines...) {
		var h lineHead
		if json.Unmarshal(line, &h) != nil {
			continue
		}
		if sessionID == "" {
			sessionID = h.SessionID
		}
		if w.holds(h.RecordedAtMS) {
			doomed[h.ToolUseID] = true
		}
	}
	if len(doomed) == 0 {
		return nil, nil
	}
	// The earliest instant actually removed, which is what the gap's own span
	// starts at when the window was left open below: there is no lower bound
	// to name there, and the records are the only thing that says how far back
	// the removal reached.
	earliest := int64(math.MaxInt64)
	keep := func(line []byte) bool {
		var h lineHead
		// A line that does not parse is kept: forget removes what it can
		// identify, and never widens into "remove whatever is here".
		if json.Unmarshal(line, &h) != nil || !doomed[h.ToolUseID] {
			return true
		}
		if h.RecordedAtMS < earliest {
			earliest = h.RecordedAtMS
		}
		return false
	}
	keptRec, removedRec := partition(recLines, keep)
	keptSpill, removedSpill := partition(spillLines, keep)
	if removedRec+removedSpill == 0 {
		return nil, nil
	}
	if sessionID == "" {
		sessionID = filepath.Base(dir)
	}

	// The gap spans what was removed. A bound the caller named is that bound;
	// an open side is closed by what was there -- the earliest record removed
	// below, and this instant above -- so the record never claims a window
	// wider than the one it emptied.
	from, to := w.fromMS, w.toMS
	if w.openBelow() {
		from = earliest
	}
	if w.openAbove() {
		to = now.UnixMilli()
	}

	// The gap lands before anything is removed. A failure between the two
	// leaves a gap for records that still exist -- an overstatement a reader
	// can see -- rather than a deletion nothing admits to.
	g := Gap{
		Type:           TypeGap,
		SchemaVersion:  SessionSchemaVersion,
		RecordedAtMS:   now.UnixMilli(),
		SessionID:      sessionID,
		Reason:         GapForget,
		FromUnixMS:     from,
		ToUnixMS:       to,
		RemovedRecords: removedRec + removedSpill,
	}
	if err := s.AppendGap(g); err != nil {
		return nil, err
	}

	if records != nil && removedRec > 0 {
		if err := rewrite(records, keptRec); err != nil {
			return &g, err
		}
	}
	if removedSpill > 0 {
		if err := rewriteAt(spillPath, keptSpill); err != nil {
			return &g, err
		}
	}
	return &g, nil
}

func partition(lines [][]byte, keep func([]byte) bool) (kept [][]byte, removed int) {
	for _, line := range lines {
		if keep(line) {
			kept = append(kept, line)
		} else {
			removed++
		}
	}
	return kept, removed
}

// readLines reads every non-empty line of an open file from its start. A nil
// file has no lines.
//
// Its callers rewrite the file from what it returns, so a line it cannot hold
// is an error here rather than a count: dropping it would delete a record
// that nothing names in a gap, and writing it back needs the line itself.
// The run is left as it is, and the forget goes on to the next (forgetEach).
func readLines(f *os.File) ([][]byte, error) {
	if f == nil {
		return nil, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var lines [][]byte
	tooLong := false
	err := forEachLine(f, func(line []byte) {
		if line == nil {
			tooLong = true
			return
		}
		lines = append(lines, line)
	})
	if err == nil && tooLong {
		err = errLineTooLong
	}
	return lines, err
}

func readLinesAt(path string) ([][]byte, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only
	return readLines(f)
}

// rewrite replaces an open file's content in place.
//
// In place rather than temp-and-rename on purpose. The append lock is taken on
// the file's inode; a handler blocked on that lock holds a descriptor to this
// inode and will append to it once the lock is released. A rename would leave
// that handler appending to an orphan.
func rewrite(f *os.File, lines [][]byte) error {
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := f.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return syncFull(f)
}

// rewriteAt rewrites the spill file under its lock. Spill writers wait only
// briefly for that lock and then append regardless, so a rewrite that takes
// longer than their patience can race one; the rewrite is a few microseconds,
// their patience is fifty milliseconds, and the loss on the far side of that
// is one terminal, which reads as unverified rather than as anything false.
func rewriteAt(path string, lines [][]byte) error {
	f, err := os.OpenFile(path, os.O_RDWR, fileMode)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // Sync reports the write failure
	unlock, err := lockCounted(f, lockBudget)
	if err != nil {
		return err
	}
	defer unlock()
	return rewrite(f, lines)
}

// canonicalHost is host.Canonical, under a name ForgetHost's parameter does
// not shadow.
func canonicalHost(raw string) (string, bool) { return host.Canonical(raw) }

// ErrNotAHostname is ForgetHost's refusal of a value that names no host: a
// path, a credential, whitespace inside, or nothing left once a scheme and a
// port are removed. It does not carry the value, which may hold a credential.
var ErrNotAHostname = errors.New("store: forget --host: the value is not a hostname")

// declHead is what a host-scoped forget needs from a record: the join key and
// the hostnames a declaration named.
type declHead struct {
	Type      string   `json:"type"`
	ToolUseID string   `json:"tool_use_id"`
	SessionID string   `json:"session_id"`
	Hosts     []string `json:"hosts"`
	SSHHosts  []string `json:"ssh_hosts"`
}

// ForgetHost removes every record of every call that NAMED the given host,
// across all runs, leaving a gap record per affected run.
//
// It removes whole calls rather than editing the host out of a declaration's
// list. Editing would keep more information -- a call that named two hosts
// loses the record of the other this way -- but it means rewriting a record's
// contents, and every other removal in this store works by dropping whole
// lines whose tool_use_id is doomed. One mechanism, already tested, is worth
// more here than the extra field survived: the caller asked for that host's
// records to be gone, and the declaration IS that host's record.
//
// The host is matched against both declared lists, hosts and ssh_hosts: an
// ssh host is as much something the session named as an HTTPS one, and the
// report reads its own lists from exactly these two fields, so a forgotten
// host returns nowhere. The gap record carries the host's keyed digest, and the
// sandbox section, whose trail is nono's file and not ours to edit, filters
// the host out by it rather than implying the trail forgot.
//
// THE NAME IS CANONICALISED FIRST, by host.Canonical, the function every host
// this store and the report hold has been through: the declarations' lists
// and the sandbox's trail. Matched and digested as typed, "GitHub.com",
// "github.com:443" and "https://github.com/" each named a host nothing held,
// so the forget removed nothing and suppressed nothing while the command said
// later reports would leave it out. A value Canonical refuses names no
// destination at all and is refused here, before anything is written.
func (s *Store) ForgetHost(host string, now time.Time) ([]Gap, error) {
	if host == "" {
		return nil, errors.New("store: forget --host needs a host")
	}
	host, ok := canonicalHost(host)
	if !ok {
		return nil, ErrNotAHostname
	}
	// The store-level record lands FIRST and ALWAYS, removed records or not.
	// First, for the reason the gaps land before their rewrites: a failure
	// after it overstates the forget, which a reader can see, rather than
	// leaving a host suppressed nowhere. Always, because the report reads a
	// source this store never held -- the sandbox's trail -- and a forget
	// that wrote nothing when it removed nothing left the host listed there
	// (#45).
	if err := s.recordForgottenHost(s.HostDigest(host), now); err != nil {
		return nil, err
	}
	return s.forgetEach(func(dir string) (*Gap, error) { return s.forgetHostInRun(dir, host, now) })
}

// recordForgottenHost appends the store-level forget record for a digest,
// unless the file already holds one for it.
//
// ONCE, checked under the file's lock: every forget --host used to append a
// line, so forgetting the same host again wrote a second record that says
// nothing the first did not, to a file no size cap covers (Evict counts runs/
// only, and a window forget never touches it). The check reads the file it
// is about to append to, under the lock a concurrent forget would take, so
// two forgets of one host cannot both find it absent.
func (s *Store) recordForgottenHost(digest string, now time.Time) error {
	line, err := marshalLine(ForgottenHostRecord{
		Type:          TypeForgottenHost,
		SchemaVersion: SessionSchemaVersion,
		RecordedAtMS:  now.UnixMilli(),
		HostDigest:    digest,
	})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.root, FileForgottenHosts), appendFlags, fileMode)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // Sync below reports the write failure
	unlock, err := lockCounted(f, lockBudget)
	if err != nil {
		return err
	}
	defer unlock()

	// Not readLines: this file is only appended to, never rewritten from
	// what is read, so a line too long to hold is passed over like any line
	// that does not parse (REC-32). As an error it would refuse every later
	// forget --host behind one damaged line.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	held := false
	err = forEachLine(f, func(l []byte) {
		var r ForgottenHostRecord
		if json.Unmarshal(l, &r) == nil && r.Type == TypeForgottenHost && r.HostDigest == digest {
			held = true
		}
	})
	if err != nil || held {
		return err
	}
	if err := appendHealed(f, line); err != nil {
		return err
	}
	return syncFull(f)
}

func (s *Store) forgetHostInRun(dir, host string, now time.Time) (*Gap, error) {
	recordsPath := filepath.Join(dir, FileRecords)
	spillPath := filepath.Join(dir, FileSpill)

	records, err := os.OpenFile(recordsPath, os.O_RDWR, fileMode)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if records != nil {
		defer records.Close() //nolint:errcheck // Sync reports the write failure
		unlock, err := lockCounted(records, lockBudget)
		if err != nil {
			return nil, err
		}
		defer unlock()
	}

	recLines, err := readLines(records)
	if err != nil {
		return nil, err
	}
	spillLines, err := readLinesAt(spillPath)
	if err != nil {
		return nil, err
	}

	// Plan: every tool_use_id whose DECLARATION named the host. Only a
	// declaration carries hostnames, so only a declaration can nominate a call
	// for removal; its execution and terminal follow it out by id, exactly as
	// they do for a window forget.
	doomed := map[string]bool{}
	sessionID := ""
	for _, line := range append(append([][]byte{}, recLines...), spillLines...) {
		var h declHead
		if json.Unmarshal(line, &h) != nil {
			continue
		}
		if sessionID == "" {
			sessionID = h.SessionID
		}
		if h.Type != TypeDeclaration {
			continue
		}
		if namesHost(h, host) {
			doomed[h.ToolUseID] = true
		}
	}
	if len(doomed) == 0 {
		return nil, nil
	}

	earliest, latest := int64(math.MaxInt64), int64(math.MinInt64)
	keep := func(line []byte) bool {
		var h lineHead
		if json.Unmarshal(line, &h) != nil || !doomed[h.ToolUseID] {
			return true
		}
		if h.RecordedAtMS < earliest {
			earliest = h.RecordedAtMS
		}
		if h.RecordedAtMS > latest {
			latest = h.RecordedAtMS
		}
		return false
	}
	keptRec, removedRec := partition(recLines, keep)
	keptSpill, removedSpill := partition(spillLines, keep)
	if removedRec+removedSpill == 0 {
		return nil, nil
	}
	if sessionID == "" {
		sessionID = filepath.Base(dir)
	}

	// The gap spans what was actually removed rather than all of time: a
	// host-scoped forget names no window, so the records themselves are the
	// only honest bounds.
	g := Gap{
		Type:           TypeGap,
		SchemaVersion:  SessionSchemaVersion,
		RecordedAtMS:   now.UnixMilli(),
		SessionID:      sessionID,
		Reason:         GapForgetHost,
		FromUnixMS:     earliest,
		ToUnixMS:       latest,
		RemovedRecords: removedRec + removedSpill,
		HostDigest:     s.HostDigest(host),
	}
	// The gap lands first, as it does for a window forget: a failure between
	// the two leaves a gap for records that still exist, an overstatement a
	// reader can see, rather than a deletion nothing admits to.
	if err := s.AppendGap(g); err != nil {
		return nil, err
	}

	if records != nil && removedRec > 0 {
		if err := rewrite(records, keptRec); err != nil {
			return &g, err
		}
	}
	if removedSpill > 0 {
		if err := rewriteAt(spillPath, keptSpill); err != nil {
			return &g, err
		}
	}
	return &g, nil
}

// namesHost reports whether a declaration named the host, in either list.
func namesHost(h declHead, host string) bool {
	for _, list := range [][]string{h.Hosts, h.SSHHosts} {
		for _, got := range list {
			if got == host {
				return true
			}
		}
	}
	return false
}

// HostDigest is the keyed identifier a gap record carries for a forgotten
// host.
//
// HMAC under the per-install key, for the reason set out on Gap.HostDigest: the
// report has to recognise the host again while the store must not hold its
// name. Two installs produce different digests for the same host, so a store
// cannot be tested against a dictionary of hostnames.
func (s *Store) HostDigest(host string) string {
	if host == "" {
		return ""
	}
	m := hmac.New(sha256.New, s.Key())
	m.Write([]byte("forget-host\x00"))
	m.Write([]byte(host))
	return hex.EncodeToString(m.Sum(nil))
}

// ForgottenHost reports whether a host has been removed by a host-scoped
// forget, by recomputing its keyed digest and looking for it among the gaps
// and the store-level forget records.
//
// BOTH SOURCES. The store-level records cover every forget since they
// existed; the gaps cover the forgets made before, which wrote only a gap.
//
// Returned as a predicate rather than a set of names because the names are not
// recoverable: the caller asks about a host it already has, which is exactly
// what the report does for each destination it observed.
func (s *Store) ForgottenHost() (func(string) bool, error) {
	times, err := s.forgottenDigestTimes()
	if err != nil {
		return nil, err
	}
	if len(times) == 0 {
		// A predicate that allocates and hashes nothing on the overwhelmingly
		// common path where nothing has been forgotten.
		return func(string) bool { return false }, nil
	}
	return func(host string) bool { _, ok := times[s.HostDigest(host)]; return ok }, nil
}

// ForgottenHostBefore is ForgottenHost's time-scoped sibling.
//
// A host's own name is forgotten forever once any forget --host names it --
// ForgottenHost's predicate has no notion of time, because the sandbox trail
// it exists for has none either: nono's file is not this store's to edit, so
// every one of its past events stays exactly as reachable as the day they
// were written, and the predicate's job is to keep them out of the report
// regardless of when they are read.
//
// A declaration is different: it is this store's own record, forget --host
// removes it on success, and the DECLARATION'S OWN recorded_at_unix_ms says
// whether that removal had a chance to run before it was written. One a
// forget already decided to remove but could not -- a run whose file could
// not be opened, say -- predates the forget and is exactly the stale
// leftover #45 and TRU-58 exist to stop reappearing. One recorded after the
// forget is a fresh declaration of a name that happens to be the same, which
// is real information about THIS session and must not disappear because an
// earlier, unrelated session once asked to forget it -- the whole reason
// forget --host clears the project baseline rather than blocking the name.
func (s *Store) ForgottenHostBefore() (func(host string, declaredAtMS int64) bool, error) {
	times, err := s.forgottenDigestTimes()
	if err != nil {
		return nil, err
	}
	if len(times) == 0 {
		return func(string, int64) bool { return false }, nil
	}
	return func(host string, declaredAtMS int64) bool {
		forgotMS, ok := times[s.HostDigest(host)]
		return ok && declaredAtMS <= forgotMS
	}, nil
}

// forgottenDigestTimes reads both sources ForgottenHost and
// ForgottenHostBefore share -- the gaps a forget wrote on success, and the
// store-level record it writes whatever it removed -- and keeps, per digest,
// the LATEST recorded_at_unix_ms either source carries. Latest, not first,
// because a host forgotten more than once must compare against the most
// recent attempt: a declaration from between two forgets of the same host is
// still stale against the second one.
func (s *Store) forgottenDigestTimes() (map[string]int64, error) {
	gaps, _, err := s.ReadGaps()
	if err != nil {
		return nil, err
	}
	times := map[string]int64{}
	bump := func(digest string, candidateMS int64) {
		if digest == "" {
			return
		}
		if cur, ok := times[digest]; !ok || candidateMS > cur {
			times[digest] = candidateMS
		}
	}
	for _, g := range gaps {
		if g.Reason == GapForgetHost {
			bump(g.HostDigest, g.RecordedAtMS)
		}
	}
	err = eachLine(filepath.Join(s.root, FileForgottenHosts), func(line []byte) {
		var r ForgottenHostRecord
		if json.Unmarshal(line, &r) == nil && r.Type == TypeForgottenHost {
			bump(r.HostDigest, r.RecordedAtMS)
		}
	})
	if err != nil {
		return nil, err
	}
	return times, nil
}
