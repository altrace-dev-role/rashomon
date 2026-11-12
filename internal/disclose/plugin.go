package disclose

// PluginDescription is the plugin's description: the text a person reads in
// Claude Code's /plugin before enabling it. Enabling the plugin is the plugin
// route's act of consent, as a y on the screen is watch's (PLUGIN-CONSENT1,
// D160), so it says what a yes on the short screen says: what is recorded,
// what never is, the notes and where they go, and how to turn it off. Its
// list says "including:", so it ends with where the full one is: the README
// section that quotes the full screen's STORED paragraph (TestPC1e). It says
// nothing of Agent SDK programs: the plugin route asks no SDK question, so it
// records none (SDK-GATE1). It names the platform graph's writer the plugin
// runs in the background in a Go repository, and what it writes there
// (PW1b-ON-230), and what the store keeps of each Go edit in a repository
// indexed with `rashomon graph index` (W-M2.11 step 1d).
// plugin/.claude-plugin/plugin.json and
// .claude-plugin/marketplace.json carry it byte for byte (TestPC1c), and a
// plugin_enable record's text_sha256 is its hash.
const PluginDescription = "Records each Claude Code tool call on this machine, in every project, once enabled, including: " +
	"the tool, the program a command runs (never its text), the paths it edits or may write and an edited " +
	"file's SHA-256, the hosts it names, the directory it ran in, the project's root and the transcript " +
	"path, and how it ended. In a Git repository whose root holds go.mod, it also runs pgraph in the " +
	"background at session start and each time Claude finishes a reply: the go tool, offline, loads the Go code, and the names of its " +
	"packages, functions and calls, with each file's SHA-256, go to .pgraph/ there (git-ignored; delete it to " +
	"forget them). In a repository indexed with rashomon graph index, it also keeps what each Go edit " +
	"declares (names, doc lines, calls and imports) in its store until purged. Adds short factual notes " +
	"to the agent's context, such as a test that passed earlier and fails now; you see each one, and they go " +
	"to your model provider with the conversation. Never stores your prompts, command text, tool output, " +
	"file contents or a spec file's text (of a spec, only its digest and its rules' ids, and, where Go " +
	"edits are kept, the paths and patterns of the rules each edit is checked against), and sends nothing " +
	"itself. Ships disabled: enabling it is your consent to the above, and " +
	"disabling it in /plugin stops all of it. Full list of what it keeps: " +
	"https://github.com/altrace-dev-role/rashomon#what-it-records-and-what-it-never-stores"

// PluginOnLine is the line the plugin's first session after an enable, or
// after an update that asked for more, shows the person: what is on and how
// to turn it off, in 80 columns or fewer (PLUGIN-CONSENT1).
const PluginOnLine = "rashomon is on: records tool calls, adds notes to the agent. Off: /plugin"
