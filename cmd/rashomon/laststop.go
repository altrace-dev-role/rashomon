package main

import (
	"errors"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/report/gather"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// A replay evaluates a session's Stop view again at its last Stop point,
// from the record (card LEDGER-WIRE1): the ledger verb prints it, and the
// session report's checks read it. The point is the last checkpoint's, its
// point_seq, prompt and recorded time, with each gate's Required pinned to
// what the checkpoint recorded, the Stop-time truth the project index may
// since have moved past; with no readable checkpoint it is the record's last
// seq, the clock now and the index on disk.
//
// The record does not hold everything the Stop read, so a replay reads each
// missing input the way that can make a row stricter than the Stop's, never
// more favourable, and carries what it read for the caller to name:
//   - the spec is discovered from the record's project root, as the Stop
//     discovers it from its cwd (hook.DiscoverSpec), and trusted by the
//     store's bindings as the Stop's is, so a trusted spec's declared gates
//     stay; the task file it carries is outside the checkpoint's spec_digest;
//   - the tree is the files' content now (hook.TreeShas, as the Stop reads
//     it), so a file changed since the Stop reads stale;
//   - the inferred test gate (GATE-INFER1) is read from the project's files
//     now, and only where the checkpoint names it, so a CI file changed
//     since names the command it holds now, and with no index read its line
//     keeps the "(inferred)" label the Stop may have dropped;
//   - the pgraph log, its digests and the written Go files are read now
//     (platformRead), so a sync or an edit since the Stop moves the
//     platform rows' families;
//   - the final message is not in the record, so what is read from it is
//     unknown (StopInput.MessageUnread);
//   - a gate the checkpoint's cut omitted (GatesOmitted, greens first) has no
//     recorded Required and keeps the fail-closed prior, required.
//
// A replay writes nothing: no checkpoint and no index.

// errNoRecords is a replay of a session the store holds no record of.
var errNoRecords = errors.New("the session has no records here")

// stopReplay is a replayed Stop view and what it was read from. Checkpoint
// is nil when none was readable; Spec is the spec discovered now, nil for
// none.
type stopReplay struct {
	View       verify.StopView
	Checkpoint *verify.Checkpoint
	Spec       *spec.Compiled
}

// replayLastStop replays sessionID's last Stop from st. A session with no
// records in st is errNoRecords, never an empty view.
func replayLastStop(st *store.Store, sessionID string) (stopReplay, error) {
	run, err := digest.ReadRun(st, sessionID)
	if err != nil {
		return stopReplay{}, err
	}
	if run.LastSeq() == 0 {
		return stopReplay{}, errNoRecords
	}
	cp, err := gather.LastStop(st, sessionID)
	if err != nil {
		return stopReplay{}, err
	}
	pt := verify.Point{Kind: verify.PointStop, Seq: run.LastSeq(), SessionID: sessionID}
	now := time.Now()
	in := verify.StopInput{Key: st.Key(), MessageUnread: true}
	in.Platform = platformRead
	if cp != nil {
		pt.Seq, pt.PromptID, now = cp.PointSeq, cp.PromptID, time.UnixMilli(cp.RecordedAtUnixMS)
		if cp.PromptID != nil {
			in.PromptID = *cp.PromptID
		}
		in.Required = make(map[string]bool, len(cp.Gates))
		for _, g := range cp.Gates {
			in.Required[g.Gate] = g.Required
		}
	}
	pt.Tree = hook.TreeShas(verify.TreeTargets(run))
	r := stopReplay{Checkpoint: cp}
	var idx *verify.ProjectIndex
	if root := verify.ProjectRoot(run, pt.Seq); root != "" {
		if key := baseline.Key(root); cp == nil && key != "" {
			idx, _ = verify.ReadIndex(verify.IndexPath(st.Root(), key))
		}
		projectDir, managedDir := specDirs()
		r.Spec = hook.DiscoverSpec(root, projectDir, managedDir, hook.TrustRoot(run, st.Root(), pt.Seq))
		in.Inferred = replayInferred(root, r.Spec, idx, in.Required)
	}
	r.View = verify.EvaluateStop(run, idx, r.Spec, pt, now, in)
	return r, nil
}
