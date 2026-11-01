# PLUGIN-CONSENT1 (B287): enabling the plugin is that route's act of
# consent; its own SessionStart entry records it before the probe syncs the
# markers, so the first session's notes reach the model, and tells the person
# once, naming the plugin's own binary.
P = "cmd/rashomon/plugin_consent.go"
MAIN = "cmd/rashomon/main.go"
J = "plugin/.claude-plugin/plugin.json"

m("PC-1 the plugin's start records no consent", MAIN,
  "lead := pluginConsent(phase, payload, args, st.Root())", "lead := \"\"", "TestPC1_")
m("PC-1 a settings entry's start consents too", P,
  "if phase != store.PhaseStart || payload == nil || !briefFromPlugin(args) {",
  "if phase != store.PhaseStart || payload == nil {", "TestPC1_")
m("PC-1 every session consents again", P,
  "if err != nil || consent.Covers(recs) {", "if err != nil {", "TestPC1_")
m("PC-1 a watch consent makes the enable an upgrade", P,
  "ok && last.Via == consent.ViaPluginEnable {", "ok {", "TestPC1_")
m("PC-1 a plugin update is a fresh enable", P,
  "via = consent.ViaUpgradeNotice", "via = consent.ViaPluginEnable", "TestPC1_")
m("PC-1 the record hashes the line, not the description", P,
  "consent.TextSHA256([]byte(disclose.PluginDescription))", "consent.TextSHA256([]byte(disclose.PluginOnLine))", "TestPC1_")
m("PC-1 the lines name a rashomon on PATH", P,
  "bin := filepath.Join(filepath.Dir(exe), install.PluginName)", "bin := install.PluginName", "TestPC1_|TestPC1b_")
m("PC-1 a quoted path takes a ~", P,
  "err == nil && shown == bin && strings.HasPrefix", "err == nil && strings.HasPrefix", "TestPC1b_")
m("PC-1 plugin.json says less than the disclosure", J,
  "disabling it in /plugin stops both.", "disabling it in /plugin stops recording.", "TestPC1c_")
