// Package gather is the report's read step. It opens what a report states
// beyond the store's records -- each session's transcripts and their subagent
// files, the sandbox trail, and the project baseline -- and hands what it read
// to report.Assemble, which decides and opens nothing.
//
// The split is what makes the turn digest's promise structural. digest links
// report for its verdict helpers and its transcript readers; it does not link
// this package, so nothing it links can open a session's transcript
// (TestRubricHLD4_TheVerdictPackagesLinkNoReadStep).
package gather

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/nono"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Option configures Build.
//
// Variadic options rather than a wider signature, so a caller that does not
// know about the nono trail keeps compiling and keeps getting an honest
// "not configured" rather than being forced to pass a path it has no opinion
// about.
type Option func(*options)

type options struct {
	nonoTrail string
}

// WithNonoTrail names the nono audit-events.ndjson the sandbox section is read
// from.
//
// An empty path means none was configured, which the report states as a
// coverage line rather than omitting -- a section that vanishes when it has
// nothing to say cannot be told apart from one that was never built.
func WithNonoTrail(path string) Option {
	return func(o *options) { o.nonoTrail = path }
}

// Build renders one session, or every session when sessionID is empty: it
// reads and assembles one session at a time, through report.AssembleSession,
// so each run's transcript, id sets and final text are garbage before the
// next session is read. A report over a long-lived store costs what
// rendering its biggest single session costs, not what holding every
// session's records at once would.
//
// It is not read-only. Asking the project baseline which hosts are new
// records this session's hosts in it (baseline.Update), which is what makes
// the next session's answer possible.
func Build(st *store.Store, sessionID string, now time.Time, opts ...Option) (*report.Report, error) {
	cfg, names, byDir, forgotten, forgottenBefore, err := plan(st, sessionID, opts)
	if err != nil {
		return nil, err
	}
	nonoConfigured := cfg.nonoTrail != ""
	rep := &report.Report{ReportVersion: report.CurrentReportVersion, GeneratedAtUnixMS: now.UnixMilli(), Sessions: make([]report.Session, 0, len(names))}
	for _, name := range names {
		si, err := readSession(st, name, byDir, cfg, forgottenBefore)
		if err != nil {
			return nil, err
		}
		rep.Sessions = append(rep.Sessions, report.AssembleSession(si, nonoConfigured, forgotten, forgottenBefore))
	}
	return rep, nil
}

// Read gathers what report.Assemble renders for one session, or for every
// session when sessionID is empty -- every run directory, and every run that
// eviction removed but a gap still names -- all held at once. Build does not
// use this: it is for a caller, such as a test, that wants the whole Input
// rather than a rendered Report.
func Read(st *store.Store, sessionID string, opts ...Option) (report.Input, error) {
	cfg, names, byDir, forgotten, forgottenBefore, err := plan(st, sessionID, opts)
	if err != nil {
		return report.Input{}, err
	}
	in := report.Input{NonoConfigured: cfg.nonoTrail != "", Forgotten: forgotten, ForgottenBefore: forgottenBefore}
	for _, name := range names {
		si, err := readSession(st, name, byDir, cfg, forgottenBefore)
		if err != nil {
			return report.Input{}, err
		}
		in.Sessions = append(in.Sessions, si)
	}
	return in, nil
}

// plan is what Build and Read share before either reads a session: which run
// directories to render, the gaps each one carries, and the store-wide
// forgotten-host answers.
func plan(st *store.Store, sessionID string, opts []Option) (cfg options, names []string, byDir map[string][]store.Gap, forgotten func(string) bool, forgottenBefore func(string, int64) bool, err error) {
	for _, o := range opts {
		o(&cfg)
	}
	gaps, err := st.ReadGaps()
	if err != nil {
		return options{}, nil, nil, nil, nil, err
	}
	// Gaps are keyed by run directory, the one name a session has whether or
	// not its records still exist and whatever characters its id contains.
	byDir = map[string][]store.Gap{}
	for _, g := range gaps {
		dir := st.DirName(g.SessionID)
		byDir[dir] = append(byDir[dir], g)
	}
	names, err = runNames(st, sessionID, byDir)
	if err != nil {
		return options{}, nil, nil, nil, nil, err
	}
	// Read once for the whole report: the forgotten set is a property of the
	// store, not of a session, and re-reading the gaps per session would be the
	// same answer at more cost.
	//
	// Two predicates, because the two readers need different questions
	// answered. The sandbox's trail is nono's file, not this store's: every
	// one of its events stays exactly as readable as the day it was written,
	// so buildNono asks only "is this host forgotten" and never returns one,
	// at any time, under any session. A declaration is this store's own
	// record, and forget --host only ever meant to remove the ones that
	// predate it; buildDestinations, buildFamilies and buildChains ask
	// "forgotten before THIS declaration was recorded", so a later, genuine
	// re-use of a once-forgotten name still shows up as what it is.
	forgotten, err = st.ForgottenHost()
	if err != nil {
		return options{}, nil, nil, nil, nil, err
	}
	forgottenBefore, err = st.ForgottenHostBefore()
	if err != nil {
		return options{}, nil, nil, nil, nil, err
	}
	return cfg, names, byDir, forgotten, forgottenBefore, nil
}

// readSession reads one run directory and what report.AssembleSession needs
// beside it. Nothing it returns is kept beyond the one Report.Session or
// Input.Sessions entry built from it, so a caller that reads one session at a
// time (Build) never holds a second session's records while it holds this
// one's.
func readSession(st *store.Store, name string, byDir map[string][]store.Gap, cfg options, forgottenBefore func(string, int64) bool) (report.SessionInput, error) {
	run, err := st.ReadRunDir(name)
	if err != nil {
		return report.SessionInput{}, err
	}
	return report.SessionInput{
		Dir:         name,
		Run:         run,
		Gaps:        byDir[name],
		Transcripts: readTranscripts(run),
		Novelty:     novelty(run, st.Root(), forgottenBefore),
		Nono:        readNono(cfg.nonoTrail, report.SessionWindow(run)),
	}, nil
}

// readNono opens the sandbox trail path names, if one was, and parses it
// over w. This is the only place that opens a nono trail: nono.Observe
// takes an already-open reader, so a package that links internal/nono only
// for its types -- report does, for Observation; digest does not link it at
// all -- has no path from that import to a trail's bytes
// (TestRubricHLD4_TheVerdictPackagesLinkNoReadStep).
func readNono(path string, w nono.Window) nono.Observation {
	if path == "" {
		return nono.Observation{Reason: nono.NotObservedNoTrail, Events: []nono.Event{}}
	}
	f, err := os.Open(path)
	if err != nil {
		reason := nono.NotObservedUnreadable
		if errors.Is(err, fs.ErrNotExist) {
			reason = nono.NotObservedNoTrail
		}
		return nono.Observation{Reason: reason, Trail: path, Events: []nono.Event{}}
	}
	defer f.Close() //nolint:errcheck // read-only
	return nono.Observe(f, path, w)
}

// runNames are the run directories to render: the one sessionID names, or
// every run directory and then, sorted, every one a gap names whose directory
// eviction removed -- a report that could not show it would be hiding a
// deletion.
func runNames(st *store.Store, sessionID string, byDir map[string][]store.Gap) ([]string, error) {
	if sessionID != "" {
		return []string{st.DirName(sessionID)}, nil
	}
	names, err := st.Runs()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
	}
	var evicted []string
	for dir := range byDir {
		if !seen[dir] {
			evicted = append(evicted, dir)
		}
	}
	sort.Strings(evicted)
	return append(names, evicted...), nil
}

// novelty asks the project baseline beside the store which of the run's hosts
// are new to its project, and records them there. forgotten is the
// per-declaration predicate the destinations apply (report.NoveltyQueryOf).
func novelty(run *store.Run, storeRoot string, forgotten func(host string, declaredAtMS int64) bool) report.Novelty {
	q, unavailable := report.NoveltyQueryOf(run, storeRoot, forgotten)
	if unavailable != nil {
		return *unavailable
	}
	res := baseline.Update(storeRoot, baseline.Key(q.CWD), q.SessionID, q.Start, q.Hosts)
	return report.AnsweredNovelty(res.Established, res.Known, res.Novel, res.Err)
}

// readTranscripts reads every transcript path the run's declarations name:
// the ids of each one, and the final assistant message of the first declared
// path, in declaration order, that has a readable one -- the only path
// buildAccount (internal/report/account.go) ever uses. Reading every path's
// final text, as readTranscript does for one path on its own, would cost a
// full extra scan of each transcript a session's own account does not use.
func readTranscripts(run *store.Run) map[string]report.TranscriptRead {
	paths := report.TranscriptPaths(run)
	out := make(map[string]report.TranscriptRead, len(paths))
	for _, p := range paths {
		var read report.TranscriptRead
		read.Set, read.Err = readIDSet(p)
		out[p] = read
	}
	readFinal(run, out)
	return out
}

// readFinal fills the final assistant message of the first declared
// transcript, in declaration order, that has a readable one, mirroring
// buildAccount's own selection rule so the two agree about which path's
// account the report renders. It stops at the first success, so no path
// after it in declaration order is opened for its final text.
func readFinal(run *store.Run, out map[string]report.TranscriptRead) {
	tried := map[string]bool{}
	for _, d := range run.Declarations {
		if d.TranscriptPath == "" || tried[d.TranscriptPath] {
			continue
		}
		tried[d.TranscriptPath] = true
		text, ok := finalText(d.TranscriptPath)
		if !ok {
			continue
		}
		read := out[d.TranscriptPath]
		read.Final, read.FinalRead = text, true
		out[d.TranscriptPath] = read
		return
	}
}

// readTranscript reads one session transcript: the ids of it and of its
// subagent transcripts, which live under <session>/subagents where <session>
// is the transcript path without its extension, and the final assistant
// message of the transcript itself. The two are read apart, so a subagent
// file that cannot be read costs the accounting and not the account.
func readTranscript(path string) report.TranscriptRead {
	var read report.TranscriptRead
	read.Final, read.FinalRead = finalText(path)
	read.Set, read.Err = readIDSet(path)
	return read
}

// readIDSet reads the ids of a transcript and of every subagent transcript
// beside it into one set. Any error is the whole set's: a count over some of
// a session's files is not the session's count.
func readIDSet(path string) (*report.TranscriptSet, error) {
	set := report.NewTranscriptSet()
	if err := readIDs(path, set); err != nil {
		return nil, err
	}
	subs, err := subagentTranscripts(filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents"))
	if err != nil {
		return nil, err
	}
	for _, s := range subs {
		if err := readIDs(s, set); err != nil {
			return nil, err
		}
	}
	return set, nil
}

// readIDs opens one transcript file and reads its ids into set.
func readIDs(path string, set *report.TranscriptSet) error {
	return withFile(path, func(r io.Reader) error { return report.TranscriptIDs(r, set) })
}

// finalText is a transcript's final assistant message. A file that cannot be
// opened has none.
func finalText(path string) (text string, ok bool) {
	err := withFile(path, func(r io.Reader) error {
		text, ok = report.FinalAssistantText(r)
		return nil
	})
	if err != nil {
		return "", false
	}
	return text, ok
}

// withFile opens path read-only for the length of read.
func withFile(path string, read func(io.Reader) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // read-only
	return read(f)
}

// subagentTranscripts lists every agent-*.jsonl under dir, at any depth.
//
// A plain subagent writes <session>/subagents/agent-*.jsonl, but a workflow's
// subagents write <session>/subagents/workflows/<run>/agent-*.jsonl, two levels
// further down. A one-level glob read 35 of a real session's 407 transcripts,
// and every call the other 372 made then read as recorded-but-not-in-the-
// transcript -- a gap that was this function's, not the recorder's. A missing
// directory is the common case, a session with no subagents, and not an error.
func subagentTranscripts(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == dir {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ok, _ := filepath.Match("agent-*.jsonl", d.Name()); ok {
			out = append(out, p)
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	sort.Strings(out)
	return out, err
}
