package disclose

// PluginDescription is the plugin's description: the text a person reads in
// Claude Code's /plugin before enabling it. Enabling the plugin is the plugin
// route's act of consent, as a y on the screen is watch's (PLUGIN-CONSENT1,
// D160), so it says what a yes on the short screen says: what is recorded,
// what never is, the notes and where they go, and how to turn it off.
// plugin/.claude-plugin/plugin.json and .claude-plugin/marketplace.json carry
// it byte for byte (TestPC1c), and a plugin_enable record's text_sha256 is
// its hash.
const PluginDescription = "Records each Claude Code tool call on this machine, in every project, once enabled, including: " +
	"the tool, the program a command runs (never its text), the paths it edits or may write and an edited " +
	"file's SHA-256, the hosts it names, the directory it ran in, and how it ended. Adds short factual notes " +
	"to the agent's context, such as a test that passed earlier and fails now; you see each one, and they go " +
	"to your model provider with the conversation. Never stores your prompts, command text, tool output or " +
	"file contents, and sends nothing itself. Ships disabled: enabling it is your consent to the above, and " +
	"disabling it in /plugin stops both."

// PluginOnLine is the line the plugin's first session after an enable, or
// after an update that asked for more, shows the person: what is on and how
// to turn it off, in 80 columns or fewer (PLUGIN-CONSENT1).
const PluginOnLine = "rashomon is on: records tool calls, adds notes to the agent. Off: /plugin"
