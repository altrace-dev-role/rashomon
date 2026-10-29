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
		{Owner: OwnerSteer, Cite: CiteSection1, Text: "None blocks a call unless you set a pattern to stop for a project."},
		{Owner: OwnerRecord, Cite: CiteSection1, Text: "Needs Claude Code 2.1.280 or later. An event your version lacks never fires; rashomon status counts the sessions each one fired in. In an interactive session Claude Code runs these only in folders you have trusted; rashomon status says when a session was not recorded."},
		{Owner: OwnerBreadth, Cite: CiteSection1, Text: "Programs built on the Claude Agent SDK on this machine also run these entries unless they set settingSources: []. rashomon records them only if you answer y to the second question below, and adds no notes for their agents unless you run `rashomon steer on --surface sdk`."},

		{Owner: OwnerRecord, Cite: CiteSection2, Text: "STORED: each command the agent runs, as typed, up to 64 KiB (longer: first 48 KiB and last 16 KiB; over 256 KiB, not stored), with the secret values rashomon recognises masked (the listed token formats, and values after a listed key or flag or in a URL's user part); its exit code; the files it names and writes; which instruction files loaded (a digest of each); a keyed digest of each claim sentence it checks in the agent's final message (never the sentence); permission prompts, denials and their reasons; interrupts; task subjects (the agent's one-line task titles, at most 200 bytes, masked); a copy of each project spec file (.claude/rashomon/spec.json) whose rules it applied; each prompt's length; background commands (first 1,000 characters); the model; turn ends and their cost. Whatever else a command contains is stored with it: file contents or data in a heredoc or inline script, and a secret in another format or another position, for example a bare hex or base64 token in a URL path or an unlisted header (rashomon masks the secret formats listed here; it is not a secrets scanner; rashomon forget --commands removes command text)."},
		{Owner: OwnerVerify, Cite: CiteSection2, Text: "READ IN MEMORY, NEVER STORED: the output of build, test, lint and scan commands; the text of file edits and the file before a Write (up to 1 MiB); failure messages; the agent's final message, subagent replies and compaction summaries; the first 1 MiB of each instruction file; Claude Code's transcript files for this project (turn cost at each turn end; rejections and interrupts at session start). rashomon keeps only what it concluded, and the claim-sentence digests above."},
		{Owner: OwnerRecord, Cite: CiteSection2, Text: "Never stored: your prompts, tool output, the agent's messages (apart from task subjects), files the agent reads or edits (apart from what a command itself holds, and those spec copies), and secret values the masker recognises. Your prompts are never stored; rashomon records only each prompt's length and does not examine its text unless you turn on record.prompt_class (off). At most 512 MiB and 180 days."},
		{Owner: OwnerRecord, Cite: CiteSection2, Text: "rashomon itself makes no network connection and sends no telemetry. The notes it adds become part of the conversation Claude Code sends to your model provider and keeps in its own transcripts."},
		{Owner: OwnerMeasure, Cite: CiteSection2, Text: "Each hook runs as two short processes, a supervisor and the recorder (a plugin install adds a small shell launcher). Budget at the median: about 11 MiB and 10.5 to 13.5 ms together for each hook of a tool call. A tool call runs about three hooks; their peaks add up to at most 32 MiB (1.1.0: two processes, about 21.5 MiB). Large tool outputs up to about 30 MiB, never more than 64 MiB; session start can add up to about 0.4 s (p95) before Claude's first reply. These are budgets, not yet measurements: rashomon disclose --cost shows this machine."},

		{Owner: OwnerSteer, Cite: CiteSection3, Text: "Besides recording, rashomon adds short factual notes to the agent's context when its record shows something the agent may not have in view. Examples: a test that passed earlier in the session and fails now, with the files written in between; a task marked done with no passing test run after its writes; a fact your project's spec declares, at the first failure it applies to. `rashomon steer explain` lists each pattern and what it says. Every note is shown to you at the moment it is added, and kept in `rashomon steer log` for as long as its session is kept. Notes are on from the first session that loads these entries."},
		{Owner: OwnerSteer, Cite: CiteSection3, Text: "`rashomon steer off --project` turns them off in one project, `rashomon steer off --user` everywhere, and they stay off. rashomon never blocks a call or a task, and never asks you to confirm one, unless you set a pattern to `stop` for a project with `rashomon steer set`. rashomon does not write your CLAUDE.md, memory files, repository files or any settings key other than its own hook entries."},
		{Owner: OwnerVerify, Cite: CiteSection3, Text: "When the agent says work is done or passing and the record does not show it, rashomon adds a short factual note to the agent's view at the end of that turn, or with your next prompt. You see every note it adds. A spec file in a repository only flags to you, and counts in none of your numbers, until you run `rashomon spec trust`. A spec your administrator manages applies as trusted; when one is present, this screen names it."},
		{Owner: OwnerSteer, Cite: CiteSection3, Text: "A note looks like this:\n  rashomon record: go test exited 0 earlier in this session (6 min\n  ago) and exited 1 on this run. Files written between the two runs:\n  'internal/a.go', 'internal/b.go'."},

		{Owner: OwnerLearn, Cite: CiteSection4, Text: "In each project it counts failures that recur and the change that made the same call succeed afterwards, and keeps this under <root>/learn. At session start it reads Claude Code's transcript files for this project to find rejections and interrupts."},
		{Owner: OwnerLearn, Cite: CiteSection4, Text: "Nothing it learns is shown to your agent until you approve it with `rashomon learn approve`. It never learns credentials, identities, hosts, regions, endpoints or certificate settings. Turn it off with `rashomon learn off --user`."},
		{Owner: OwnerMeasure, Cite: CiteSection4, Text: "rashomon keeps a local tally of how often each note it gives the agent matched what the record showed next. A note type that is wrong too often is switched to show only to you, and rashomon tells you when that happens. `rashomon scorecard` summarises your own record. Nothing is sent anywhere."},

		{Owner: OwnerSteer, Cite: CiteSection5, Text: "In an interactive session, every note the agent gets, at the same moment, as a line starting \"※ rashomon told the agent:\" (in claude -p runs, only in rashomon steer log)."},
		{Owner: OwnerRecord, Cite: CiteSection5, Text: "At the end of a turn that edited files or ran a test, build or lint, one line says what the record shows: which checks passed after the last edit, and what followed any note. Off, and it stays off: rashomon summary off --user"},

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
