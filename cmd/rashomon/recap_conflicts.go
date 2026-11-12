package main

import (
	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// conflictsSeen is the conflicts the session was shown, for the Stop's
// StopInput.Conflicts (W-M2.4): the run's conflict_check records, folded
// (foldConflictChecks). A variable so the Stop's own tests can set it.
var conflictsSeen = foldConflictChecks

// foldConflictChecks folds run's conflict_check records at or below seq, in
// seq order, into the conflicts the session was shown (card m2-seam1 D5).
// A conflict's identity is its key and the commit it was checked at; its
// row is the one it was first raised with. Seqs gain a check's landed seq
// when the conflict is first raised and when it is raised again after it
// resolved. A check of file F at commit c clears F from each conflict at c
// that F showed and the check no longer holds, and a conflict no file still
// shows is resolved at that check's seq. A check at another commit
// resolves nothing (ruling 02:11 item 4): after a re-index the conflict may
// be in the base, which is not a resolution, so it stays unmet. Cost:
// O(checks x conflicts).
func foldConflictChecks(run *store.Run, seq int64) []verify.ConflictSeen {
	if run == nil {
		return nil
	}
	type id struct{ key, commit string }
	type state struct {
		seen  verify.ConflictSeen
		files map[string]bool
	}
	var order []id
	byID := map[id]*state{}
	for _, rec := range run.ConflictChecks {
		if seq > 0 && rec.Seq != nil && *rec.Seq > seq {
			continue
		}
		here := map[string]bool{}
		for _, r := range rec.Rows {
			k := id{r.Key, rec.Commit}
			here[r.Key] = true
			s := byID[k]
			if s == nil {
				s = &state{seen: verify.ConflictSeen{Row: verify.ConflictRow{Kind: r.Kind, Unit: r.Unit, UnitPath: r.UnitPath,
					RuleID: r.RuleID, Subject: r.Subject}}, files: map[string]bool{}}
				byID[k] = s
				order = append(order, k)
			}
			if len(s.seen.Seqs) == 0 || s.seen.ResolvedSeq != 0 {
				s.seen.Seqs = append(s.seen.Seqs, rec.LandedSeq)
				s.seen.ResolvedSeq = 0
			}
			s.files[rec.File] = true
		}
		for _, k := range order {
			if s := byID[k]; k.commit == rec.Commit && !here[k.key] && s.files[rec.File] {
				delete(s.files, rec.File)
				if len(s.files) == 0 {
					s.seen.ResolvedSeq = rec.LandedSeq
				}
			}
		}
	}
	if len(order) == 0 {
		return nil
	}
	out := make([]verify.ConflictSeen, 0, len(order))
	for _, k := range order {
		out = append(out, byID[k].seen)
	}
	return out
}

// stopLineWith is recap.LineWith over the Stop's rule and platform lines, and,
// when the Stop speaks and a conflict is unresolved, a second composition with
// the conflict lines added (W-M2.4). The conflict lines come last and take
// what the message leaves under Claude Code's cap, so they are composed
// against the message as the first pass left it (B2). With no conflict there
// is no second pass: the common Stop pays nothing. It is separate from
// recapTurn to keep that function within Law 10.
func stopLineWith(d *digest.Digest, stop stopTurn, fromPlugin bool) (string, bool) {
	lines := stop.rules() + recap.PlatformLines(stop.view, fromPlugin)
	line, wantSpeak := recap.LineWith(d, d.SessionID, fromPlugin, lines)
	if !wantSpeak || !stop.speaks {
		return line, wantSpeak
	}
	if cl := recap.ConflictLines(stop.view, fromPlugin, line); cl != "" {
		line, _ = recap.LineWith(d, d.SessionID, fromPlugin, lines+cl)
	}
	return line, wantSpeak
}
