package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/brief"
	"github.com/altrace-dev-role/rashomon/internal/consent"
	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// pluginConsent is the plugin route's act of consent (PLUGIN-CONSENT1). The
// person enabled the plugin in /plugin after reading
// disclose.PluginDescription, which is to the plugin what a y on the short
// screen is to watch (D160): a plugin user has no rashomon on PATH and no
// terminal screen to answer. The plugin's own SessionStart entry, and only
// it (probe start with a payload, briefFromPlugin: no --install marker, and
// the plugin fires, D175), appends one record when consent.json does not
// cover this build: upgrade_notice when the last act of consent on file was
// a plugin enable, so a plugin update is noticed, not taken as a fresh enable
// (design spec, upgrades); plugin_enable otherwise, a watch-route consent
// included, since this person's act was the enable. Append writes both
// markers, scope v5 and this build's steer disclosure, before the probe syncs
// them, so the session's notes reach the model. It returns the lines the person sees (pluginLead), or "" when
// nothing was written. A settings entry never comes here: watch's terminal
// consent stays that route's act.
func pluginConsent(phase string, payload *hookio.Payload, args []string, root string) string {
	if phase != store.PhaseStart || payload == nil || !briefFromPlugin(args) {
		return ""
	}
	recs, err := consent.Read(root)
	if err != nil || consent.Covers(recs) {
		return ""
	}
	via := consent.ViaPluginEnable
	if last, ok := consent.LastAccepted(recs); ok && last.Via == consent.ViaPluginEnable {
		via = consent.ViaUpgradeNotice
	}
	exe, err := install.Executable()
	if err != nil {
		return ""
	}
	exeSHA, err := fileSHA256(exe)
	if err != nil {
		return ""
	}
	id, err := consent.NewInvocationID()
	if err != nil {
		return ""
	}
	err = consent.Append(root, consent.Record{
		V:               consent.RecordVersion,
		ScopeVersion:    disclose.ScopeVersion,
		RecordScope:     consent.RecordScopeV5,
		SteerDisclosure: steer.DisclosureVersion,
		Steer:           consent.SteerOn,
		TextSHA256:      consent.TextSHA256([]byte(disclose.PluginDescription)),
		BinaryVersion:   buildVersion(),
		BinarySHA256:    exeSHA,
		Via:             via,
		InvocationID:    id,
		At:              time.Now().UnixMilli(),
	})
	// A line on file with a marker unwritten is consent all the same; the
	// probe's own sync writes the marker from it next.
	if err != nil && !errors.Is(err, consent.ErrScopeMarker) && !errors.Is(err, consent.ErrSteerMarker) {
		return ""
	}
	return pluginLead(exe)
}

// pluginLead is what the plugin consent shows the person, user channel only
// (PLUGIN-CONSENT1, the DE's addition 2): what is on and how to turn it off,
// then where the record is read, the plugin's report command and, for a
// terminal, which has no rashomon on PATH (B287), the plugin's own binary,
// home as ~ where the path needs no quoting. Every line fits 80 columns but
// the binary's, a path printed whole.
func pluginLead(exe string) string {
	bin := filepath.Join(filepath.Dir(exe), install.PluginName)
	shown := install.ShellQuote(bin)
	if home, err := os.UserHomeDir(); err == nil && shown == bin && strings.HasPrefix(bin, home+"/") {
		shown = "~" + bin[len(home):]
	}
	return disclose.PluginOnLine + "\nSee what it recorded with " + brief.PluginReport + ", or in a terminal:\n  " + shown + " report"
}
