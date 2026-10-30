package main

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The ledger verb (card LEDGER-WIRE1, PF-PG5): `rashomon report --ledger
// --session <id>` prints the obligation ledger of the session's last Stop,
// replayed from the record (replayLastStop), so two derivations of one record
// print the same bytes. A header of "name: value" lines comes first and names
// what the replay read in place of what the record does not hold: the point
// and where Required came from, any gates the checkpoint's cut omitted, the
// spec (and the Stop's, where it differs), the task file outside spec_digest,
// the tree read now and the final message the record does not keep. Then one
// row to a line: id, scope, kind, source, state, the evidence seqs joined by
// commas ("-" for none) and the family the id digests (a declared gate's id
// for its rows; "-" where none of the view's gates or kinds gives it), one
// space apart; or, with no row, a line naming the point.

// reportLedger prints sessionID's ledger to stdout.
func reportLedger(sessionID string, stdout io.Writer) error {
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		err = errNoRecords
	}
	var r stopReplay
	if err == nil {
		r, err = replayLastStop(st, sessionID)
	}
	if errors.Is(err, errNoRecords) {
		return fmt.Errorf("session %q has no records here", sessionID)
	}
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, ledgerText(r))
	return err
}

// ledgerText is the verb's output for r: the header, then the rows.
func ledgerText(r stopReplay) string {
	var b strings.Builder
	v, cp, seq := r.View, r.Checkpoint, r.View.Snap.Point.Seq
	switch {
	case cp == nil:
		fmt.Fprintf(&b, "point: seq %d, the record's last; no checkpoint, so the Stop-time spec is unknown and required gates read the project index now\n", seq)
	case cp.Seq != nil:
		fmt.Fprintf(&b, "point: seq %d, required gates as recorded, from the checkpoint at seq %d\n", seq, *cp.Seq)
	default:
		fmt.Fprintf(&b, "point: seq %d, required gates as recorded, from the checkpoint\n", seq)
	}
	if cp != nil && cp.GatesOmitted > 0 {
		fmt.Fprintf(&b, "gates: %d cut from the checkpoint read required\n", cp.GatesOmitted)
	}
	now := specNow(r.Spec)
	if cp != nil && specText(cp.SpecDigest, cp.SpecTrust) != now {
		fmt.Fprintf(&b, "spec: %s; the Stop judged against %s; the rows read the spec now\n", now, specText(cp.SpecDigest, cp.SpecTrust))
	} else {
		fmt.Fprintf(&b, "spec: %s\n", now)
	}
	b.WriteString("task: outside spec_digest, so a change since the Stop is not named\n")
	b.WriteString("tree: read now, not at the Stop\n")
	b.WriteString("final message: not in the record, so judgment_disclosed reads unknown\n")
	if len(v.Obligations) == 0 {
		fmt.Fprintf(&b, "no obligation rows at seq %d\n", seq)
	}
	families := ledgerFamilies(v)
	for _, o := range v.Obligations {
		b.WriteString(ledgerRow(o, families[o.ID]))
	}
	return b.String()
}

// specText names a spec by digest and trust, "none" for no spec.
func specText(sum *string, trust *spec.Trust) string {
	if sum == nil {
		return "none"
	}
	if trust == nil {
		return *sum
	}
	return *sum + " (" + string(*trust) + ")"
}

// specNow is specText of the spec discovered now.
func specNow(sp *spec.Compiled) string {
	if sp == nil {
		return specText(nil, nil)
	}
	return specText(&sp.Digest, &sp.Trust)
}

// ledgerFamilies maps each row's id to the family ObligationID digested it
// over: a gate's Family, a declared gate's Gate id (every declared gate's
// Family is "declared", so its rows digest the id instead, DECLARED-GATES1),
// or an expected row's evidence kind. A row carries no such text (D-A1), so
// it is recomputed over the view's gates and kinds, and the string printed is
// whichever digests to the id. Cost: O(R x (2G + K)) digests, a verb's, never
// a hook's.
func ledgerFamilies(v verify.StopView) map[string]string {
	out := make(map[string]string, len(v.Obligations))
	for _, o := range v.Obligations {
		if o.Kind == verify.ExpectedEvidence {
			for _, e := range v.Expected {
				if verify.ObligationID(o.Scope, o.Kind, e.Kind) == o.ID {
					out[o.ID] = e.Kind
				}
			}
			continue
		}
	gates:
		for _, g := range v.Snap.Gates {
			for _, fam := range []string{g.Family, g.Gate} {
				if verify.ObligationID(o.Scope, o.Kind, fam) == o.ID {
					out[o.ID] = fam
					break gates
				}
			}
		}
	}
	return out
}

// ledgerRow is o as one line, family "" printed as "-".
func ledgerRow(o verify.Obligation, family string) string {
	ev := "-"
	if len(o.Evidence) > 0 {
		seqs := make([]string, len(o.Evidence))
		for i, s := range o.Evidence {
			seqs[i] = strconv.FormatInt(s, 10)
		}
		ev = strings.Join(seqs, ",")
	}
	if family == "" {
		family = "-"
	}
	return strings.Join([]string{o.ID, o.Scope, string(o.Kind), o.Source, string(o.State), ev, family}, " ") + "\n"
}
