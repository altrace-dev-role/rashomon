package hook

import (
	"time"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Resolution is the configuration state as this process sees it, at the moment
// it looks. It is recorded, never recomputed later: a run is judged by what was
// true while it ran, so that detach cannot rewrite the past.
type Resolution struct {
	HookEntry string
	Probe     string
}

// Resolve reads the user's settings file, this process's own plugin identity,
// and the probe marker.
//
// hook_entry is "present_settings" or "present_plugin" only when our entry
// for event is installed as watch installs it, or the plugin declares it,
// respectively -- never a bare "present": the two are different strengths of
// evidence and a coverage record that collapsed them would let the weaker one
// borrow the stronger one's credibility. An entry that exists but has been
// narrowed to "Bash" is "absent" for coverage purposes, because that is what
// it is: a config under which most tool calls produce no record.
//
// The settings check is a fresh file read, trustworthy because Claude Code's
// own file watcher keeps hook edits current for a running session. The plugin
// check is NOT a fresh read of enabledPlugins -- see install.PluginPresent for
// why that would only prove "the file currently says enabled" and not "this
// session's hook set includes it" -- it is this process asking whether it is
// itself the plugin's own hook binary. That is evidence from where this
// process runs rather than a config file's opinion of itself -- stronger, but
// not proof: the same binary run by hand from inside the plugin says the same.
//
// This used to be a narrower claim: a session started under a settings file
// or a CLAUDE_CONFIG_DIR the probe does not read "records its declarations
// perfectly well and still reports hook_entry_absent... that is the right
// failure direction -- it under-claims -- but it means a temporary or
// side-loaded install cannot produce a verified report, by construction."
// That predicate has not been relaxed; it has been given a second thing it
// can honestly confirm. present_plugin means the probe read evidence from
// where this process runs -- self-identification, not a config file's opinion
// about itself -- that this session's hooks include ours. Evidence, not
// proof, for the reason above: a manual run from inside the plugin reads the
// same. A plugin install that merely SITS ON DISK, with no process running
// from it, still never reads present_plugin.
// The under-claim survives; what widened is what the probe is able to read.
//
// Every failure that lasts resolves to "unknown". That is a real answer and a
// worse one than either present value, which is the point: a handler that
// cannot read its own configuration must not claim to know it. A failure that
// does not last is a different thing and is retried; see loadSettings.
func Resolve(st *store.Store, sessionID, event string) Resolution {
	res := Resolution{HookEntry: store.EntryUnknown, Probe: store.ProbeUnknown}

	settingsOK, settingsPresent := false, false
	if path, err := settings.UserPath(); err == nil {
		if doc, err := loadSettings(path); err == nil {
			if present, err := install.Present(doc, st.InstallID(), event); err == nil {
				settingsOK, settingsPresent = true, present
			}
		}
	}
	pluginOK, pluginPresent := false, false
	if present, err := install.PluginPresent(event); err == nil {
		pluginOK, pluginPresent = true, present
	}

	switch {
	case settingsPresent:
		res.HookEntry = store.EntryPresentSettings
	case pluginPresent:
		res.HookEntry = store.EntryPresentPlugin
	case settingsOK && pluginOK:
		// Absent requires BOTH checks to have resolved, not either one: a
		// settings.json read that failed persistently while the plugin check
		// happened to resolve (negatively) still leaves this handler unable to
		// say whether OUR settings entry is there, and "absent" is a claim
		// about that specifically. Only when neither origin could be found,
		// with both actually checked, is the config genuinely absent rather
		// than partly unread.
		res.HookEntry = store.EntryAbsent
	}

	if fresh, err := st.ProbeFresh(sessionID); err == nil {
		res.Probe = store.ProbeAbsent
		if fresh {
			res.Probe = store.ProbeFresh
		}
	}
	return res
}

// How far loadSettings goes before it gives up.
const (
	settingsAttempts   = 3
	settingsRetryPause = 3 * time.Millisecond
)

// loadSettings reads the settings document, retrying a read that failed.
//
// Claude Code and other tools rewrite this file during normal use, and a
// rewrite that is not atomic shows up here, for the instant it lasts, as a
// failed read or a parse of a half-written file. Resolving "unknown" on that
// instant costs the run and not just the call: one unverified call is enough
// to make the whole run unverified in report.
//
// The bound is as load-bearing as the retry. A file that is genuinely
// unreadable has to reach "unknown" rather than hold the hook open, and the
// first attempt is not preceded by a wait, so a healthy read pays nothing for
// this.
func loadSettings(path string) (*settings.Document, error) {
	doc, err := settings.Load(path)
	for attempt := 1; err != nil && attempt < settingsAttempts; attempt++ {
		time.Sleep(settingsRetryPause)
		doc, err = settings.Load(path)
	}
	return doc, err
}

// entryEvent is the installed entry whose state a phase's coverage record
// reports. The post phase reports the PostToolUse entry: reading the recorder's
// entry there would let a run claim its executions were covered on the strength
// of an entry that records none of them.
func entryEvent(phase string) string {
	if phase == store.PhasePost {
		return install.EventPostToolUse
	}
	return install.EventPreToolUse
}

// BuildCoverage assembles the coverage record for one hook invocation. A
// failure reason from the invocation itself takes precedence over anything
// resolved from configuration; after that, the entry's state outranks the
// probe's, because an absent entry explains an absent probe and not the other
// way round.
func BuildCoverage(st *store.Store, sessionID, phase, reason, cwd string, now time.Time) store.Coverage {
	res := Resolve(st, sessionID, entryEvent(phase))
	if phase == store.PhaseStart {
		// This invocation is the probe; it has just marked itself.
		res.Probe = store.ProbeFresh
	}

	if reason == "" {
		switch {
		case res.HookEntry == store.EntryAbsent:
			reason = store.ReasonHookEntryAbsent
		case res.HookEntry == store.EntryUnknown:
			reason = store.ReasonHookEntryUnresolved
		case res.Probe == store.ProbeAbsent:
			reason = store.ReasonProbeAbsent
		case res.Probe == store.ProbeUnknown:
			reason = store.ReasonProbeUnresolved
		}
	}

	state := store.StateVerified
	if reason != "" {
		state = store.StateUnverified
	}
	return store.Coverage{
		Type:          store.TypeCoverage,
		SchemaVersion: store.SchemaVersion,
		RecordedAtMS:  now.UnixMilli(),
		SessionID:     sessionID,
		InstallID:     st.InstallID(),
		Phase:         phase,
		State:         state,
		Reason:        nilIfEmpty(reason),
		HookEntry:     res.HookEntry,
		Probe:         res.Probe,
		CWD:           cwd,
	}
}
