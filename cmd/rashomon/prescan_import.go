package main

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/history"
	"github.com/altrace-dev-role/rashomon/internal/replay"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The prescan's import (docs/autosteer-prescan.md, B2c): a set of Claude Code
// transcript files becomes, per session, a summary of what its gate runs were,
// ready for B3a to turn into the seed file. Nothing here is written to disk
// but the replay worlds, which are made under a scratch directory the caller
// owns and removed when their session is done. It is unexported and no
// subcommand calls it yet.
//
// # What it does
//
//  1. Every file is inspected once (replay.Inspect): its digest, the sessions
//     it names, whether it is a session's own or a subagent's, and when it
//     begins. A file that is only a subagent's, and whose session has no file
//     of its own, belongs to no session: subagent files are never sessions.
//  2. A session is its Claude Code sessionId. Its files are put in the order of
//     replay.OrderFiles (own files before subagents', then by earliest
//     timestamp, then by digest; the path, the listing and the modification
//     time play no part) and merged into one timeline (replay.WalkSession).
//  3. A session the live recorder already recorded (its run directory exists in
//     the real store) is excluded, and an earlier import of it is removed from
//     the ledger. The check only looks (store.RunExistsAt); it opens nothing.
//  4. The merged timeline is replayed through the live recorder into a world of
//     its own (replayEvents), one session at a time, and the world's records
//     are read through verify, as a Stop reads them, into the summary.
//  5. The summary replaces the session's entry in the ledger.
//
// # The ledger
//
// importLedger holds, per imported session, HMAC(install key, sessionId) and the
// digests of the files that contributed, with the session's summary. The key is
// the session's own parameter: the real install key is B3a's to read. A rescan
// of a session puts its new entry over the old, never beside it; a session that
// is now recorded live is removed. The family totals are computed from the
// entries (Totals), never kept as running counters, so no sequence of scans can
// count a session twice (DEDUP-1) and no session is in both the ledger and the
// live store (DEDUP-2).
//
// # Matches recorded inputs, not executed inputs
//
// What a summary says of a run has two separate parts, kept in two places:
//
//   - Reconstruction (the Verified grade of a run, and everything counted
//     "verified" below): replay reproduced the recorder's view of the inputs the
//     transcript recorded, with no gap before the run. Such a run "matches
//     recorded inputs". It is never "verified to match executed inputs", because
//     no transcript field records the input a PreToolUse hook may have rewritten
//     (flagRewriteAbsent).
//   - Source completeness and lifecycle (sessionBasis): the SessionStart the
//     replay feeds is scaffolding that lets the recorder speak at all, never
//     evidence that capture was complete; whether the session ended, and how, is
//     unknown (flagSessionEndUnknown). Neither raises any coverage to verified.
//
// A consumer that needs more than "consistent with the transcript" (a safety
// conclusion, B3e) must read sessionBasis and refuse a session whose basis
// does not give it.

// SafetyGrade reports whether the basis supports a conclusion about what
// executed rather than about what the transcript recorded.
func (b sessionBasis) SafetyGrade() bool { return b.ExecutedInputsVerified && b.CaptureComplete }

// importConfig configures an import.
type importConfig struct {
	// Identity is the key and install id the replay worlds are pinned to, and
	// the key the ledger's HMAC is made under. The zero value is the replay's
	// own constant (defaultReplayIdentity), under which the family keys are
	// comparable with other replays and with nothing live. Replaying under the
	// real install key is B3a's.
	Identity replayIdentity
	// Scratch is a directory the import makes its worlds under, one at a time,
	// and removes them from. It must not overlap the real store or Claude Code
	// configuration (newReplayWorld).
	Scratch string
	// LiveRoot is the real store's root, "" for none: a session whose run is
	// there is excluded. It is only looked at.
	LiveRoot string
	// Rescan replays a session even when the ledger already holds it with the
	// same files.
	Rescan bool
	// Ctx stops the import between events.
	Ctx context.Context
	// MaxCallIDs overrides the merge's cap on tool_use_id digests; zero is
	// replay.DefaultMaxCallIDs.
	MaxCallIDs int
	// Limits bound the call reader; zero is history.DefaultLimits.
	Limits history.Limits
}

// importer imports sessions into a ledger.
type importer struct {
	cfg    importConfig
	ledger *importLedger
}

func newImporter(cfg importConfig) (*importer, error) {
	if cfg.Identity == (replayIdentity{}) {
		cfg.Identity = defaultReplayIdentity
	}
	key, err := hex.DecodeString(cfg.Identity.KeyHex)
	if err != nil || len(key) == 0 {
		return nil, errors.New("import: the identity's key is not hex")
	}
	if cfg.Scratch == "" {
		return nil, errors.New("import: no scratch directory")
	}
	return &importer{cfg: cfg, ledger: newImportLedger(key)}, nil
}

// session statuses.
const (
	statusImported  = "imported"
	statusUnchanged = "unchanged"
	statusLive      = "excluded_live"
)

// sessionOutcome is what an import did with one session.
type sessionOutcome struct {
	SessionHMAC string
	Status      string
	Files       int
	// Removed: an earlier import of the session was taken out of the ledger
	// because it is recorded live now.
	Removed  bool
	Replaced bool
}

// importResult is what an import did.
type importResult struct {
	Sessions []sessionOutcome
	// Files were inspected; Duplicates were byte for byte another file;
	// Unreadable could not be opened or read; Orphans were subagent files whose
	// session has no file of its own.
	Files, Duplicates, Unreadable, Orphans int
}

// Import imports every session the files name. Files are taken in sorted path
// order and by their bytes; the same files in any other order, or other
// directories, give the same result. Sessions are replayed one at a time.
func (im *importer) Import(paths []string) (*importResult, error) {
	res := &importResult{}
	paths = append([]string(nil), paths...)
	sort.Strings(paths)
	var kept []string
	var metas []replay.FileMeta
	for _, p := range paths {
		m, err := inspectFile(p)
		if err != nil {
			res.Unreadable++
			continue
		}
		res.Files++
		kept = append(kept, p)
		metas = append(metas, m)
	}
	own := map[string]bool{}
	named := map[string]bool{}
	for _, m := range metas {
		for _, s := range m.Sessions {
			named[s.ID] = true
			own[s.ID] = own[s.ID] || s.Main
		}
	}
	var ids []string
	for id := range named {
		if own[id] {
			ids = append(ids, id)
		} else {
			res.Orphans++
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := im.cfg.ctxErr(); err != nil {
			return res, err
		}
		out, err := im.session(id, kept, metas, res)
		if err != nil {
			return res, err
		}
		res.Sessions = append(res.Sessions, out)
	}
	return res, nil
}

func (c importConfig) ctxErr() error {
	if c.Ctx == nil {
		return nil
	}
	return c.Ctx.Err()
}

func inspectFile(path string) (replay.FileMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return replay.FileMeta{}, err
	}
	defer f.Close()
	return replay.Inspect(f)
}

// session imports one session.
func (im *importer) session(id string, paths []string, metas []replay.FileMeta, res *importResult) (sessionOutcome, error) {
	out := sessionOutcome{SessionHMAC: im.ledger.ID(id)}
	order, dups := replay.OrderFiles(metas, id)
	res.Duplicates += len(dups)
	digests := map[string]bool{}
	for _, i := range append(append([]int(nil), order...), dups...) {
		digests[metas[i].Digest] = true
	}
	files := make([]string, 0, len(digests))
	for d := range digests {
		files = append(files, d)
	}
	sort.Strings(files)
	out.Files = len(files)

	// A session the live recorder recorded is the live index's: its record is
	// the better evidence and it has been counted.
	if im.cfg.LiveRoot != "" {
		live, err := store.RunExistsAt(im.cfg.LiveRoot, id)
		if err != nil {
			return out, err
		}
		if live {
			out.Status = statusLive
			out.Removed = im.ledger.Remove(id)
			return out, nil
		}
	}
	if !im.cfg.Rescan && im.ledger.Unchanged(id, files) {
		out.Status = statusUnchanged
		return out, nil
	}

	var srcs []replay.Source
	for _, i := range order {
		path := paths[i]
		srcs = append(srcs, replay.Source{Name: path, Main: isMainOf(metas[i], id),
			Open: func() (io.ReadSeekCloser, error) { return os.Open(path) }})
	}
	sum, err := im.replay(id, paths[order[0]], srcs)
	if err != nil {
		return out, err
	}
	sum.Files = files
	out.Replaced = im.ledger.Put(id, sum)
	out.Status = statusImported
	return out, nil
}

func isMainOf(m replay.FileMeta, id string) bool {
	_, main := m.Has(id)
	return main
}

// replay replays one session's merged files into a world of its own, removed
// afterwards, and summarises it.
func (im *importer) replay(id, transcriptPath string, srcs []replay.Source) (*sessionSummary, error) {
	dir, err := os.MkdirTemp(im.cfg.Scratch, "world-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	w, err := newReplayWorld(dir, im.cfg.Identity)
	if err != nil {
		return nil, err
	}
	calls := map[string]*callObs{}
	res := &replayResult{}
	var ms replay.MergeSummary
	var lastAt time.Time
	cfg := replayConfig{Ctx: im.cfg.Ctx, Observe: func(e replay.Event, fed bool) error {
		if !e.At.IsZero() {
			lastAt = e.At
		}
		if e.ToolUseID == "" {
			return nil
		}
		k := store.CapID(e.ToolUseID)
		o := calls[k]
		if o == nil {
			o = &callObs{}
			calls[k] = o
		}
		switch e.Kind {
		case replay.PreToolUse:
			o.preFed = o.preFed || fed
		case replay.PostToolUse, replay.PostToolUseFailure:
			o.postFed, o.postGrade, o.postTainted, o.postAt, o.file = fed, e.Grade, !e.Taint.IsZero(), e.At, e.File
			o.assumed = fed && len(e.Assumed) > 0
		}
		return nil
	}}
	err = w.replayEvents(res, cfg, func(yield func(replay.Event) error) error {
		var werr error
		ms, werr = replay.WalkSession(id, srcs, replay.MergeOptions{
			Options:    replay.Options{TranscriptPath: transcriptPath, Limits: im.cfg.Limits},
			MaxCallIDs: im.cfg.MaxCallIDs,
		}, yield)
		return werr
	})
	if err != nil {
		return nil, err
	}
	st, err := store.OpenExisting(w.root)
	if err != nil {
		if errors.Is(err, store.ErrNoStore) {
			return summarise(nil, id, res, ms, calls, lastAt), nil
		}
		return nil, err
	}
	run, err := st.ReadRun(id)
	if err != nil {
		return nil, err
	}
	return summarise(run, id, res, ms, calls, lastAt), nil
}

// findTranscripts lists the transcript files under root: regular files named
// *.jsonl, found by walking, in path order. A symbolic link is not followed
// and is not a file here, and the files beside a subagent's transcript
// (agent-<id>.meta.json) are not transcripts.
func findTranscripts(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && strings.HasSuffix(d.Name(), ".jsonl") {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}
