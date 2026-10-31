package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/selfguard"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// steerUsage is the usage screen's lines for the steer verb.
const steerUsage = `  rashomon steer [off|on] [--user|--project]
                               say whether the notes are on, or turn them off
                               or on for you (--user, the default) or for this
                               project (--project); off sends the agent no
                               note and shows you what it would have said,
                               recording goes on, and the end-of-turn line
                               still prints
  rashomon steer on --repair   replace a config.json that does not read, after
                               asking on your terminal (never from an agent
                               session); until then the notes are off
  rashomon steer explain       the last decision: its pattern, state, the
                               reason it did or did not print, and what that
                               reason means
  rashomon steer log [--all] [--json]
                               the last 20 decisions (--all: every one): when,
                               session, event, pattern, level, state, reason,
                               delivery; never a note's text

`

// logTail is how many decisions `steer log` prints without --all.
const logTail = 20

// cmdSteer is `rashomon steer` (card STEER-CLI1, D213): the person's controls
// over AutoSteer's notes and a read of what it decided. `off` and `on` write
// config.json's steer_user or steer_projects key under UpdateConfig's lock
// and read as the hook reads them (hook.Post.steerOff); explain and log read
// the steer logs and write nothing. A switch needs a store to be read by:
// with none, the person is told so and nothing is made (summary's rule).
func cmdSteer(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return steerState(stdout)
	}
	switch args[0] {
	case "off", "on":
		project, rest, err := switchScope(args[1:])
		if err != nil {
			return err
		}
		repair := false
		if i := slices.Index(rest, "--repair"); i >= 0 {
			if args[0] != "on" {
				return errors.New("--repair goes with `steer on`: it replaces a config.json that does not read")
			}
			repair, rest = true, slices.Delete(rest, i, i+1)
		}
		if len(rest) > 0 {
			return fmt.Errorf("unknown argument %q", rest[0])
		}
		if repair {
			return steerRepair(stdout, project, askOnTerminal)
		}
		return steerSet(stdout, args[0] == "off", project)
	case "explain":
		if len(args) > 1 {
			return fmt.Errorf("unknown argument %q", args[1])
		}
		return steerExplain(stdout)
	case "log":
		return steerLog(stdout, args[1:])
	}
	return fmt.Errorf("unknown argument %q", args[0])
}

// steerState prints whether the notes are on here and why not.
func steerState(stdout io.Writer) error {
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		_, err = fmt.Fprintln(stdout, "steer: on (nothing has been recorded here, so no switch is set)")
		return err
	}
	if err != nil {
		return err
	}
	key, err := projectKey()
	if err != nil {
		return err
	}
	c, err := store.ReadConfig(st.Root())
	if errors.Is(err, store.ErrConfigUnreadable) {
		_, err = fmt.Fprintln(stdout, unreadableLine(st.Root(), err))
		return err
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, steerLine(c, key))
	return err
}

// unreadableLine is the state when config.json is there and does not read:
// the hook clamps the notes on it (config_unreadable), so it is off, and the
// line says why and what to do.
func unreadableLine(root string, err error) string {
	return fmt.Sprintf("steer: off (config.json is unreadable: %s; fix or delete %s)",
		safeText(store.ConfigReason(err)), safeText(filepath.Join(root, store.FileConfig)))
}

// unreadableConfigText is unreadableLine's reason in a refusal, for a verb
// that cannot write a file it cannot read.
func unreadableConfigText(root string, err error) error {
	return fmt.Errorf("config.json is unreadable: %s; the notes are off until it is fixed; fix or delete %s, "+
		"or run `rashomon steer on --repair` to replace it (it asks first); nothing was changed",
		safeText(store.ConfigReason(err)), safeText(filepath.Join(root, store.FileConfig)))
}

// askOnTerminal is the question a confirming verb asks: prompt on the
// controlling terminal, never on stdin, which a pipe can answer for the user.
func askOnTerminal(prompt string) (bool, error) {
	in, out, closeTTY, err := openTerminal()
	if err != nil {
		return false, errors.New("there is no terminal to ask on, so nothing was written; run it in your own terminal")
	}
	defer closeTTY()
	return askYN(in, out, prompt), nil
}

// steerChange is the scope's key set by `steer off|on`.
func steerChange(off, project bool, key string) func(*store.Config) {
	return func(c *store.Config) {
		switch {
		case project && off:
			if c.SteerProjects == nil {
				c.SteerProjects = map[string]string{}
			}
			c.SteerProjects[key] = store.SwitchOff
		case project:
			delete(c.SteerProjects, key)
		case off:
			c.SteerUser = store.SwitchOff
		default:
			c.SteerUser = ""
		}
	}
}

// steerRepair is `steer on --repair`: a config.json that does not read is
// replaced by a fresh one holding the switch, after ask says yes. It is a
// confirming verb as consent and purge are: refused in an agent session,
// asked on the terminal only. A file that reads is not replaced; the switch
// is set as `steer on` does.
func steerRepair(stdout io.Writer, project bool, ask func(prompt string) (bool, error)) error {
	if s := selfguard.AgentSignal(os.Getenv); s != "" {
		return fmt.Errorf("%s is set, so this is an agent session; nothing was written. Run `rashomon steer on --repair` in your own terminal.", s)
	}
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		return errors.New("nothing has been recorded here yet, so there is no store to hold the switch")
	}
	if err != nil {
		return err
	}
	key, err := projectKey()
	if err != nil {
		return err
	}
	_, err = store.ReadConfig(st.Root())
	if err == nil {
		return steerSet(stdout, false, project)
	}
	if !errors.Is(err, store.ErrConfigUnreadable) {
		return err
	}
	path := filepath.Join(st.Root(), store.FileConfig)
	ok, err := ask(fmt.Sprintf("%s is unreadable (%s). Replace it with a new one? Its content is lost. [y/N] ",
		safeText(path), safeText(store.ConfigReason(err))))
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("nothing was written")
	}
	c, err := store.RepairConfig(st.Root(), steerChange(false, project, key))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, steerLine(c, key))
	return err
}

// steerLine is the state in a line: the effective switch, then each scope's.
func steerLine(c store.Config, key string) string {
	state := "on"
	if c.SteerOffFor(key) {
		state = "off"
	}
	user, proj := "on", "on"
	if c.SteerOffUser() {
		user = "off"
	}
	if c.SteerProjects[key] == store.SwitchOff {
		proj = "off"
	}
	return fmt.Sprintf("steer: %s (you: %s; this project: %s, %s)", state, user, proj, safeText(key))
}

// steerVerb is the `steer off|on` command line for a scope.
func steerVerb(off, project bool) string {
	v := "on"
	if off {
		v = "off"
	}
	if project {
		v += " --project"
	}
	return v
}

// steerSet is `steer off|on`: the scope's key, under the config lock. It is
// refused in an agent session as repair is, so the steered agent cannot turn
// its own notes off.
func steerSet(stdout io.Writer, off, project bool) error {
	if s := selfguard.AgentSignal(os.Getenv); s != "" {
		return fmt.Errorf("%s is set, so this is an agent session; nothing was written. Run `rashomon steer %s` in your own terminal.", s, steerVerb(off, project))
	}
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		return errors.New("nothing has been recorded here yet, so there is no store to hold the switch")
	}
	if err != nil {
		return err
	}
	key, err := projectKey()
	if err != nil {
		return err
	}
	c, err := store.UpdateConfig(st.Root(), steerChange(off, project, key))
	if errors.Is(err, store.ErrConfigUnreadable) {
		return unreadableConfigText(st.Root(), err)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, steerLine(c, key))
	return err
}

// steerRow is one steer line as the verbs read it: only closed values, each
// taken from its vocabulary or "?" (closed), so a line a hand wrote cannot put
// text of its own on the screen, and never the note's text.
type steerRow struct {
	AtUnixMS     int64  `json:"at_unix_ms"`
	Session      string `json:"session"`
	Event        string `json:"event"`
	Pattern      string `json:"pattern"`
	Level        string `json:"level"`
	State        string `json:"state"`
	SuppressedBy string `json:"suppressed_by"`
	Delivery     string `json:"delivery"`
}

// closed is v when it is one of the vocabulary, else "?".
func closed(v string, vocab []string) string {
	if slices.Contains(vocab, v) {
		return v
	}
	return "?"
}

// vocab is a closed list's values as strings.
func vocab[T ~string](all []T) []string {
	out := make([]string, len(all))
	for i, v := range all {
		out[i] = string(v)
	}
	return out
}

// rowOf is a decoded line as the verbs show it. A line's null reason and
// delivery read as "-".
func rowOf(l steer.Line) steerRow {
	r := steerRow{AtUnixMS: l.AtUnixMS, Session: safeText(l.SessionID), SuppressedBy: "-", Delivery: "-"}
	r.Event = closed(l.Event, vocab(steer.AllEvents()))
	r.Pattern = "?"
	if pattern.Valid(pattern.ID(l.Pattern)) {
		r.Pattern = l.Pattern
	}
	levels := make([]string, 0, 4)
	for _, lv := range steer.AllLevels() {
		levels = append(levels, lv.String())
	}
	r.Level = closed(l.Level, levels)
	r.State = closed(string(l.State), vocab(steer.AllLineStates()))
	if l.SuppressedBy != nil {
		r.SuppressedBy = closed(string(*l.SuppressedBy), vocab(steer.AllSuppressedBy()))
	}
	if l.Delivery != nil {
		r.Delivery = closed(string(*l.Delivery), vocab(steer.AllDelivery()))
	}
	return r
}

// steerRows is every steer line of every run in the store, oldest first, and
// how many lines could not be read.
func steerRows(st *store.Store) (rows []steerRow, skipped int, err error) {
	runs, err := st.Runs()
	if err != nil {
		return nil, 0, err
	}
	for _, run := range runs {
		raw, n, err := st.ReadSteer(run)
		if err != nil {
			return nil, 0, err
		}
		skipped += n
		for _, b := range raw {
			var l steer.Line
			err := json.Unmarshal(b, &l)
			if err == nil && l.Type == steer.EvidenceLineType {
				continue // a channel-evidence line is valid and is not a decision
			}
			if err != nil || l.Type != steer.LineType {
				skipped++
				continue
			}
			rows = append(rows, rowOf(l))
		}
	}
	slices.SortStableFunc(rows, func(a, b steerRow) int { return int(a.AtUnixMS - b.AtUnixMS) })
	return rows, skipped, nil
}

// readSteerRows opens the store for a read and returns its rows, ok false
// after saying there is no store.
func readSteerRows(stdout io.Writer) (rows []steerRow, skipped int, ok bool, err error) {
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		_, err = fmt.Fprintln(stdout, nothingDecidedHere)
		return nil, 0, false, err
	}
	if err != nil {
		return nil, 0, false, err
	}
	rows, skipped, err = steerRows(st)
	return rows, skipped, err == nil, err
}

// nothingDecidedHere is what the read-only steer verbs say with no store.
const nothingDecidedHere = "nothing has been recorded here, so no steer decision has been made"

// steerLog is `steer log`: the decisions, oldest first, as closed fields.
func steerLog(stdout io.Writer, args []string) error {
	var all, asJSON bool
	for _, a := range args {
		switch a {
		case "--all":
			all = true
		case "--json":
			asJSON = true
		default:
			return fmt.Errorf("unknown argument %q", a)
		}
	}
	rows, skipped, ok, err := readSteerRows(stdout)
	if !ok {
		return err
	}
	if !all && len(rows) > logTail {
		rows = rows[len(rows)-logTail:]
	}
	if asJSON {
		return printJSON(stdout, struct {
			Decisions  []steerRow `json:"decisions"`
			Unreadable int        `json:"unreadable"`
		}{append([]steerRow{}, rows...), skipped})
	}
	if len(rows) == 0 {
		_, err = fmt.Fprintln(stdout, "steer log: no decisions recorded")
		return err
	}
	for _, r := range rows {
		fmt.Fprintf(stdout, "%s  session %s  %s  %s  level %s  %s  by %s  delivery %s\n",
			time.UnixMilli(r.AtUnixMS).UTC().Format("2006-01-02T15:04:05Z"), r.Session, r.Event, r.Pattern,
			r.Level, r.State, r.SuppressedBy, r.Delivery)
	}
	if skipped > 0 {
		fmt.Fprintf(stdout, "steer log: %d line(s) could not be read\n", skipped)
	}
	return nil
}

// steerExplain is `steer explain`: the newest decision in words, after the
// state's line when config.json does not read.
func steerExplain(stdout io.Writer) error {
	if st, err := openStoreForRead(); err == nil {
		if _, err := store.ReadConfig(st.Root()); errors.Is(err, store.ErrConfigUnreadable) {
			fmt.Fprintln(stdout, unreadableLine(st.Root(), err))
		}
	}
	rows, _, ok, err := readSteerRows(stdout)
	if !ok {
		return err
	}
	if len(rows) == 0 {
		_, err = fmt.Fprintln(stdout, "steer explain: no decisions recorded")
		return err
	}
	r := rows[len(rows)-1]
	fmt.Fprintf(stdout, "last decision: %s, session %s, at %s\n", r.Pattern, r.Session,
		time.UnixMilli(r.AtUnixMS).UTC().Format("2006-01-02T15:04:05Z"))
	fmt.Fprintf(stdout, "  event:         %s\n  level:         %s\n  state:         %s\n  suppressed_by: %s\n  delivery:      %s\n",
		r.Event, r.Level, r.State, r.SuppressedBy, r.Delivery)
	_, err = fmt.Fprintf(stdout, "  meaning:       %s\n", meaning(r))
	return err
}

// reasonMeaning is each suppressed_by reason in a sentence. It is closed with
// the vocabulary: TestSTC1e_EveryReasonIsExplained fails on a reason without
// one. It is built on first use: a table built at package init is allocated by
// every hook call, and only `steer explain` reads it (RES-2's init budget).
var reasonMeaning = sync.OnceValue(func() map[steer.SuppressedBy]string {
	return map[steer.SuppressedBy]string{
		steer.SuppressedByBudgetPrompt:      "this prompt had already had its three notes",
		steer.SuppressedByBudgetPattern:     "this prompt had already been told the same thing",
		steer.SuppressedByBudgetBatch:       "another note in the same call went first",
		steer.SuppressedByStopHookActive:    "the turn was already being continued by a stop hook",
		steer.SuppressedByBackgroundRunning: "a background task was still running",
		steer.SuppressedByHostForeign:       "the call came from a host rashomon is not installed for",
		steer.SuppressedBySteerOff:          "you turned the notes to your agent off (rashomon steer on turns them back on); nothing reached the agent",
		steer.SuppressedByConfigUnreadable:  "config.json did not read, so the notes were off; nothing reached the agent (fix or delete the file, or run rashomon steer on --repair)",
		steer.SuppressedByConsentMissing:    "the notes were not agreed to on this build's consent screen",
		steer.SuppressedByArmOff:            "this install's lab arm is off",
		steer.SuppressedByDemoted:           "the pattern was held to a flag after it proved noisy",
		steer.SuppressedByLockTimeout:       "the steer log was busy, so the note was dropped rather than said twice",
		steer.SuppressedByRenderRefused:     "the note's text failed its checks and was not shown",
		steer.SuppressedByDeadline:          "the hook ran out of time",
		steer.SuppressedByDenyCap:           "the cap on denials was reached",
		steer.SuppressedBySlotRejected:      "a value the note quotes failed its checks",
		steer.SuppressedByOutputGate:        "the note was over the size cap",
		steer.SuppressedByUnverifiedChannel: "Claude Code's version has not been verified to carry the note to the agent, or a transcript did not show an earlier note delivered on this event",
		steer.SuppressedByNoChannel:         "this event has no way to carry a note",
		steer.SuppressedByNoMirror:          "nobody was at the keyboard to be shown the note",
		steer.SuppressedByHarnessOff:        "this harness is switched off",
		steer.SuppressedBySurfaceOff:        "this surface is switched off",
		steer.SuppressedByLate:              "the note arrived after the moment it applied to",
	}
})

// meaning is a row in a sentence: its reason when it has one, else what its
// delivery says happened.
func meaning(r steerRow) string {
	if m, ok := reasonMeaning()[steer.SuppressedBy(r.SuppressedBy)]; ok {
		if r.State == string(steer.LineStateDelivered) {
			return "shown to you as a flag, because " + m
		}
		return "nothing was printed, because " + m
	}
	switch steer.Delivery(r.Delivery) {
	case steer.DeliveryDelivered:
		return "the agent was told, and you were shown the same note"
	case steer.DeliveryFlagOnly:
		return "shown to you only; the agent was not told"
	case steer.DeliveryHoldout:
		return "held out of the field holdout: neither you nor the agent was shown it"
	}
	return "no note was printed"
}
