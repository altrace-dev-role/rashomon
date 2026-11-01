package main

import "github.com/altrace-dev-role/rashomon/internal/disclose"

// pluginLead is the plugin's first-session line until PLUGIN-CONSENT1 names
// the plugin's own binary.
func pluginLead(string) string { return disclose.PluginOnLine }
