package disclose

// Cite values a Paragraph carries. A layer paragraph cites the screen
// section it is printed in (CiteSection1 to CiteSection5); a variant cites
// the variant it is the template of. The consent screen prints a layer
// paragraph's text verbatim, wrapped to the width, or line for line when the
// text holds its own line feeds; it renders a variant only when that state
// holds, filling the template's {slots} (Technical spec 7.2.3).
const (
	CiteSection1 = "screen:1"
	CiteSection2 = "screen:2"
	CiteSection3 = "screen:3"
	CiteSection4 = "screen:4"
	CiteSection5 = "screen:5"

	// CiteChangedSince is the CHANGED SINCE YOU LAST AGREED block, shown
	// when the consent on file is for an older scope. Slots: {version},
	// {scope}, {current}.
	CiteChangedSince = "variant:changed_since"
)

// Paragraph owners: the layer whose capability a paragraph discloses.
const (
	OwnerRecord  = "record"
	OwnerSteer   = "steer"
	OwnerVerify  = "verify"
	OwnerLearn   = "learn"
	OwnerMeasure = "measure"
	OwnerBreadth = "breadth"
	OwnerScreen  = "screen"
	OwnerTrust   = "trust"
)

// defaultParagraphs is the consent screen's text, owner by owner, in print
// order within each section. Everything the screen computes -- the version,
// the paths, the install id, the entry list and its time limits, the store's
// state -- is rendered around these, never written into them.
func defaultParagraphs() []Paragraph {
	return []Paragraph{
		{Owner: OwnerSteer, Cite: CiteSection1, Text: "None blocks a call."},
		{Owner: OwnerRecord, Cite: CiteSection1, Text: "Needs Claude Code 2.1.280 or later. An event your version lacks never fires. In an interactive session Claude Code runs these only in folders you have trusted."},
		{Owner: OwnerBreadth, Cite: CiteSection1, Text: "Programs built on the Claude Agent SDK on this machine also run these entries unless they set settingSources: []. rashomon records them only if you answer y to the second question below, and adds no notes for their agents."},

		{Owner: OwnerRecord, Cite: CiteSection2, Text: "STORED: for each tool call, the tool; which agent made it; the program a command runs, its argument count and a keyed digest of its input (never its text); the hostnames it names; the working directory and project; the transcript path; the permission mode; the file an edit names; the files a shell line may write (those paths in the clear at record scope v5; keyed digests at v4, the default, which `rashomon status` shows); how it ended (outcome, exit code, interrupted or not, duration); and the time each hook event first ran in a session. For a test or build line: the runner it ran in and the pipe stage that hid its exit status, each one word from a closed list the binary ships (never a word of the command); for a gate run, one closed word that classifies its output (a passing or failing summary, a failure line, an empty run, or none found), and not the output, which is read in memory; one bit, when true, that says the hook could not read where a shell line started because a leading cd was one it does not read (never the directory it names); in a note's steer line, where the outcome it rests on came from (the exit status or the output's summary); and, when a prompt's preflight finds no spec, one small hint that holds the turn's id and a keyed digest of the directory it looked from, nothing else. At each turn end: the state of each test or build check, a keyed digest of each claim sentence it checks in the agent's final message (never the sentence), and the digest of the project spec (.claude/rashomon/spec.json) whose rules it applied. Each note it adds, word for word, except a request's preflight note at record scope v4, which keeps a count of the paths it named and keyed digests of them. Your on and off switches (config.json), and for a project's switch (summary_projects, steer_projects) the project's directory."},
		{Owner: OwnerVerify, Cite: CiteSection2, Text: "READ IN MEMORY, NEVER STORED: the output of build, test, lint and scan commands; the text of file edits and the file before a Write (up to 1 MiB); failure messages; the agent's final message and subagent replies; Claude Code's transcript files for this project. rashomon keeps only what it concluded, and the claim-sentence digests above."},
		{Owner: OwnerRecord, Cite: CiteSection2, Text: "Never stored: your prompts, command text, tool output, the agent's messages, task subjects, files the agent reads or edits, and spec files. Your prompts are never stored: rashomon reads each in memory to pick project facts for a note and keeps only, in the note's steer line, the project paths it names at record scope v5, or only their count and keyed digests at v4 (`rashomon status` shows the scope). At most 512 MiB by default."},
		{Owner: OwnerRecord, Cite: CiteSection2, Text: "rashomon itself makes no network connection and sends no telemetry. The notes it adds become part of the conversation Claude Code sends to your model provider and keeps in its own transcripts."},
		{Owner: OwnerMeasure, Cite: CiteSection2, Text: "Each hook runs as two short processes, a supervisor and the recorder (a plugin install adds a small shell launcher). Budget at the median: about 11 MiB and 10.5 to 13.5 ms together for each hook of a tool call. A tool call runs about three hooks; their peaks add up to at most 32 MiB (1.1.0: two processes, about 21.5 MiB). Large tool outputs up to about 30 MiB, never more than 64 MiB; session start can add up to about 0.4 s (p95) before Claude's first reply. These are budgets, not yet measurements."},

		{Owner: OwnerSteer, Cite: CiteSection3, Text: "Besides recording, rashomon adds short factual notes to the agent's context when its record shows something the agent may not have in view. Examples: with your prompt, the spec rules and checks that apply to the paths it names; a test that passed earlier in the session and fails now, with the files written in between (Claude Code 2.1.304 and later); an edit after a test passed, with no later run of it. Every note is shown to you at the moment it is added, and kept in the record for as long as its session is kept. Notes are on from the first session that loads these entries. At a turn end, rashomon reads Claude Code's transcript in memory, and stores none of it, to check that each note it sent appears there. If a note is missing at two turn ends, one small file holding only the Claude Code version is stored, and notes for that hook event stop until a delivery check passes or Claude Code is newer; one line per session then tells you a note was built and not sent. `rashomon steer explain` says why the last note was or was not printed, and `rashomon steer log` lists each decision made."},
		{Owner: OwnerSteer, Cite: CiteSection3, Text: "`rashomon steer off --user` stops the notes to your agent, and `rashomon steer off --project` for this project; what a note would have said is shown to you as a flag where the call has room for one (one flag per call, five per prompt), and nothing is shown on an event that has no way to carry one; recording goes on, and the line at the end of the turn still prints. `rashomon detach` removes the hook entries, and the notes stop with them. rashomon never blocks a call or a task, and never asks you to confirm one. rashomon does not write your CLAUDE.md, memory files, repository files or any settings key other than its own hook entries."},
		{Owner: OwnerVerify, Cite: CiteSection3, Text: "When the agent says work is done or passing and the record does not show it, the line at the end of the turn says so. A spec file in a repository only flags to you until you run `rashomon spec trust`. A spec your administrator manages applies as trusted."},
		{Owner: OwnerSteer, Cite: CiteSection3, Text: "A note looks like this:\n  rashomon record: gate 'go' exited 0 earlier in this session (6 min\n  ago) and exited 1 on this run. Files written between the two runs:\n  'internal/a.go' and 2 more."},

		{Owner: OwnerLearn, Cite: CiteSection4, Text: "This build learns nothing by itself: no hook counts failures or learns from Claude Code's transcripts. `rashomon learn import FILE` adds a shared lessons file's rows to this project's lessons under <root>/learn."},
		{Owner: OwnerLearn, Cite: CiteSection4, Text: "An approved lesson is sent with a prompt, at most one per prompt and each lesson once a session: as the last line of that prompt's preflight note when your prompt names a path the lesson names, and otherwise with the next prompt that carries no preflight note. If steering is off or you have not agreed to steering notes, it is shown to you and not sent. `rashomon learn approve ID` marks one approved after you type its one-time code, and `rashomon learn withdraw ID` stops one the same way. A write to the lesson files that this program did not make is detected, and no lesson is sent, unless the writer can also read the store key: the same limit as `rashomon spec trust`."},
		{Owner: OwnerMeasure, Cite: CiteSection4, Text: "This build keeps no tally of how its notes turned out; `rashomon scorecard` counts what it decided, by pattern. Nothing is sent anywhere."},

		{Owner: OwnerSteer, Cite: CiteSection5, Text: "In an interactive session, every note the agent gets, at the same moment, as a line starting \"※ rashomon told the agent:\"."},
		{Owner: OwnerRecord, Cite: CiteSection5, Text: "At the end of a turn that edited files or ran a test, build or lint, one line says what the record shows: which checks passed after the last edit, and the notes the agent was given. `rashomon summary off --user` stops its value line for you, and `rashomon summary off --project` for this project."},
		{Owner: OwnerRecord, Cite: CiteSection5, Text: "When a session starts or is cleared, one block shown only to you says what the project holds: which instruction files (CLAUDE.md, AGENTS.md) and build manifests (go.mod, package.json and the like) exist, with their sizes, the module line of go.mod, the test command, the project spec and its task, and the names of the Go package directories. It reads those at that moment and stores none of it; the agent is not shown the block, and no instruction file's text is read. `rashomon brief` prints the same block."},

		{Owner: OwnerScreen, Cite: CiteChangedSince, Text: "CHANGED SINCE YOU LAST AGREED ({version}, scope {scope}): this build asks for scope {current}. Everything it now does is below."},
	}
}

// UnitParagraph is one consent unit's screen text (card 4.2): what a y to
// `rashomon consent <unit>` allows. It is not on the install screen, so the
// install text_sha256 and ScopeVersion do not move with it.
type UnitParagraph struct {
	Owner string `json:"owner"`
	Unit  string `json:"unit"`
	Text  string `json:"text"`
	Cite  string `json:"cite"`
}

// CitePFSH1 is the requirement the unit paragraphs answer: units are off
// until the user turns each on.
const CitePFSH1 = "prd:PF-SH1"

// defaultUnits is the unit screens' text, in consent.Units' order, for the
// units a command in this release uses. share.lessons and share.bodies have
// none until a command builds them: a grant to a bundle nobody has been
// shown would not be consent to it.
func defaultUnits() []UnitParagraph {
	return []UnitParagraph{
		{Owner: OwnerTrust, Unit: "bodies", Cite: CitePFSH1, Text: "Lets rashomon copy each Claude Code transcript of a project you list with `rashomon bodies on` into <root>/bodies when its session ends, mode 0600, within the cap you set (2 GiB unless you say otherwise), removing the oldest copy first. The copies stay on this machine. `rashomon forget --bodies` removes a project's copies; `rashomon consent bodies off` stops new ones."},
		{Owner: OwnerTrust, Unit: "share.outcomes", Cite: CitePFSH1, Text: "Lets `rashomon share build outcomes --out FILE` write a file of counts: how many notes rashomon gave the agent, by pattern, and the span of time they cover, any count under five written as <5, with a keyed digest of this install that names no person or machine. It prints the whole file and its sha256 before it writes it. rashomon sends nothing; the file goes only where you take it. `rashomon consent share.outcomes off` withdraws this."},
	}
}
