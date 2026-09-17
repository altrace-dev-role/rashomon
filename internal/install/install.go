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

	"github.com/altrace-dev-role/altrace-attest/internal/settings"
)

// Events we install into. PreToolUse is the recorder. SessionStart and
// SessionEnd are the liveness probe, and they are also what makes "at start
// and at end" in the coverage record literal rather than approximate.
const (
	EventPreToolUse   = "PreToolUse"
	EventSessionStart = "SessionStart"
	EventSessionEnd   = "SessionEnd"
)

// Events in the order they are installed.
var Events = []string{EventPreToolUse, EventSessionStart, EventSessionEnd}

const (
	// Matcher is installed on PreToolUse. A narrower matcher is the classic
	// silent under-count: "Bash" means Edit, Write, WebFetch and every mcp__*
	// call produce no declaration while the report goes on claiming full
	// coverage.
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
		return "", errors.New("install: refusing to install a `go run` temporary binary; build attest and run the built binary")
	}
	return p, nil
}

func subcommand(event string) string {
	switch event {
	case EventPreToolUse:
		return "hook"
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
	// Session events match on source or reason, not tool name; omitting the
	// matcher is the documented way to match every one.
	if event == EventPreToolUse {
		m := Matcher
		g.Matcher = &m
	}
	return g
}

// Entry renders our matcher group as it sits as an item of hooks.<event>,
// indented for that position so it reads as native in the file.
func (s Spec) Entry(event string) json.RawMessage {
	b, err := json.MarshalIndent(s.group(event), settings.Indent(3), "  ")
	if err != nil {
		panic("install: static entry does not marshal: " + err.Error())
	}
	return b
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
		want := spec.Entry(event)

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

func (e *ErrModified) Error() string {
	return fmt.Sprintf("install: our %s entry was modified (%s); not touching it", e.Event, e.Detail)
}

// Remove drops every entry of ours and reports how many. It touches nothing
// else, and it refuses if any entry of ours is not intact.
func Remove(doc *settings.Document, spec Spec) (int, error) {
	total := 0
	for _, event := range Events {
		entries, err := doc.HookEntries(event)
		if err != nil {
			return 0, err
		}

		var (
			out     []json.RawMessage
			removed int
		)
		for _, e := range entries {
			if Owner(e, event) != spec.InstallID {
				out = append(out, e)
				continue
			}
			if detail := intact(e, event); detail != "" {
				return 0, &ErrModified{Event: event, Detail: detail}
			}
			removed++
		}

		if removed > 0 {
			if err := doc.SetHookEntries(event, out); err != nil {
				return 0, err
			}
			total += removed
		}
	}
	return total, nil
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
	case event == EventPreToolUse && (g.Matcher == nil || *g.Matcher != Matcher):
		got := "absent"
		if g.Matcher != nil {
			got = fmt.Sprintf("%q", *g.Matcher)
		}
		return fmt.Sprintf("matcher is %s, expected %q", got, Matcher)
	case event != EventPreToolUse && g.Matcher != nil && *g.Matcher != "":
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
