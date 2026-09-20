// Package install owns the hook entries this program places in the user's
// settings: how they are rendered, how they are recognised, and what counts as
// one of ours having been tampered with.
package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/settings"
)

// Events we install into. PreToolUse records what was asked for and
// PostToolUse records what ran. SessionStart and SessionEnd are the liveness
// probe, and they are also what makes "at start and at end" in the coverage
// record literal rather than approximate.
const (
	EventPreToolUse  = "PreToolUse"
	EventPostToolUse = "PostToolUse"
	// EventPostToolUseFailure is where a failed call goes, and subscribing to
	// it is not an improvement in coverage -- it is the difference between
	// recording failures and recording none.
	//
	// Measured on Claude Code 2.1.258: a failing Bash call fires THIS event and
	// not PostToolUse. With only PostToolUse installed, every failed call left
	// a declaration with no execution beside it, which is the same shape on
	// disk as a call the user denied and a call whose execution went
	// unrecorded. The report could not have told the three apart, so the
	// silent-failure line -- the one that says how many calls failed while the
	// final message mentioned none -- would have counted zero on every session.
	//
	// It shares the `post` command line rather than getting one of its own:
	// the payloads have the same shape, `post` dispatches on hook_event_name,
	// and a second command line would be a second place for the attribution
	// and panic-barrier discipline to drift.
	EventPostToolUseFailure = "PostToolUseFailure"
	EventSessionStart       = "SessionStart"
	EventSessionEnd         = "SessionEnd"
)

// Events in the order they are installed.
var Events = []string{
	EventPreToolUse,
	EventPostToolUse,
	EventPostToolUseFailure,
	EventSessionStart,
	EventSessionEnd,
}

// hasMatcher reports whether an event's entry carries a matcher. The tool
// events match on tool name; the session events match on source or reason, and
// omitting the matcher is the documented way to match every one of those.
func hasMatcher(event string) bool {
	return event == EventPreToolUse ||
		event == EventPostToolUse ||
		event == EventPostToolUseFailure
}

const (
	// Matcher is installed on the tool events. A narrower matcher is the
	// classic silent under-count: "Bash" means Edit, Write, WebFetch and every
	// mcp__* call produce no declaration while the report goes on claiming
	// full coverage.
	Matcher = "*"
	// Timeout is in seconds. Claude Code's documented default is 600.
	Timeout = 5

	// Marker precedes the install id in an installed command line. The hook
	// paths read it back to tell an entry of ours from one belonging to
	// another install that shares the settings file.
	Marker = "--install"
)

// Spec is what an installed entry points at and who owns it.
type Spec struct {
	Executable string
	InstallID  string
}

// Executable resolves the running binary's path for installation.
//
// A binary under a go-build directory is a `go run` artefact that will not
// exist tomorrow, and installing a hook that points at it produces a config
// that fails on every tool call. Refusing is the only honest answer.
func Executable() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	p, err = filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	if strings.Contains(p, string(filepath.Separator)+"go-build") {
		return "", errors.New("install: refusing to install a `go run` temporary binary; build rashomon and run the built binary")
	}
	return p, nil
}

func subcommand(event string) string {
	switch event {
	case EventPreToolUse:
		return "hook"
	case EventPostToolUse, EventPostToolUseFailure:
		// One subcommand, two events. `post` reads hook_event_name and decides
		// which it is; see EventPostToolUseFailure for why that is preferable
		// to a second command line.
		return "post"
	case EventSessionStart:
		return "probe start"
	case EventSessionEnd:
		return "probe end"
	}
	return ""
}

// Command is the shell command line installed for an event.
func (s Spec) Command(event string) string {
	return shellQuote(s.Executable) + " " + subcommand(event) + " " + Marker + " " + s.InstallID
}

type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

type matcherGroup struct {
	Matcher *string       `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

func (s Spec) group(event string) matcherGroup {
	g := matcherGroup{Hooks: []hookCommand{{
		Type:    "command",
		Command: s.Command(event),
		Timeout: Timeout,
	}}}
	if hasMatcher(event) {
		m := Matcher
		g.Matcher = &m
	}
	return g
}

// Entry renders our matcher group as it sits as an item of hooks.<event>,
// indented for that position so it reads as native in the file.
//
// It returns an error rather than panicking. The values marshalled here are
// this package's own structs, so a failure is not reachable by any input --
// which is exactly the argument that made a panic look free. It is not free:
// this package is on the recorder's import path (internal/hook imports it), a
// panic in library code is a Law 14 violation, and the recorder's whole
// contract is that nothing it does can exit 2 and block a tool call. The only
// caller already returns an error.
func (s Spec) Entry(event string) (json.RawMessage, error) {
	b, err := json.MarshalIndent(s.group(event), settings.Indent(3), "  ")
	if err != nil {
		return nil, fmt.Errorf("install: rendering the %s entry: %w", event, err)
	}
	return b, nil
}

// Owner returns the install id an entry claims for an event, or "" for an
// entry that is not ours. Ownership is carried in the command line itself,
// which survives every round trip a settings file can take.
func Owner(raw json.RawMessage, event string) string {
	var g struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if json.Unmarshal(raw, &g) != nil {
		return ""
	}
	suffix := " " + subcommand(event) + " " + Marker + " "
	for _, h := range g.Hooks {
		if i := strings.LastIndex(h.Command, suffix); i >= 0 {
			if id := h.Command[i+len(suffix):]; isInstallID(id) {
				return id
			}
		}
	}
	return ""
}

// Apply ensures exactly one entry of ours per event, refreshing it if the
// binary has moved and collapsing duplicates. It reports whether the document
// changed, so a no-op watch writes nothing. An entry of ours that is no longer
// intact -- a second hook added to the group, a changed matcher -- is refused,
// not overwritten.
func Apply(doc *settings.Document, spec Spec) (bool, error) {
	changed := false
	for _, event := range Events {
		entries, err := doc.HookEntries(event)
		if err != nil {
			return false, err
		}
		want, err := spec.Entry(event)
		if err != nil {
			return false, err
		}

		var (
			out          []json.RawMessage
			found        bool
			eventChanged bool
		)
		for _, e := range entries {
			if Owner(e, event) != spec.InstallID {
				out = append(out, e)
				continue
			}
			if detail := intact(e, event); detail != "" {
				// Someone added to or altered our group. Overwriting it would
				// delete their work; refusing names what changed.
				return false, &ErrModified{Event: event, Detail: detail}
			}
			if found {
				eventChanged = true
				continue
			}
			found = true
			if !sameJSON(e, want) {
				eventChanged = true
			}
			out = append(out, want)
		}
		if !found {
			out = append(out, want)
			eventChanged = true
		}

		if eventChanged {
			if err := doc.SetHookEntries(event, out); err != nil {
				return false, err
			}
			changed = true
		}
	}
	return changed, nil
}

// ErrModified reports that one of our entries was edited in a way that changes
// what it captures. detach refuses rather than delete something it no longer
// recognises as its own, and watch refuses rather than overwrite it.
type ErrModified struct {
	Event  string
	Detail string
}

// Modified names an entry of ours that someone has edited, so a caller can
// say which and leave it alone without abandoning the rest.
type Modified struct {
	Event  string
	Detail string
}

func (e *ErrModified) Error() string {
	// The recovery is named, because the refusal without one is what made a
	// single edited value feel like a locked door: restore the value, or say
	// --force and have it removed as it stands.
	return fmt.Sprintf("install: our %s entry was modified (%s); not touching it "+
		"-- restore that value, or re-run detach with --force to remove it as it stands", e.Event, e.Detail)
}

// Remove drops every entry claimed by one install and reports how many, along
// with any entry it left alone because someone had edited it.
func Remove(doc *settings.Document, spec Spec, force bool) (int, []Modified, error) {
	return RemoveIf(doc, func(id string) bool { return id == spec.InstallID }, force)
}

// RemoveIf drops every entry of ours whose install id satisfies match, under
// the same rules as Remove. It is what detach --all needs: on a machine whose
// store is gone the install id cannot be read back, and the marker in the
// command line is then the only thing left that recognises an entry as ours.
//
// An entry that is not ours is never offered to match. Its id is "", and a
// predicate written to accept anything would otherwise delete another tool's
// hooks.
func RemoveIf(doc *settings.Document, match func(installID string) bool, force bool) (int, []Modified, error) {
	total := 0
	var left []Modified
	for _, event := range Events {
		entries, err := doc.HookEntries(event)
		if err != nil {
			return 0, nil, err
		}

		var (
			out     []json.RawMessage
			removed int
		)
		for _, e := range entries {
			id := Owner(e, event)
			if id == "" || !match(id) {
				out = append(out, e)
				continue
			}
			if detail := intact(e, event); detail != "" {
				// Refusing to clobber an entry someone edited is deliberate
				// and is H-6's and H-7's contract, so the default is
				// unchanged: the whole document is left alone and the caller
				// is told which entry and why.
				//
				// force is the way out, and it exists because without one a
				// single edited timeout -- the most natural edit there is to
				// these entries -- left detach, detach --all and watch all
				// failing, every entry installed, and no supported way to
				// remove them. Refusing is right; refusing with no override
				// is a trap.
				if !force {
					return 0, nil, &ErrModified{Event: event, Detail: detail}
				}
				left = append(left, Modified{Event: event, Detail: detail})
			}
			removed++
		}

		if removed > 0 {
			// An event we emptied loses its key rather than keeping an empty
			// array. watch creates the key, so leaving it behind means detach
			// does not keep its own promise to leave everything else as found:
			// on a file with no hooks at all, watch-then-detach left a hooks
			// object and one empty key per installed event.
			if len(out) == 0 {
				if err := doc.RemoveHookEvent(event); err != nil {
					return 0, nil, err
				}
			} else if err := doc.SetHookEntries(event, out); err != nil {
				return 0, nil, err
			}
			total += removed
		}
	}
	return total, left, nil
}

// Present reports whether our entry for an event is installed as watch
// installs it. This is what the coverage record's hook_entry field means: not
// "some entry exists" but "our entry, with our matcher and our timeout".
func Present(doc *settings.Document, installID, event string) (bool, error) {
	entries, err := doc.HookEntries(event)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if Owner(e, event) == installID && intact(e, event) == "" {
			return true, nil
		}
	}
	return false, nil
}

// ForeignOwners lists the install ids other than ours that claim an entry
// under any of our events, in the order the file presents them and once each.
//
// It decides nothing about ownership: those entries belong to the installs
// that wrote them, and neither watch nor detach touches them. It exists so
// that watch can say they are there, because Claude Code runs every entry with
// one environment and the foreign ones will resolve this store, find another
// install's id in it, and record nothing.
func ForeignOwners(doc *settings.Document, installID string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, event := range Events {
		entries, err := doc.HookEntries(event)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			id := Owner(e, event)
			if id == "" || id == installID || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}

// intact describes how an entry of ours differs from what watch installs, or
// returns "" if it does not. The executable path is deliberately not checked:
// a moved binary is what watch exists to refresh, and refusing to detach over
// it would strand the user.
func intact(raw json.RawMessage, event string) string {
	var g matcherGroup
	if err := json.Unmarshal(raw, &g); err != nil {
		return "entry does not parse as a matcher group"
	}

	switch {
	case hasMatcher(event) && (g.Matcher == nil || *g.Matcher != Matcher):
		got := "absent"
		if g.Matcher != nil {
			got = fmt.Sprintf("%q", *g.Matcher)
		}
		return fmt.Sprintf("matcher is %s, expected %q", got, Matcher)
	case !hasMatcher(event) && g.Matcher != nil && *g.Matcher != "":
		return fmt.Sprintf("matcher is %q, expected none", *g.Matcher)
	}

	if len(g.Hooks) != 1 {
		return fmt.Sprintf("entry carries %d hooks, expected 1", len(g.Hooks))
	}
	h := g.Hooks[0]
	if h.Type != "command" {
		return fmt.Sprintf("hook type is %q, expected \"command\"", h.Type)
	}
	if h.Timeout != Timeout {
		return fmt.Sprintf("timeout is %d, expected %d", h.Timeout, Timeout)
	}
	return ""
}

func sameJSON(a, b json.RawMessage) bool {
	var ca, cb bytes.Buffer
	if json.Compact(&ca, a) != nil || json.Compact(&cb, b) != nil {
		return false
	}
	return bytes.Equal(ca.Bytes(), cb.Bytes())
}

func isInstallID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// shellQuote quotes a path for the shell Claude Code runs hook commands
// through. Paths made of safe characters are left bare so the common case
// reads plainly in the file.
func shellQuote(s string) string {
	safe := s != "" && strings.IndexFunc(s, func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return false
		case r == '/', r == '.', r == '_', r == '-', r == '+', r == ':', r == '@', r == '%':
			return false
		}
		return true
	}) < 0
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
