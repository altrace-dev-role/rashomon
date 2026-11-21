package hook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// The callers note's words. The read (readCallers) names a function and its
// callers by their graph ids; the note lists them by their qualified names,
// each an identifier slot inside its own quote. The steer line records the
// counts alone (the record rows), and the note's key holds a keyed digest of
// the ids it lists, so neither the line nor the key holds a name.

// maxCallersListed is the most callers a note lists; the rest are counted.
const maxCallersListed = 5

// callersDraft is the blast draft of a read that named callers (note or
// stale): the note at the blast-radius pattern's priority, a nudge with no
// stop form, after the call's landed seq, carrying the read's marker, so a
// note that does not print gives it back (settleBlast). False where no note
// can be said (callersMessage); the marker then stays, as that index's
// answer stands.
func (p *Post) callersDraft(r callersRead) (blastDraft, bool) {
	m, rec, listed, ok := callersMessage(r.Func, r.Callers, r.Case == callersStale)
	if !ok {
		return blastDraft{}, false
	}
	f := steer.Finding{Pattern: pattern.BlastRadius, Key: callersKey(p.st.Key(), r.Func, listed), MaxLevel: steer.LevelNudge,
		StopForm: steer.StopFormNone, Message: m, Record: rec, AfterSeq: p.landedSeq, AgentID: p.agentID}
	f.Priority, _ = pattern.Priority(pattern.BlastRadius)
	return blastDraft{f: f, marker: r.Marker}, true
}

// callersMessage is the note naming callers as calling fn, the stale rows
// where stale is set, its record (the counts alone), and the ids it lists.
// It lists up to maxCallersListed callers in the read's order; a caller
// whose id gives no name (symName) is left out and counted, as is every
// caller past the limit. While the note does not render (its text past
// render's message cap, among other refusals), it drops its last listed
// name into the count, and with one name left it says nothing. False where
// fn gives no name, where no caller gives one, or where the record does not
// render.
func callersMessage(fn graph.ID, callers []graph.ID, stale bool) (m, rec render.Message, listed []graph.ID, ok bool) {
	name, ok := symName(fn)
	if !ok {
		return render.Message{}, render.Message{}, nil, false
	}
	var names []slot.Ident
	more := 0
	for _, c := range callers {
		n, named := symName(c)
		switch {
		case !named:
			more++
		case len(names) == maxCallersListed:
			more++
		default:
			names, listed = append(names, n), append(listed, c)
		}
	}
	if len(names) == 0 {
		return render.Message{}, render.Message{}, nil, false
	}
	for {
		var err error
		if m, err = render.New(pattern.BlastRadius, callersNoteBinding(names, more, name, stale)); err == nil {
			break
		}
		if len(names) == 1 {
			return render.Message{}, render.Message{}, nil, false
		}
		names, listed, more = names[:len(names)-1], listed[:len(listed)-1], more+1
	}
	rec, err := render.New(pattern.BlastRadius, callersRecord(len(names), more, stale))
	if err != nil {
		return render.Message{}, render.Message{}, nil, false
	}
	return m, rec, listed, true
}

// callersNoteBinding is the note's binding: names listed and more counted, as
// calling fn, with the stale clause where stale is set.
func callersNoteBinding(names []slot.Ident, more int, fn slot.Ident, stale bool) any {
	l := slot.NewIdentList(names, more)
	if stale {
		return render.BlastCallersStaleBinding{Callers: l, Function: fn}
	}
	return render.BlastCallersBinding{Callers: l, Function: fn}
}

// callersRecord is the record's binding: the two counts alone.
func callersRecord(listed, more int, stale bool) any {
	if stale {
		return render.BlastCallersStaleRecordBinding{Listed: slot.Count(listed), More: slot.Count(more)}
	}
	return render.BlastCallersRecordBinding{Listed: slot.Count(listed), More: slot.Count(more)}
}

// symName is the qualified name a symbol's id ends with
// (sym:{lang}:{unit}:{symkind}:{qualified}, graph.SymID), false for an id
// of another kind and for a name the identifier slot refuses.
func symName(id graph.ID) (slot.Ident, bool) {
	parts := strings.SplitN(string(id), ":", 5)
	if len(parts) != 5 || parts[0] != "sym" {
		return "", false
	}
	n := slot.Ident(parts[4])
	return n, n.Validate() == nil
}

// callersKey is the note's key: "callers-" and 16 hex digits of an HMAC
// under the store's key over fn and the ids the note lists, one per line, so
// two notes are one claim only where they list the same callers of the same
// function, and the key holds no name.
func callersKey(key []byte, fn graph.ID, listed []graph.ID) string {
	m := hmac.New(sha256.New, key)
	b := []byte(fn)
	for _, id := range listed {
		b = append(append(b, '\n'), string(id)...)
	}
	m.Write(b)
	return "callers-" + hex.EncodeToString(m.Sum(nil))[:16]
}
