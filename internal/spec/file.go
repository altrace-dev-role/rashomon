package spec

// The spec file's schema, version 1 (archive 20.2.8). The field names, json
// tags and comments are the archive's; where a type differs from the text
// (Rule.Kind is a RuleKind, the package's one vocabulary for the five kinds,
// not a bare string) the difference is a strengthening and the encoding is
// the same.

// File is one spec.json as written (20.2.8). It is never decoded into
// directly: Parse reads a document tree, checks it, and fills a File, so no
// default of encoding/json (case-insensitive keys, last duplicate wins,
// null as zero) reaches a rule.
type File struct {
	RashomonSpec int        `json:"rashomon_spec"` // must be 1
	Gates        []Gate     `json:"gates,omitempty"`
	CICommands   [][]string `json:"ci_commands,omitempty"`
	Rules        []Rule     `json:"rules,omitempty"`
	Facts        []Fact     `json:"facts,omitempty"` // declared facts; owner VERIFY (FD-1, recommended)
}

// Fact is a declared environment fact: the mechanism the project uses, never a
// secret value. P-42 builds it.
type Fact struct {
	ID        string      `json:"id"`               // RuleID charset
	Kind      string      `json:"kind"`             // aws_profile | aws_region | kube_context | kube_namespace | package_index_host | env_var_name | sign_in_step
	Programs  []string    `json:"programs"`         // closed family programs it applies to: aws, sam, cdk, kubectl, helm, npm, pip, gh, …
	Target    *FactTarget `json:"target,omitempty"` // {kind, id}: the 10.2.6 target key and value this fact is about
	Classes   []string    `json:"classes"`          // failure classes it fires on (10.2.5)
	Value     string      `json:"value"`            // STEER's FactValue allowlist; an identifier, no spaces
	Mechanism string      `json:"mechanism"`        // closed: profile | region | context | namespace | index | env var name | sign-in step
	Why       string      `json:"why,omitempty"`    // user-only; never model-visible
}

// FactTarget names what a fact is about: a 10.2.6 target key and its value.
type FactTarget struct {
	Kind string `json:"kind"` // aws_account | aws_region | endpoint_host | kube_context | gcp_project | azure_subscription
	ID   string `json:"id"`
}

// Gate is one declared verification gate. A parsed Gate carries its defaults
// (Match "prefix", Dir ".", Required true) so a consumer never reads nil as a
// third state.
type Gate struct {
	ID           string            `json:"id"`
	Kind         string            `json:"kind"`                 // test|build|lint|typecheck|scan|custom
	Argv         []string          `json:"argv"`                 // program and plain words at command position
	Match        string            `json:"match,omitempty"`      // "prefix" (default) | "exact"
	Aliases      [][]string        `json:"aliases,omitempty"`    // approved alternative spellings: make verify for tools/verify.py
	Dir          string            `json:"dir,omitempty"`        // project-relative start directory; default "."
	AppliesTo    []string          `json:"applies_to,omitempty"` // globs; default: the family's languages
	Required     *bool             `json:"required,omitempty"`   // default true
	PartialFlags []string          `json:"partial_flags,omitempty"`
	SuccessLine  []string          `json:"success_line,omitempty"` // RE2, whole-line
	FailureLine  []string          `json:"failure_line,omitempty"`
	EmptyRunLine []string          `json:"empty_run_line,omitempty"`
	ExitCodes    map[string]string `json:"exit_codes,omitempty"` // "0":"pass", "5":"empty_run", "2":"fail"
}

// Rule is one declared rule. Which fields a rule may carry depends on its
// Kind (ruleFields); Parse refuses a field that belongs to another kind.
type Rule struct {
	ID          string   `json:"id"`
	Kind        RuleKind `json:"kind"` // require_evidence|forbid_code|forbid_command|protect_path|order
	Why         string   `json:"why"`
	HardControl string   `json:"hard_control"` // the real control: a CI job, a scanner gate, CODEOWNERS
	Alternative string   `json:"alternative,omitempty"`

	// forbid_code
	Pattern      string   `json:"pattern,omitempty"`
	Languages    []string `json:"languages,omitempty"`
	Paths        []string `json:"paths,omitempty"`
	ExcludePaths []string `json:"exclude_paths,omitempty"`

	// forbid_command
	Command *CommandMatch `json:"command,omitempty"`

	// protect_path (Paths above)
	Modes []string `json:"modes,omitempty"` // modify|delete|create; default all three

	// require_evidence
	At       []string      `json:"at,omitempty"`       // commit|push|pr|done|subtask
	Before   *CommandMatch `json:"before,omitempty"`   // or a command
	Evidence []string      `json:"evidence,omitempty"` // gate ids, or "kind:<kind>"

	// order: Then only after a qualifying After
	Then          *CommandMatch `json:"then,omitempty"`
	After         *CommandMatch `json:"after,omitempty"`
	AfterGate     string        `json:"after_gate,omitempty"`
	SameRunKey    []string      `json:"same_run_key,omitempty"`   // flag names whose values must match
	InvalidatedBy []string      `json:"invalidated_by,omitempty"` // globs: a write here between After and Then voids After
}

// CommandMatch matches a command line at command position (20.2.8,
// forbid_command).
type CommandMatch struct {
	Argv         []string            `json:"argv"`
	Flags        []string            `json:"flags,omitempty"`         // any present: match
	RequireFlags []string            `json:"require_flags,omitempty"` // argv matches and none present: match
	FlagValues   map[string][]string `json:"flag_values,omitempty"`   // case-insensitive value match
	SQL          []string            `json:"sql,omitempty"`           // statement keywords for psql, mysql, sqlite3, cqlsh, clickhouse-client
}

// The closed value sets of 20.2.8, in the archive's order. They are the
// package's one vocabulary: verify reads these lists, it does not spell its
// own.
var (
	// GateKinds is Gate.Kind.
	GateKinds = []string{"test", "build", "lint", "typecheck", "scan", "custom"}
	// GateMatches is Gate.Match; the first is the default.
	GateMatches = []string{"prefix", "exact"}
	// ExitStates is the value side of Gate.ExitCodes (the archive's example
	// names these three).
	ExitStates = []string{"pass", "empty_run", "fail"}
	// Modes is Rule.Modes; protect_path defaults to all three.
	Modes = []string{"modify", "delete", "create"}
	// Points is Rule.At.
	Points = []string{"commit", "push", "pr", "done", "subtask"}
	// FactKinds is Fact.Kind.
	FactKinds = []string{"aws_profile", "aws_region", "kube_context", "kube_namespace", "package_index_host", "env_var_name", "sign_in_step"}
	// TargetKinds is FactTarget.Kind.
	TargetKinds = []string{"aws_account", "aws_region", "endpoint_host", "kube_context", "gcp_project", "azure_subscription"}
	// SQLPrograms are the programs whose CommandMatch may carry SQL keywords.
	SQLPrograms = []string{"psql", "mysql", "sqlite3", "cqlsh", "clickhouse-client"}
)

// factMechanisms pairs each fact kind with its mechanism word, index for
// index with FactKinds. The archive lists seven mechanisms for seven kinds
// and does not say they pair; Parse reads them as one-to-one. Its last two
// spellings, "env var name" and "sign-in step", carry a space the same
// section forbids in a mechanism, so no fact of those two kinds parses
// until the archive is corrected (see mechanismReason).
var factMechanisms = []string{"profile", "region", "context", "namespace", "index", "env var name", "sign-in step"}

// Caps the archive names are in spec.go. These are the ones it leaves
// unnamed; each bounds a list or string the schema accepts, so no field of
// an untrusted spec is unbounded. All are proposed, pending the archive
// naming them (20.2.8 limits sentence).
const (
	// MaxArgvWords bounds one argv: a gate's, an alias's, a ci command's, a
	// CommandMatch's.
	MaxArgvWords = 32
	// MaxAliases bounds Gate.Aliases.
	MaxAliases = 8
	// MaxListItems bounds every other string list: paths, globs, languages,
	// flags, programs, classes, evidence, regexes per line field, one flag's
	// values.
	MaxListItems = 64
	// MaxWordBytes bounds one word, glob, flag, class, program or value.
	MaxWordBytes = 256
	// MaxFlagValueKeys bounds CommandMatch.FlagValues.
	MaxFlagValueKeys = 16
	// MaxExitCodes bounds Gate.ExitCodes.
	MaxExitCodes = 16
	// MaxFactValueBytes bounds Fact.Value, Fact.Mechanism and FactTarget.ID:
	// the length of STEER's FactValue charset.
	MaxFactValueBytes = 128
	// MaxDepth bounds JSON nesting. The schema's deepest path is seven.
	MaxDepth = 16
	// MaxNodes bounds JSON values in one file, so a 256 KiB file of short
	// values cannot allocate a tree of 100,000 nodes.
	MaxNodes = 20000
	// MaxWhyRunes bounds Rule.Why, Rule.HardControl and Fact.Why (20.2.8:
	// "`why` and `hard_control` at most 300 runes"; Fact.Why is a `why`).
	MaxWhyRunes = 300
	// MaxAlternativeRunes bounds Rule.Alternative (20.2.8).
	MaxAlternativeRunes = 160
	// MaxIDBytes is the id charset's length: ^[a-z0-9][a-z0-9._-]{0,47}$.
	MaxIDBytes = 48
)
