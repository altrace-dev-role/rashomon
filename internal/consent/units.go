package consent

import "fmt"

// Unit is one thing a person can agree to separately from installing: a
// closed set, so a unit name read from a file or typed on a command line is
// parsed into this type or refused (Task 4.2).
type Unit string

// The closed set of consent units. Installing grants UnitRecord and nothing
// else; every other unit is off until `rashomon consent <unit>`.
const (
	UnitRecord        Unit = "record"
	UnitBodies        Unit = "bodies"
	UnitShareOutcomes Unit = "share.outcomes"
	UnitShareLessons  Unit = "share.lessons"
	UnitShareBodies   Unit = "share.bodies"
)

// Units is every Unit, in the order they are documented.
var Units = []Unit{UnitRecord, UnitBodies, UnitShareOutcomes, UnitShareLessons, UnitShareBodies}

// ParseUnit returns the Unit s names. Matching is exact: no case folding, no
// trimming.
func ParseUnit(s string) (Unit, bool) {
	for _, u := range Units {
		if string(u) == s {
			return u, true
		}
	}
	return "", false
}

// valid reports whether u is in Units.
func (u Unit) valid() bool {
	_, ok := ParseUnit(string(u))
	return ok
}

// grantable reports whether a unit_tty record may name u: any unit but
// record, which only the install screen grants.
func (u Unit) grantable() bool { return u.valid() && u != UnitRecord }

// Has reports whether root's consent.json grants u. record is granted by any
// act of consent that showed the install screen (LastAccepted), so a
// consent.json written before units existed reads as record granted. Every
// other unit needs a unit_tty record naming it, and sits under record: a
// grant with no install beneath it grants nothing. An unreadable file grants
// nothing and the error says why, as do the other readers; so does a name
// outside Units. Grants are append-only, so Has only turns true as records
// are added (until purge removes the file). Time is linear in the file, which
// is capped.
func Has(root string, u Unit) (bool, error) {
	if !u.valid() {
		return false, fmt.Errorf("consent: %q is not a consent unit", string(u))
	}
	recs, err := Read(root)
	if err != nil {
		return false, err
	}
	return holds(recs, u), nil
}

// holds is Has over records already read.
func holds(recs []Record, u Unit) bool {
	if _, ok := LastAccepted(recs); !ok {
		return false
	}
	if u == UnitRecord {
		return true
	}
	for _, r := range recs {
		if r.Via == ViaUnitTTY && r.Unit == u {
			return true
		}
	}
	return false
}

// notices are the first-run notices, one per unit but record. Each is shown
// before the question for its unit and is the text whose hash the grant
// record carries, so a change to a notice is a change to what a later record
// attests to. Lines stay inside Width. A unit whose feature is not in this
// release says so: agreeing records the choice and enables nothing.
var notices = map[Unit]string{
	UnitBodies: `bodies: off until you agree.

Keeping the text of what was said and run, beside the record of what happened.
Nothing in this release reads this choice; agreeing records it for a release
that does.

Agree with: rashomon consent bodies
`,
	UnitShareOutcomes: `share.outcomes: off until you agree.

Lets "rashomon share build outcomes" build a file of counts: how often each
named pattern fired and what became of it, with no text from your sessions in
it. The command prints the whole file and its SHA-256 before it writes it.
rashomon sends nothing anywhere; what you do with the file is up to you.

Agree with: rashomon consent share.outcomes
`,
	UnitShareLessons: `share.lessons: off until you agree.

Sharing lessons rashomon has drawn from your sessions. Nothing in this release
reads this choice; agreeing records it for a release that does.

Agree with: rashomon consent share.lessons
`,
	UnitShareBodies: `share.bodies: off until you agree.

Sharing the text kept under "bodies". Nothing in this release reads this
choice; agreeing records it for a release that does.

Agree with: rashomon consent share.bodies
`,
}

// Notice is the text shown before u is agreed to. "" for UnitRecord, which
// the install screen covers, and for anything outside Units.
func Notice(u Unit) string { return notices[u] }
