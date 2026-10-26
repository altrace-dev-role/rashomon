package spec

// The schema-version-1 document of archive 20.2.8, as Go types. Every field
// is the archive's, in its order, with its json key. A File that Parse returns
// has passed every cap, charset and closed list the archive names, with the
// defaults applied that it states (Gate.Match "prefix", Gate.Required true,
// Rule.Modes of a protect_path rule all three).

// File is one parsed spec.json (archive 20.2.8, Schema, version 1).
// Invariant: RashomonSpec is 1, len(Gates)+len(CICommands) <= MaxGates,
// len(Rules) <= MaxRules, len(Facts) <= MaxFacts, and ids are unique within
// Gates, Rules and Facts each.
type File struct {
	RashomonSpec int        `json:"rashomon_spec"`
	Gates        []Gate     `json:"gates,omitempty"`
	CICommands   [][]string `json:"ci_commands,omitempty"`
	Rules        []Rule     `json:"rules,omitempty"`
	Facts        []Fact     `json:"facts,omitempty"`
}

// Fact is a declared environment fact: the mechanism the project uses, never a
// secret value (archive 20.2.8, Facts). Kind and Mechanism come from closed
// lists; Why is user-only text, at most MaxWhyRunes runes.
type Fact struct {
	ID        string      `json:"id"`
	Kind      string      `json:"kind"`
	Programs  []string    `json:"programs"`
	Target    *FactTarget `json:"target,omitempty"`
	Classes   []string    `json:"classes"`
	Value     string      `json:"value"`
	Mechanism string      `json:"mechanism"`
	Why       string      `json:"why,omitempty"`
}

// FactTarget names the target a Fact is about: a kind from a closed list and
// an id (archive 20.2.8, Schema).
type FactTarget struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Gate is a declared evidence gate (archive 20.2.8, Schema). Match is
// "prefix" or "exact"; after Parse it is never empty. Required is never nil
// after Parse. The three *Line lists, with Rule.Pattern, are the only regular
// expressions in a File, each within the RE2 bounds of archive 20.2.8.
type Gate struct {
	ID           string            `json:"id"`
	Kind         string            `json:"kind"`
	Argv         []string          `json:"argv"`
	Match        string            `json:"match,omitempty"`
	Aliases      [][]string        `json:"aliases,omitempty"`
	Dir          string            `json:"dir,omitempty"`
	AppliesTo    []string          `json:"applies_to,omitempty"`
	Required     *bool             `json:"required,omitempty"`
	PartialFlags []string          `json:"partial_flags,omitempty"`
	SuccessLine  []string          `json:"success_line,omitempty"`
	FailureLine  []string          `json:"failure_line,omitempty"`
	EmptyRunLine []string          `json:"empty_run_line,omitempty"`
	ExitCodes    map[string]string `json:"exit_codes,omitempty"`
}

// Rule is one rule of the five kinds in archive 20.2.8's table. Why and
// HardControl are required, at most MaxWhyRunes runes each; Alternative is at
// most MaxAlternativeRunes. Pattern is an RE2 expression. Parse does not check
// that a kind carries the fields it needs: the archive is silent on that, and
// the evaluator treats a missing field as undecided.
type Rule struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Why         string `json:"why"`
	HardControl string `json:"hard_control"`
	Alternative string `json:"alternative,omitempty"`

	Pattern      string   `json:"pattern,omitempty"`
	Languages    []string `json:"languages,omitempty"`
	Paths        []string `json:"paths,omitempty"`
	ExcludePaths []string `json:"exclude_paths,omitempty"`

	Command *CommandMatch `json:"command,omitempty"`

	Modes []string `json:"modes,omitempty"`

	At       []string      `json:"at,omitempty"`
	Before   *CommandMatch `json:"before,omitempty"`
	Evidence []string      `json:"evidence,omitempty"`

	Then          *CommandMatch `json:"then,omitempty"`
	After         *CommandMatch `json:"after,omitempty"`
	AfterGate     string        `json:"after_gate,omitempty"`
	SameRunKey    []string      `json:"same_run_key,omitempty"`
	InvalidatedBy []string      `json:"invalidated_by,omitempty"`
}

// CommandMatch is a command at command position: its argv plus the flag and
// SQL conditions of a forbid_command, require_evidence or order rule (archive
// 20.2.8, Schema). Argv is required.
type CommandMatch struct {
	Argv         []string            `json:"argv"`
	Flags        []string            `json:"flags,omitempty"`
	RequireFlags []string            `json:"require_flags,omitempty"`
	FlagValues   map[string][]string `json:"flag_values,omitempty"`
	SQL          []string            `json:"sql,omitempty"`
}
