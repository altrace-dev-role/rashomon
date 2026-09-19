// Package wire reads the destinations the proxy observed.
//
// This is the only part of the product that sees the network. Everything else
// records what the client SAID; this records where the traffic went, and the
// difference between the two is the whole finding. It is a read-only reader of
// a database another program owns: the proxy writes causal.db, this never
// writes to it, and an absent or unreadable file is a coverage statement
// ("destinations: not observed, with the reason") rather than an error, because
// a report that fails when the proxy was not running is a report nobody can use
// to learn that the proxy was not running.
package wire

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver; see the note in Read about H-17

	"github.com/altrace-dev-role/rashomon/internal/host"
)

// Reasons the proxy's store could not be read. Fixed codes, never free text,
// and never derived from the path, so a report line cannot carry a fragment of
// someone's filesystem.
const (
	NotObservedNoStore     = "no_proxy_store"
	NotObservedUnreadable  = "proxy_store_unreadable"
	NotObservedNoTable     = "proxy_store_has_no_records_table"
	NotObservedQueryFailed = "proxy_store_query_failed"

	// NotObservedNoWindow is the run's own gap rather than the store's: the run
	// recorded no coverage, so there is no interval to read the store against.
	//
	// It is separate from a parse failure because the consequence is opposite.
	// With an unusable clock the rows still belong to this session and are
	// reported with the window flag down. With no window at all, NOTHING can be
	// attributed -- and treating that as "every row is in window" made an
	// evicted run render as a phantom session whose reached-but-never-named
	// list was the entire proxy database.
	NotObservedNoWindow = "run_recorded_no_window"
)

// Destination is one host the proxy saw, with how often.
//
// Attempts and Distinct are separate because they answer different questions
// and one number cannot serve both: four CONNECTs to api.anthropic.com in three
// seconds is one destination and four attempts, and a report that prints only
// the larger number makes a retrying client look like a busy agent.
type Destination struct {
	Host string `json:"host"`
	// Attempts counts only rows inside the window. InheritedAttempts counts the
	// rest, separately, because a host can have both: the same destination
	// reached by this session and by an earlier one through the same proxy.
	// Folding them into one number would put another session's traffic into
	// this session's headline count, and a host that appears in both is
	// precisely the case where nobody would notice.
	Attempts          int      `json:"attempts"`
	InheritedAttempts int      `json:"inherited_attempts"`
	Actions           []string `json:"actions"`
	Reasons           []string `json:"reasons"`
	FirstSeen         string   `json:"first_seen"`
	LastSeen          string   `json:"last_seen"`
	// Inherited marks a row from outside the watched window, or one carrying
	// another session's run id. Such rows are excluded from accounting rather
	// than dropped: "the proxy saw traffic that was not this session's" is a
	// fact the report has to be able to state, and silently discarding it would
	// make a shared proxy look quiet.
	Inherited bool `json:"inherited"`
	// Unreached is true when every row for this host ended in a dial failure.
	// "Allowed but never reached" and "reached" are different facts, and a
	// report that cannot tell them apart will assert a host was contacted when
	// the connection was refused.
	Unreached bool `json:"unreached"`
	// InWindowReached and InWindowFailed answer the same question as Unreached
	// -- did a connection actually happen -- but about THIS SESSION rather than
	// about the host, and Unreached cannot be reused for it: any row that is
	// not a dial failure clears it, including a row from outside the window. So
	// a host reached yesterday and refused today reads as reached, which is the
	// right answer for the destinations section and the wrong one for a link in
	// a chain.
	//
	// They count FOLDED REQUESTS, not rows, for the same reason Attempts does:
	// one attempt writes up to two rows and counting rows reports a single
	// refused attempt as two.
	InWindowReached int `json:"in_window_reached"`
	InWindowFailed  int `json:"in_window_failed"`
}

// Observation is what one session's window yields.
type Observation struct {
	// Observed is false when the store could not be read at all. The report
	// prints "destinations: not observed" plus Reason, and never an empty list,
	// because an empty list is the answer to a different question.
	Observed bool          `json:"observed"`
	Reason   string        `json:"reason,omitempty"`
	Store    string        `json:"store,omitempty"`
	Hosts    []Destination `json:"hosts"`
	// Attempts and DistinctHosts are the totals over non-inherited rows.
	Attempts      int `json:"attempts"`
	DistinctHosts int `json:"distinct_hosts"`
	Inherited     int `json:"inherited"`
	// WindowApplied is false when the store's timestamps could not be parsed,
	// in which case every row is reported and the report must say the window
	// was not applied. Claiming a window that was not enforced would attribute
	// another session's destinations to this one.
	//
	// It does NOT also mean "the run recorded no window": that is
	// NotObservedNoWindow, with Observed false. The two used to share this flag
	// and the single message blamed the timestamps, which in that case had
	// parsed perfectly.
	WindowApplied bool `json:"window_applied"`
}

// Window is the watched interval. End is zero when the session has not ended,
// which means "up to the last row".
type Window struct {
	RunID string
	Start time.Time
	End   time.Time
}

// row is one causal record, reduced to the columns this reader uses.
type row struct {
	seq       int64
	requestID string
	runID     string
	when      time.Time
	whenOK    bool
	action    string
	reason    string
	host      string
}

// dialFailedPrefix is the proxy's reason prefix for a destination that was
// permitted and could not be reached. Matched as a prefix because the suffix is
// the proxy's bounded dial-error vocabulary and this reader must not have to
// track additions to it.
const dialFailedPrefix = "dial_failed_"

// Read returns the destinations the proxy observed inside the window.
//
// The driver is modernc.org/sqlite, pure Go so the release stays cgo-free. Its
// dependency graph transitively contains net and os/exec, which is why H-17's
// structural assertion is scoped to the recorder packages rather than to the
// whole binary: the recorder still has no socket path, proven by the syscall
// trace, and this reader opens a file rather than a socket. The connection
// string is mode=ro so a reader bug cannot corrupt an audit store the operator
// may need as evidence.
func Read(path string, w Window) Observation {
	if path == "" {
		return Observation{Reason: NotObservedNoStore}
	}
	if _, err := os.Stat(path); err != nil {
		reason := NotObservedUnreadable
		if errors.Is(err, fs.ErrNotExist) {
			reason = NotObservedNoStore
		}
		return Observation{Reason: reason, Store: path}
	}

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return Observation{Reason: NotObservedUnreadable, Store: path}
	}
	defer func() { _ = db.Close() }()

	rows, err := query(db)
	if err != nil {
		return Observation{Reason: classify(err), Store: path}
	}
	return summarise(rows, w, path)
}

func query(db *sql.DB) ([]row, error) {
	// Ordered by sequence_num, which is the store's own total order and the
	// only field guaranteed monotonic. The timestamp column is not usable for
	// ordering: the proxy writes it as Go's time.Time.String(), monotonic
	// suffix included, so it does not sort correctly as text.
	const q = `SELECT sequence_num, request_id, run_id, timestamp, action, reason, target_host
	           FROM causal_records
	           WHERE target_host <> ''
	           ORDER BY sequence_num`
	rs, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rs.Close() }()

	var out []row
	for rs.Next() {
		var (
			r  row
			ts string
		)
		if err := rs.Scan(&r.seq, &r.requestID, &r.runID, &ts, &r.action, &r.reason, &r.host); err != nil {
			return nil, err
		}
		r.when, r.whenOK = parseStamp(ts)
		out = append(out, r)
	}
	// rows.Err is checked because a scan loop that ends early on a read error
	// otherwise returns a short list as if it were the whole store (CWE-252).
	if err := rs.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// classify maps a query error onto a fixed reason code. The error text is not
// carried into the report: it can contain the path, and the path can contain a
// user's directory names.
func classify(err error) string {
	if strings.Contains(err.Error(), "no such table") {
		return NotObservedNoTable
	}
	return NotObservedQueryFailed
}

// parseStamp reads the proxy's timestamp column.
//
// The proxy binds a time.Time to a DATETIME column, and the value lands as
// Go's default String() rendering WITH the monotonic-clock suffix:
//
//	2026-09-18 15:21:23.565097 -0400 EDT m=+7.991661334
//
// That is not RFC 3339 and no standard layout parses it, so the suffix is cut
// and the Go layout tried first. RFC 3339 is tried second so this reader keeps
// working if the proxy is ever fixed to write it, which is the change that
// should happen: as stored, the column cannot be compared or sorted as text by
// any other tool.
//
// A stamp that parses under neither is reported as unusable rather than
// defaulted to the zero time, because the zero time is before every window and
// would silently mark the row inherited -- dropping a real destination out of
// the report.
func parseStamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, " m=+"); i > 0 {
		s = s[:i]
	}
	if i := strings.Index(s, " m=-"); i > 0 {
		s = s[:i]
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999 -0700",
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// summarise folds rows into destinations.
func summarise(rows []row, w Window, path string) Observation {
	if w.Start.IsZero() {
		// No interval to read the store against, so nothing here can be
		// attributed to this run. Reporting the rows anyway made every one of
		// them count as this session's: with no declarations to match, every
		// host became "reached but never named" and proxy-on-path became true.
		// Reachable through the shipped path, because report renders evicted run
		// directories and an evicted run has no coverage records.
		return Observation{Store: path, Reason: NotObservedNoWindow, Hosts: []Destination{}}
	}
	obs := Observation{Observed: true, Store: path, WindowApplied: true}
	if !anyParsed(rows) {
		// Without a usable clock the window cannot be enforced. Every row is
		// reported and the flag says the window was not applied; pretending
		// otherwise would attribute another session's destinations to this one.
		obs.WindowApplied = false
	}

	// One request produces up to two rows: the chain's verdict and, when the
	// destination could not be reached, a dial outcome. They are folded by
	// request_id so a single attempt is counted once, with the outcome
	// attached -- counting both would double every failed dial.
	byRequest := map[string]*row{}
	outcome := map[string]string{}
	// hostOutcome is the fallback for a dial outcome that carries no request id
	// to be folded onto. Without it such a row became its own attempt and then
	// missed the outcome lookup, so the host read as REACHED when its
	// connection had been refused -- the one distinction this field exists to
	// make, wrong in the direction that overstates what was observed.
	hostOutcome := map[string]string{}
	var order []string
	for i := range rows {
		r := rows[i]
		if strings.HasPrefix(r.reason, dialFailedPrefix) {
			if r.requestID != "" {
				outcome[r.requestID] = r.reason
				continue
			}
			if h, ok := host.Canonical(r.host); ok {
				hostOutcome[h] = r.reason
				continue
			}
		}
		key := r.requestID
		if key == "" {
			// No request id: treat the row as its own attempt rather than
			// folding every such row together, which would under-count.
			key = fmt.Sprintf("seq-%d", r.seq)
		}
		if _, seen := byRequest[key]; !seen {
			order = append(order, key)
			c := r
			byRequest[key] = &c
		}
	}

	agg := map[string]*Destination{}
	var hostOrder []string
	for _, key := range order {
		r := byRequest[key]
		h, ok := host.Canonical(r.host)
		if !ok {
			continue
		}
		inherited := isInherited(*r, w, obs.WindowApplied)

		d, exists := agg[h]
		if !exists {
			d = &Destination{Host: h, Unreached: true}
			agg[h] = d
			hostOrder = append(hostOrder, h)
		}
		// Counted on the side the row belongs to. A host is INHERITED only when
		// every row for it is out of window; one in-window row makes the
		// destination this session's, but it does not make the out-of-window
		// rows this session's attempts.
		if inherited {
			d.InheritedAttempts++
		} else {
			d.Attempts++
		}
		d.Actions = addOnce(d.Actions, r.action)
		d.Reasons = addOnce(d.Reasons, r.reason)
		reached := false
		switch o, failed := outcome[key]; {
		case failed:
			d.Reasons = addOnce(d.Reasons, o)
		case hostOutcome[h] != "":
			d.Reasons = addOnce(d.Reasons, hostOutcome[h])
		default:
			d.Unreached = false
			reached = true
		}
		// Derived from the same branch that clears Unreached rather than from a
		// second test of the same condition. The two answer one question at
		// different scopes, and a chain link saying "reached" above a
		// destination line saying "never reached" is exactly what a reader
		// cannot resolve. One predicate, read twice.
		if !inherited {
			if reached {
				d.InWindowReached++
			} else {
				d.InWindowFailed++
			}
		}
		// Timestamps from IN-WINDOW rows only. The attempt counts are kept apart
		// so another session's traffic cannot enter this session's numbers, and
		// first_seen/last_seen are part of those numbers: computed over every
		// row, a consumer reading them got an instant from a session that is
		// deliberately excluded from everything else on this line.
		if r.whenOK && !inherited {
			stamp := r.when.UTC().Format(time.RFC3339)
			if d.FirstSeen == "" || stamp < d.FirstSeen {
				d.FirstSeen = stamp
			}
			if stamp > d.LastSeen {
				d.LastSeen = stamp
			}
		}
	}

	sort.Strings(hostOrder)
	for _, h := range hostOrder {
		d := agg[h]
		sort.Strings(d.Actions)
		sort.Strings(d.Reasons)
		// Inherited is derived at the end rather than tracked: it means "this
		// destination is not this session's at all", which is only knowable
		// once every row for the host has been seen.
		d.Inherited = d.Attempts == 0 && d.InheritedAttempts > 0
		obs.Hosts = append(obs.Hosts, *d)

		obs.Inherited += d.InheritedAttempts
		obs.Attempts += d.Attempts
		if d.Attempts > 0 {
			obs.DistinctHosts++
		}
	}
	return obs
}

func anyParsed(rows []row) bool {
	for _, r := range rows {
		if r.whenOK {
			return true
		}
	}
	return len(rows) == 0
}

// isInherited reports whether a row belongs to some other session.
//
// Two independent grounds, and either is sufficient. A run id that names a
// different run is decisive whatever the clock says. Otherwise the window
// decides, but only when it can be enforced: with no usable timestamp, calling
// a row inherited would quietly delete a real destination from the report, and
// the honest answer is to include it and say the window was not applied.
func isInherited(r row, w Window, windowApplied bool) bool {
	if w.RunID != "" && r.runID != "" && r.runID != w.RunID {
		return true
	}
	if !windowApplied || !r.whenOK {
		return false
	}
	if r.when.Before(w.Start) {
		return true
	}
	return !w.End.IsZero() && r.when.After(w.End)
}

// addOnce appends want unless it is already present, keeping the small
// action/reason sets on a destination free of duplicates.
//
// The parameter is `want` rather than `v`: `x == v` reads as a secret
// comparison to the constitution's CWE-208 grep and to a human skimming for
// one, and a lint trap costs a reviewer the same attention as a real finding.
func addOnce(xs []string, want string) []string {
	if want == "" {
		return xs
	}
	for _, x := range xs {
		if x == want {
			return xs
		}
	}
	return append(xs, want)
}
