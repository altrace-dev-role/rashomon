package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// statusWindow is how far back status counts the sessions with tool calls.
const statusWindow = 7 * 24 * time.Hour

// eventSince is, for an event Claude Code added after the oldest version the
// consent screen supports (2.1.280), the version that added it; an event with
// no entry exists on every supported version, so a missing marker for it is
// never a version. None of the eight is newer than the floor (PreToolUse and
// PostToolUse are the original two, PostToolUseFailure was measured on
// 2.1.258), so the table is empty; an event added to install.Events() later
// that needs a newer Claude Code adds its row here.
var eventSince = map[string]string{}

// firedRemedy is a missing-marker row's remedy: the causes that can leave an
// installed event with no marker in sessions that carry others. They are the
// entry's absence, a pause or stand-down (no entry marks then), a folder
// Claude Code has not trusted (it runs no hook there) and, only for an event
// newer than the oldest supported Claude Code, a Claude Code too old to have
// it. None is proven from here, so the line says "no marker", never "not
// firing".
func firedRemedy(event string) string {
	causes := ""
	if since, ok := eventSince[event]; ok {
		causes = "Claude Code before " + since + " lacks this event, or "
	}
	return causes + "recording was paused or stood down, or this folder is not trusted, or run `rashomon watch` to reinstall its entry"
}

// errCheckpointShape is a verify record whose recording or point_seq is not
// the type the checkpoint writes.
var errCheckpointShape = errors.New("the last checkpoint's recording or point_seq does not read")

// cmdStatusVerb is `rashomon status [--json]`. --json prints what status adds
// below the lines it prints as it reads (statusView), as one object.
func cmdStatusVerb(args []string, stdout io.Writer) error {
	asJSON, err := jsonFlag(args)
	if err != nil {
		return err
	}
	if !asJSON {
		return cmdStatus(stdout)
	}
	root, err := store.DefaultRoot()
	if err != nil {
		return err
	}
	st, err := statusStore(root)
	if err != nil {
		return err
	}
	return printJSON(stdout, statusOf(root, st, time.Now()))
}

// cmdStatus prints what is installed here and what the store holds, and writes
// nothing at all.
//
// Reading the store is the easy way to break that. Opening a store creates one,
// key material and all, so the command whose whole answer may be "nothing is
// installed on this machine" must not be the command that installs something.
// It looks for the store the same way a plain detach does: by the one file that
// is only there if a store is (statusStore).
func cmdStatus(stdout io.Writer) error {
	root, err := store.DefaultRoot()
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "store: %s\n", root)
	statusRecording(stdout, root)
	statusRecordScope(stdout, root)

	installID := ""
	st, err := statusStore(root)
	if err != nil {
		return err
	}
	if st != nil {
		installID = st.InstallID()
		fmt.Fprintf(stdout, "  present: yes (install %s)\n", installID)
		runs, err := st.Runs()
		if err != nil {
			fmt.Fprintf(stdout, "  runs: %s\n", statusUnknown)
		} else {
			fmt.Fprintf(stdout, "  runs: %d\n", len(runs))
		}
		statusSize(stdout, st)
		fmt.Fprintf(stdout, "  %s\n", bodiesLine(st))
		statusLockWait(stdout, st)
	} else {
		fmt.Fprintln(stdout, "  present: no")
	}
	statusCapProblem(stdout, "  cap: ", st)

	settingsLive, err := statusSettings(stdout, installID)
	if err != nil {
		return err
	}
	pluginLive, err := statusPlugin(stdout)
	if err != nil {
		return err
	}
	// Named here so a user sees it without running report, and in the SAME
	// words report uses (duplicate_declarations): a settings install and an
	// enabled plugin can share one machine during the migration window Part 1's
	// spec describes, and nothing removes either of them on its own. detach is
	// the resolution, and it is the settings entries it removes -- the
	// plugin's are not detach's to touch.
	if settingsLive && pluginLive {
		fmt.Fprintln(stdout, "overlap: both a settings install and the rashomon plugin provide these entries, "+
			"recording every tool call twice (report will read coverage: unverified, reason duplicate_declarations); "+
			"run `rashomon detach` to remove the settings entries and keep the plugin")
	}
	if err := statusHooks(stdout); err != nil {
		return err
	}
	if err := statusRecap(stdout, root); err != nil {
		return err
	}
	_, err = io.WriteString(stdout, statusOf(root, st, time.Now()).text())
	return err
}

// statusStore opens root's store when install.json says one is there, and is
// nil when none is: status creates nothing.
func statusStore(root string) (*store.Store, error) {
	if _, err := os.Stat(filepath.Join(root, installMetaFile)); err != nil {
		return nil, nil
	}
	return openStore()
}

// statusView is what status adds: the hook events against the sessions they
// fired in, the harness stand-downs, the last Stop's recording and the consent
// agreed. Its JSON keys are closed; a list is [] when empty and an object is
// null when there is none or status could not read it, which the text says.
type statusView struct {
	Store             string  `json:"store"`
	Session           *string `json:"session"`
	SessionsWithCalls int     `json:"sessions_with_calls"`
	// BeforeMarkers is how many of the window's sessions, the newest apart,
	// carry no marker at all: they ran on a build that wrote none, so no event
	// is judged over them.
	BeforeMarkers     int             `json:"sessions_before_markers"`
	Events            []statusEvent   `json:"events"`
	HarnessMismatches []statusHarness `json:"harness_mismatches"`
	LastStop          *statusStop     `json:"last_stop"`
	Consent           *statusConsent  `json:"consent"`
	// Unknown names the blocks status could not read: a null above is then
	// "unknown", not "none". [] when every block read.
	Unknown []string `json:"unknown"`

	// The reads that failed, each said as unknown in the text.
	runsErr, stopErr, harnessErr, consentErr error
}

// statusEvent is one installed event: whether it fired in the newest session
// with tool calls, and in how many of the sessions it is judged over (those
// that carry a marker).
type statusEvent struct {
	Event         string `json:"event"`
	Installed     bool   `json:"installed"`
	NewestSession bool   `json:"newest_session"`
	Sessions      int    `json:"sessions"`
	Of            int    `json:"of"`
	NotFiring     bool   `json:"not_firing"`
}

// statusHarness is a harness_counts.json key whose sessions stood down.
type statusHarness struct {
	Key       string `json:"key"`
	Seen      int    `json:"seen"`
	StoodDown int    `json:"stood_down"`
}

// statusStop is the session's last checkpoint: Checked is false when no Stop
// has cut one, and Notes say why an incomplete record was.
type statusStop struct {
	Session   string            `json:"session"`
	Checked   bool              `json:"checked"`
	Recording *verify.Recording `json:"recording"`
	PointSeq  *int64            `json:"point_seq"`
	Notes     []string          `json:"notes"`
}

// statusConsent is the last act of consent on file beside this build's
// versions; Current is whether it covers both.
type statusConsent struct {
	ScopeVersion         int   `json:"scope_version"`
	SteerDisclosure      int   `json:"steer_disclosure"`
	AgreedAtUnixMS       int64 `json:"agreed_at_unix_ms"`
	BuildScopeVersion    int   `json:"build_scope_version"`
	BuildSteerDisclosure int   `json:"build_steer_disclosure"`
	Current              bool  `json:"current"`
}

// statusRun is one run directory that holds a call.
type statusRun struct {
	name string
	store.RunActivity
}

// statusOf reads the view. st is nil with no store, and then only the files
// at root are read: nothing is created.
func statusOf(root string, st *store.Store, now time.Time) statusView {
	v := statusView{Store: root, Events: []statusEvent{}}
	if st != nil {
		v.readRuns(st, now)
	}
	v.HarnessMismatches, v.harnessErr = harnessMismatches(root)
	v.Consent, v.consentErr = consentAgreed(root)
	v.Unknown = []string{}
	stopErr := v.stopErr
	if v.runsErr != nil {
		stopErr = v.runsErr
	}
	for _, b := range []struct {
		name string
		err  error
	}{{"events", v.runsErr}, {"last_stop", stopErr}, {"harness_mismatches", v.harnessErr}, {"consent", v.consentErr}} {
		if b.err != nil {
			v.Unknown = append(v.Unknown, b.name)
		}
	}
	return v
}

// readRuns fills the events and the last Stop from the runs that hold a call.
func (v *statusView) readRuns(st *store.Store, now time.Time) {
	newest, window, err := sessionsWithCalls(st, now)
	if err != nil || newest == nil {
		v.runsErr = err
		return
	}
	v.Session, v.SessionsWithCalls = &newest.name, len(window)
	if len(window) > 0 {
		v.Events = eventsOf(newest, window, installedEvents(st.InstallID()))
		for _, r := range window {
			if r.name != newest.name && len(r.Fired) == 0 {
				v.BeforeMarkers++
			}
		}
	}
	v.LastStop, v.stopErr = lastStop(st, newest.name)
}

// sessionsWithCalls is the store's runs that hold a call: the newest of them,
// any age, and those whose last call is within statusWindow of now. Every run
// directory costs one stat and one listing.
func sessionsWithCalls(st *store.Store, now time.Time) (newest *statusRun, window []statusRun, err error) {
	names, err := st.Runs()
	if err != nil {
		return nil, nil, err
	}
	for _, name := range names {
		a, err := st.Activity(name)
		if err != nil {
			return nil, nil, err
		}
		if !a.Calls {
			continue
		}
		r := statusRun{name, a}
		if newest == nil || !a.LastAt.Before(newest.LastAt) {
			newest = &r
		}
		if now.Sub(a.LastAt) <= statusWindow {
			window = append(window, r)
		}
	}
	return newest, window, nil
}

// firedRule is when a session with tool calls fires event: SessionEnd and
// Stop are judged over the sessions before the newest, which may still be
// open or mid-turn, and the two failure events fire only on what onlyWhen
// names, so a count of 0 for them is never "not firing".
func firedRule(event string) (earlierOnly bool, onlyWhen string) {
	switch event {
	case "SessionEnd", "Stop":
		return true, ""
	case "PostToolUseFailure":
		return false, "a tool call fails"
	case "StopFailure":
		return false, "a turn ends in an API error"
	}
	return false, ""
}

// eventsOf is each installed event's row over the window's sessions that carry
// a marker: only a session run on a build that writes markers can show an
// event missing, so one with none (it predates them) is not counted. An event
// every such session fires that has a marker in none of them, the newest
// included, is flagged.
func eventsOf(newest *statusRun, window []statusRun, installed []bool) []statusEvent {
	events := install.Events()
	rows := make([]statusEvent, 0, len(events))
	for i, e := range events {
		earlierOnly, onlyWhen := firedRule(e)
		ev := statusEvent{Event: e, Installed: installed[i], NewestSession: slices.Contains(newest.Fired, e)}
		for _, r := range window {
			if len(r.Fired) == 0 || earlierOnly && r.name == newest.name {
				continue
			}
			ev.Of++
			if slices.Contains(r.Fired, e) {
				ev.Sessions++
			}
		}
		ev.NotFiring = onlyWhen == "" && ev.Of > 0 && ev.Sessions == 0 && !ev.NewestSession
		rows = append(rows, ev)
	}
	return rows
}

// installedEvents is, per install.Events(), whether this install's settings
// entry or the enabled plugin provides it. A settings file status cannot read
// vouches for none.
func installedEvents(installID string) []bool {
	events := install.Events()
	on := make([]bool, len(events))
	path, err := settings.UserPath()
	if err != nil {
		return on
	}
	doc, err := settings.Load(path)
	if err != nil {
		return on
	}
	p, _ := install.FindPlugin(doc)
	for i, e := range events {
		present, _ := install.Present(doc, installID, e)
		on[i] = present || p != nil && p.Enabled && slices.Contains(p.Declared, e)
	}
	return on
}

// lastStop is the run's last checkpoint, read with jsonscan (no decode on
// this package's paths); an incomplete one's notes are recording's own at
// its point, from the run read whole.
func lastStop(st *store.Store, name string) (*statusStop, error) {
	stop := &statusStop{Session: name, Notes: []string{}}
	line, ok, _, err := st.LatestRecordBytes(name, store.TypeVerify)
	if err != nil || !ok {
		return stop, err
	}
	var out [3]jsonscan.Span
	if err := jsonscan.Top(line, jsonscan.NewKeySet("point_seq", "recording", "point"), out[:]); err != nil {
		return nil, err
	}
	// A checkpoint at another point (a commit's, a push's) is not a Stop's:
	// it says nothing of how the last Stop's record stood.
	if out[2].Kind == jsonscan.String {
		if point, err := jsonscan.DecodeString(line, out[2]); err == nil && point != verify.PointStop {
			return stop, nil
		}
	}
	if out[0].Kind != jsonscan.Number || out[1].Kind != jsonscan.String {
		return nil, errCheckpointShape
	}
	seq, err := strconv.ParseInt(string(out[0].Raw(line)), 10, 64)
	if err != nil {
		return nil, err
	}
	word, err := jsonscan.DecodeString(line, out[1])
	if err != nil {
		return nil, err
	}
	recording := verify.Recording(word)
	stop.Checked, stop.Recording, stop.PointSeq = true, &recording, &seq
	if recording != verify.Incomplete {
		return stop, nil
	}
	run, err := st.ReadRunDir(name)
	if err != nil {
		return nil, err
	}
	_, notes := verify.RecordingAt(run, seq)
	stop.Notes = append(stop.Notes, notes...)
	return stop, nil
}

// harnessMismatches is every harness_counts.json key with a stand-down.
func harnessMismatches(root string) ([]statusHarness, error) {
	totals, err := store.HarnessTotals(root)
	if err != nil {
		return nil, err
	}
	rows := []statusHarness{}
	for _, t := range totals {
		if t.StoodDown > 0 {
			rows = append(rows, statusHarness{Key: t.Key, Seen: t.Seen, StoodDown: t.StoodDown})
		}
	}
	return rows, nil
}

// consentAgreed is the last act of consent on file, nil for none.
func consentAgreed(root string) (*statusConsent, error) {
	recs, err := consent.Read(root)
	if err != nil {
		return nil, err
	}
	rec, ok := consent.LastAccepted(recs)
	if !ok {
		return nil, nil
	}
	return &statusConsent{
		ScopeVersion: rec.ScopeVersion, SteerDisclosure: rec.SteerDisclosure, AgreedAtUnixMS: rec.At,
		BuildScopeVersion: disclose.ScopeVersion, BuildSteerDisclosure: steer.DisclosureVersion,
		Current: rec.ScopeVersion >= disclose.ScopeVersion && rec.SteerDisclosure >= steer.DisclosureVersion,
	}, nil
}

// text is the view as status's closing lines.
func (v statusView) text() string {
	var b strings.Builder
	switch {
	case v.runsErr != nil:
		fmt.Fprintf(&b, "events: %s (the run directories could not be read)\n", statusUnknown)
	case v.Session == nil:
		b.WriteString("events: nothing recorded yet\n")
	case v.SessionsWithCalls == 0:
		b.WriteString("events: no session with tool calls in the last 7 days\n")
	default:
		fmt.Fprintf(&b, "events: newest session %s; %d %s with tool calls in the last 7 days",
			*v.Session, v.SessionsWithCalls, plural(v.SessionsWithCalls, "session", "sessions"))
		if n := v.BeforeMarkers; n > 0 {
			fmt.Fprintf(&b, "; %d earlier %s %s the markers", n, plural(n, "session", "sessions"), plural(n, "predates", "predate"))
		}
		b.WriteString("\n")
		for _, ev := range v.Events {
			b.WriteString(ev.line())
		}
	}
	switch {
	case v.harnessErr != nil:
		fmt.Fprintf(&b, "harness mismatches: %s (%s could not be read)\n", statusUnknown, store.FileHarnessCounts)
	case len(v.HarnessMismatches) == 0:
		b.WriteString("harness mismatches: none\n")
	default:
		b.WriteString("harness mismatches:\n")
		for _, h := range v.HarnessMismatches {
			fmt.Fprintf(&b, "  %s: stood down in %d of %d sessions seen\n", h.Key, h.StoodDown, h.Seen)
		}
	}
	b.WriteString(v.stopLine())
	b.WriteString(v.consentLine())
	return b.String()
}

// line is one event's row.
func (ev statusEvent) line() string {
	earlierOnly, onlyWhen := firedRule(ev.Event)
	sessions := "sessions"
	if earlierOnly {
		sessions = "earlier sessions"
	}
	if ev.NotFiring {
		return fmt.Sprintf("  %s: no marker: 0 of %d %s with a marker; %s\n", ev.Event, ev.Of, sessions, firedRemedy(ev.Event))
	}
	here := "not in the newest session"
	if ev.NewestSession {
		here = "fired in the newest session"
	}
	count := fmt.Sprintf("%d of %d %s", ev.Sessions, ev.Of, sessions)
	if ev.Of == 0 {
		count = "no earlier session"
	}
	if onlyWhen != "" {
		count += " (fires only when " + onlyWhen + ")"
	}
	return "  " + ev.Event + ": " + here + "; " + count + "\n"
}

// stopLine is the last Stop's line.
func (v statusView) stopLine() string {
	s := v.LastStop
	switch {
	case v.runsErr != nil || v.stopErr != nil:
		return "last stop: " + statusUnknown + " (the last checkpoint could not be read)\n"
	case v.Session == nil:
		return "last stop: nothing recorded yet\n"
	case !s.Checked:
		return "last stop: no Stop has checked session " + s.Session + "\n"
	}
	line := fmt.Sprintf("last stop: session %s recorded %s at seq %d", s.Session, *s.Recording, *s.PointSeq)
	if len(s.Notes) > 0 {
		line += ": " + strings.Join(s.Notes, ", ")
	}
	return line + "\n"
}

// consentLine is the consent agreed, against this build's versions.
func (v statusView) consentLine() string {
	c := v.Consent
	switch {
	case v.consentErr != nil:
		return "consent: " + statusUnknown + " (" + store.FileConsent + " could not be read)\n"
	case c == nil:
		return "consent: none on file\n"
	}
	line := fmt.Sprintf("consent: screen version %d, steer disclosure %d, agreed %s; this build shows %d, %d",
		c.ScopeVersion, c.SteerDisclosure, time.UnixMilli(c.AgreedAtUnixMS).UTC().Format(time.RFC3339),
		c.BuildScopeVersion, c.BuildSteerDisclosure)
	if !c.Current {
		line += ": run `rashomon consent` to see what changed"
	}
	return line + "\n"
}

// plural is one when n is 1, else many.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
