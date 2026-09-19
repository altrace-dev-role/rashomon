// Package nono reads the audit trail another sandbox wrote.
//
// It is a FOURTH EVIDENCE SOURCE, beside the hook's declarations, the
// transcript, and the proxy's wire rows. Like the wire reader it is read-only
// over a file another program owns, and an absent or unreadable trail is a
// coverage statement rather than an error: a report that failed when nono was
// not running is a report nobody can use to learn that nono was not running.
//
// WHAT IT TAKES, AND NOTHING ELSE: a decision, a transport mode, a host, a
// port, a denial category and an instant. The shapes below were MEASURED
// against nono v0.78.0 on 2026-09-19 -- a real session, its real
// audit-events.ndjson -- and not read out of documentation, which described
// event types (`log_allowed`, `log_denied`) that the program does not emit.
//
// WHAT IT REFUSES TO TAKE is the more important half. nono's `session_started`
// event carries the FULL COMMAND LINE of the sandboxed process. That is
// content, and this program does not store content -- so the command is not
// read, not returned, and has no field here to be put in. A fourth evidence
// source that quietly became the first one to record a command would undo the
// property every other line of this codebase is arranged around, and it would
// do it while looking like an integration.
package nono

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/host"
)

// Reasons the trail could not be read. Fixed codes, never free text and never
// derived from a path, so a report line cannot carry a fragment of somebody's
// filesystem.
const (
	NotObservedNoTrail    = "no_nono_audit"
	NotObservedUnreadable = "nono_audit_unreadable"
	NotObservedNoWindow   = "run_recorded_no_window"
	// NotObservedNoRecords: the file opened and held no parseable record.
	// Separate from an absent file, because "nono wrote nothing here" and
	// "nono was never run" are different facts about the session.
	NotObservedNoRecords = "nono_audit_no_records"
)

// Decisions, as nono writes them.
const (
	DecisionAllow = "allow"
	DecisionDeny  = "deny"
)

// Event is one network decision, reduced to what a join needs.
type Event struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// Decision is allow or deny. Carried verbatim rather than as a bool: the
	// vocabulary is nono's, and a bool would silently absorb a third value if
	// one is ever added.
	Decision string `json:"decision"`
	// Mode is nono's transport: "connect" for a tunnel, "reverse" for a
	// request that went through its reverse proxy.
	//
	// It is kept because it is the one field that explains a DISAGREEMENT with
	// the wire. rashomon exports no HTTP_PROXY, so plain HTTP is outside what
	// its proxy can see -- and a reverse-mode event is exactly that traffic.
	// Without this field, "nono saw a host we did not" reads as a recording
	// failure instead of as a known boundary.
	Mode string `json:"mode"`
	// Category is nono's denial_category, empty on an allow. Measured values
	// include "host_denied" for both an allowlist miss and a deny-list hit;
	// the two are distinguished by nono's own reason string, which is NOT
	// carried here because it embeds the host and is free text.
	Category string `json:"category,omitempty"`
	At       time.Time
}

// Observation is what one session's window yields from the trail.
type Observation struct {
	Observed bool   `json:"observed"`
	Reason   string `json:"reason,omitempty"`
	Trail    string `json:"trail,omitempty"`
	// Events inside the window. Always an array, never null.
	Events []Event `json:"events"`
	// Inherited counts events outside it, reported rather than dropped for the
	// same reason the wire reader reports them: "the sandbox saw traffic that
	// was not this session's" is a fact, and discarding it makes a shared
	// audit directory look quiet.
	Inherited int `json:"inherited"`
	// Sessions counts session_started records seen, so a reader can tell one
	// sandbox session from several sharing a trail.
	//
	// NOT window-filtered, and it cannot be: the record carries an ISO string
	// and no timestamp_unix_ms, so there is nothing to filter on. That makes
	// it a lifetime count beside two windowed ones, which the renderer has to
	// say rather than joining all three in one sentence.
	Sessions int `json:"sessions"`
	// Skipped counts records this reader COULD NOT READ: a torn line, an
	// absent or unparseable event, an event type nono emits that this reader
	// has not learned. It does NOT count session_ended, which is known and
	// deliberately unused -- a counter that rose on every healthy trail would
	// be ignored by the time it mattered. UnparseableTargets
	// counts network events whose target the canonicaliser refused.
	//
	// Both exist because silence is not an answer. Without them a trail that
	// was three-quarters unreadable rendered identically to a quiet session.
	Skipped            int `json:"skipped"`
	UnparseableTargets int `json:"unparseable_targets"`
}

// Window is the watched interval. End zero means "up to the last event".
type Window struct {
	Start time.Time
	End   time.Time
}

// envelope is nono's per-record wrapper. Measured: every record carries both
// `event_json` (the canonical string the hash chain is computed over) and
// `event` (the same value, parsed).
//
// THE PARSED HALF IS READ, deliberately, and the choice is worth stating: the
// two are redundant by construction, and a record where they disagree is one
// whose chain does not verify -- which is nono's integrity property to enforce
// and not this reader's to relitigate. Reading `event` avoids a second JSON
// parse of a string this program would otherwise have to hold in memory whole.
type envelope struct {
	Sequence int             `json:"sequence"`
	Event    json.RawMessage `json:"event"`
}

type eventHead struct {
	Type string `json:"type"`
	// Nested, because a network event is `{"type":"network","event":{...}}`.
	Event networkEvent `json:"event"`
}

type networkEvent struct {
	TimestampUnixMS int64  `json:"timestamp_unix_ms"`
	Mode            string `json:"mode"`
	Decision        string `json:"decision"`
	DenialCategory  string `json:"denial_category"`
	Target          string `json:"target"`
	Port            int    `json:"port"`
}

// Read returns the network decisions nono recorded inside the window.
//
// path is one audit-events.ndjson. A directory of sessions is the caller's to
// choose from: this reader does not walk a tree, because deciding WHICH
// sandbox session belongs to a rashomon session is a join, and a reader that
// silently merged several would answer a question nobody asked.
func Read(path string, w Window) Observation {
	if path == "" {
		return Observation{Reason: NotObservedNoTrail, Events: []Event{}}
	}
	if w.Start.IsZero() {
		// No interval to read against, so nothing here can be attributed. The
		// wire reader learned this the hard way: with no window every row
		// counted as this session's.
		return Observation{Reason: NotObservedNoWindow, Trail: path, Events: []Event{}}
	}
	f, err := os.Open(path)
	if err != nil {
		reason := NotObservedUnreadable
		if errors.Is(err, fs.ErrNotExist) {
			reason = NotObservedNoTrail
		}
		return Observation{Reason: reason, Trail: path, Events: []Event{}}
	}
	defer func() { _ = f.Close() }()

	obs := Observation{Trail: path, Events: []Event{}}
	// Observed is set only once a record PARSES. Setting it on a successful
	// open made an empty, truncated or wrong-format file report as "the
	// sandbox watched and saw nothing" -- silence read as zero, which is the
	// failure this package's own doc says it exists to prevent. Three inputs
	// produced it: a zero-byte file, blank lines, and valid JSON of the wrong
	// shape.
	var parsed int
	sc := bufio.NewScanner(f)
	// A generous line cap: nono's session_started carries a whole command line,
	// and a scanner that stopped at the default 64KB would silently truncate
	// the trail at the first long one -- reading as "the sandbox saw nothing
	// after this point", which is the failure this package exists to avoid.
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var env envelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			// One malformed line is not a reason to discard the rest. A trail
			// is append-only and a torn final write is the ordinary case.
			// COUNTED, though: four separate paths used to drop a record with
			// no trace, so a file that was three-quarters unreadable reported
			// the same as a quiet session.
			obs.Skipped++
			continue
		}
		var head eventHead
		if err := json.Unmarshal(env.Event, &head); err != nil {
			obs.Skipped++
			continue
		}
		parsed++
		switch head.Type {
		case "session_started":
			// COUNTED, NEVER READ. This record carries the sandboxed command
			// line, and nothing below touches any field of it.
			obs.Sessions++
			continue
		case "session_ended":
			// KNOWN AND DELIBERATELY IGNORED, not skipped. It carries an exit
			// code and an ISO instant, neither of which this reader joins on.
			// Counting it as a drop would put a skipped-record line on every
			// healthy trail -- the crying-wolf failure H-28 exists to prevent,
			// and the reason Skipped must mean "a record I could not read"
			// rather than "a record I did not use".
			continue
		case "network":
			// fall through
		default:
			// A type nono has and this reader has not learned. Counted,
			// because schema drift is a measured property of this dependency:
			// its documentation named event types the program does not emit.
			obs.Skipped++
			continue
		}

		n := head.Event
		h, ok := host.Canonical(n.Target)
		if !ok {
			// The single most interesting record a sandbox can write -- a DENY
			// on a credential-bearing target -- was erased here without a
			// trace. Refusing to carry the string is right; refusing to carry
			// the count was not.
			obs.UnparseableTargets++
			// An unparseable target is dropped rather than guessed at. The
			// canonicaliser refuses anything carrying userinfo or a path, so
			// a credential in a target cannot reach a record from here.
			continue
		}
		at := time.UnixMilli(n.TimestampUnixMS)
		if at.Before(w.Start) || (!w.End.IsZero() && at.After(w.End)) {
			obs.Inherited++
			continue
		}
		obs.Events = append(obs.Events, Event{
			Host:     h,
			Port:     n.Port,
			Decision: n.Decision,
			Mode:     n.Mode,
			Category: n.DenialCategory,
			At:       at,
		})
	}
	if err := sc.Err(); err != nil {
		// A read that failed partway is not a complete observation, and saying
		// so is the whole contract of this package.
		return Observation{Reason: NotObservedUnreadable, Trail: path, Events: []Event{}}
	}

	if parsed == 0 {
		// Nothing in this file was a record. Distinct from an absent file and
		// from a readable-but-quiet one, and the reader has to be able to tell.
		return Observation{Reason: NotObservedNoRecords, Trail: path, Events: []Event{}}
	}
	obs.Observed = true

	sort.SliceStable(obs.Events, func(i, j int) bool {
		if obs.Events[i].At.Equal(obs.Events[j].At) {
			return obs.Events[i].Host < obs.Events[j].Host
		}
		return obs.Events[i].At.Before(obs.Events[j].At)
	})
	return obs
}

// Hosts returns the distinct hosts nono allowed, in order.
//
// Allowed only: a DENIED host is one the agent tried to reach and did not, so
// folding it in with the reached ones would report an attempt as a
// destination. The denials have their own accessor for the same reason.
func (o Observation) Hosts() []string {
	return o.distinct(DecisionAllow)
}

// Denied returns the distinct hosts nono refused.
func (o Observation) Denied() []string {
	return o.distinct(DecisionDeny)
}

func (o Observation) distinct(decision string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, e := range o.Events {
		if e.Decision != decision || seen[e.Host] {
			continue
		}
		seen[e.Host] = true
		out = append(out, e.Host)
	}
	sort.Strings(out)
	return out
}
