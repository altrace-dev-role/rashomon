package report

import (
	"sort"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/host"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Destinations is what the session declared it would reach.
//
// It used to set the wire's observation against the declarations, and the
// comparison was the product: "this host was reached and no tool call ever
// named it" is the line no transcript can produce. The proxy that made the
// observation was a separate product that never shipped, so every report
// rendered the comparison as not observed, and the proxy-era code is gone.
// What remains is the half the recorder can stand behind on its own: an
// inventory of what the calls named, and the calls that ran with an input
// other than the one they were declared with.
type Destinations struct {
	// DeclaredHosts is the sorted, de-duplicated set of hostnames tool calls
	// named. It is the record as written: a call to localhost or to the
	// client's own API host is listed like any other, because a declaration is
	// the agent's own command, and a list is not a finding.
	DeclaredHosts []string `json:"declared_hosts"`

	// SSHHosts are the ssh and git+ssh hosts the session named, kept apart
	// because an ssh host is a different kind of destination from an HTTPS
	// one, and the family view treats it so.
	SSHHosts []string `json:"ssh_hosts"`

	// ExecutedNotAsDeclared counts calls whose executed input digest differs
	// from the digest of the input they were declared with.
	//
	// A COUNT and not a diff. Neither input is stored, so the report can say
	// that a call ran differently from how it was asked for and cannot say how
	// -- which is the honest limit of a digest, and the reason the number is
	// rendered plainly as "0" on the overwhelming majority of sessions rather
	// than being hidden when it is zero. A line that only ever appeared when
	// non-zero would give a reader no way to know it was being checked.
	//
	// Only comparable pairs count: an execution with no digest (a payload that
	// carried no tool_input) is unknown, and unknown is not a difference.
	ExecutedNotAsDeclared int `json:"executed_not_as_declared"`

	// Rewritten names them. The count above answers "how many"; this answers
	// "which", which the store can support and the report was withholding.
	// Always an array, never null, so a consumer does not have to tell an
	// empty session from an unbuilt field.
	Rewritten []Rewritten `json:"rewritten"`

	// Novelty says which of this session's declared hosts are not yet in
	// this project's baseline. See internal/baseline for why ownership is
	// earliest-session-wins and why the file lives outside the evictable run
	// store.
	Novelty Novelty `json:"novelty"`
}

// Novelty is the per-project baseline-membership line.
//
// Its value depends entirely on not crying wolf. A novelty line that fires on
// a quarter of sessions is read once and ignored afterwards, which is why the
// ubiquitous list exists, why the first session in a project establishes rather
// than reports, and why an unreadable baseline renders unknown instead of
// treating every host as new.
//
// NOT a claim that a host is new to the project, whatever Hosts is named:
// the baseline is seeded from whichever session happens to be rendered
// first, which Runs() orders by directory mtime and --session can pin to any
// one session at all, not from the project's actual earliest session in
// start-time order. A host a genuinely earlier, not-yet-rendered session
// already named can still show up here as absent from the baseline. "Absent
// from the baseline" is the fact this type can back; "named for the first
// time" is a claim about history this type cannot make.
type Novelty struct {
	// Available is false when the baseline could not be read or written.
	// Novelty then renders as unknown: "no new hosts" and "we could not tell"
	// are different answers, and a corrupt baseline must not present as the
	// former.
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	// Established is true when this render created the project's baseline. The
	// first session has nothing to be novel against, so reporting every host it
	// reached as new would be true and useless.
	Established bool `json:"established"`
	// KnownHosts is the size of the baseline after this session.
	KnownHosts int `json:"known_hosts"`
	// Hosts are this session's declared hosts that were not yet in the
	// project's baseline, sorted, excluding the ubiquitous list.
	Hosts []string `json:"hosts"`
	// Evicted is how many hosts this render dropped from the baseline at its
	// 4,096-host cap (baseline.Result.Evicted), least recently seen first.
	// Without it, a host that returns after its own eviction reads as
	// "never named before" rather than "named before, and the baseline
	// aged it out" -- the same bytes, a different sentence. Omitted at
	// zero, which is every render that never reaches the cap.
	Evicted int `json:"evicted,omitempty"`
}

// clientPlaneHosts are the hosts the client contacts on its own behalf,
// regardless of the agent's tool calls.
//
// A fixed list, and it grows only by a RECORDED DECISION, never by an agent's
// judgement in the moment. Listed among the declared hosts like any other,
// since a declaration is the agent's own command, and excluded from novelty
// and from the sandbox section's agent columns: the client contacts these on
// every session of every project, so a first mention is not news, and an
// attempt on one in the sandbox's trail is the client's unless a call named
// it.
//
// http-intake.logs.us5.datadoghq.com was added on a recorded measurement
// (2026-10-09, Claude Code 2.1.285 inside nono 0.79.0 on macOS): the client
// posted its logs there during a session whose every tool call was a curl to
// other hosts or a WebFetch of example.com, and the sandbox, with that host
// off its allowlist, refused it. The sandbox section then read "refused by the
// sandbox: http-intake.logs.us5.datadoghq.com", an attempt the agent never
// made. The exact host, as measured; another region's intake is not assumed.
var clientPlaneHosts = map[string]bool{
	"api.anthropic.com":                  true,
	"statsig.anthropic.com":              true,
	"statsig.com":                        true,
	"sentry.io":                          true,
	"mcp-proxy.anthropic.com":            true,
	"http-intake.logs.us5.datadoghq.com": true,
}

// clientPlaneHost is the one rule for whether a host is the client's own
// traffic in a given session: on the list, except the MCP transport on a
// session that made mcp__* calls, which is then the agent's. Every reader that
// asks it of this session calls it -- novelHostCandidates and the sandbox
// section -- rather than restating it, so the views cannot drift apart;
// TestClientPlane_EveryViewAppliesTheOneRule holds them together.
func clientPlaneHost(h string, mcpAttributed bool) bool {
	return clientPlaneHosts[h] && !(h == mcpProxyHost && mcpAttributed)
}

// mcpProxyHost is the one client-plane host that can belong to the agent.
//
// It is the transport an mcp__* tool call travels over. On a session that made
// such a call, filing its traffic under the client plane would bury the only
// destination an MCP call can be seen at; on a session that made none, the
// client contacted it anyway and counting it as the agent's would accuse the
// agent of traffic it did not cause. Which of the two is true is decided by
// the declarations, not by the host.
const mcpProxyHost = "mcp-proxy.anthropic.com"

// mcpTool is the prefix every MCP tool name carries.
const mcpTool = "mcp__"

// buildDestinations reads the declarations. Every field but novelty is a
// property of the record the recorder wrote itself; novelty is the project
// baseline's answer, which the read step asked (NoveltyQueryOf).
//
// forgotten excludes a host from the two lists only for a declaration whose
// own recorded_at_unix_ms is at or before the latest forget --host of that
// name: stale evidence a failed removal left behind. A declaration from
// after the forget is this session's own fresh use of the name and is kept,
// so a project can still name a host it once forgot. See
// store.Store.ForgottenHostBefore.
func buildDestinations(run *store.Run, novelty Novelty, forgotten func(host string, declaredAtMS int64) bool) Destinations {
	return Destinations{
		DeclaredHosts:         declaredHostsFiltered(run, forgotten),
		SSHHosts:              sshDeclaredFiltered(run, forgotten),
		ExecutedNotAsDeclared: executedNotAsDeclared(run),
		Rewritten:             rewrittenCalls(run),
		Novelty:               novelty,
	}
}

// novelHostCandidates are the hosts a novelty baseline should consider: what
// this session named, whether over https or ssh, minus loopback and minus
// the client's own plane. Loopback is listed among the declared hosts and
// never counted as novel: a local server is not a new place the project's
// traffic goes, first baseline entry or not. It is decided on the parsed
// address (host.Loopback), so [0:0:0:0:0:0:0:1], 127.0.0.2 and api.localhost
// are loopback as [::1] is. Folding the client plane into a baseline would make
// api.anthropic.com a "new destination for this project" on the first session
// of every project, which is the novelty line's most obvious way to make
// itself worthless.
//
// wire and ssh are filtered and returned separately, never merged: the
// baseline has to know which of this session's hosts came from an ssh
// destination (baseline.UpdateOrigin's sshHosts) to skip the inet_aton fold
// for them, and that is a fact only the caller still has at this point. The
// two also ask Loopback differently for the same reason -- host.Loopback
// folds a wire host's numeric forms before asking, which is right for a URL
// a client actually resolves; host.LoopbackName does not, because an ssh
// alias such as "0" is not a URL a client resolves at all, and folding it
// first would read a destination that named nothing as this machine.
func novelHostCandidates(wireHosts, sshHosts []string, mcpAttributed bool) (wire, ssh []string) {
	filter := func(hosts []string, loopback func(string) bool) []string {
		var out []string
		for _, h := range hosts {
			if loopback(h) || clientPlaneHost(h, mcpAttributed) {
				continue
			}
			out = append(out, h)
		}
		return out
	}
	return filter(wireHosts, host.Loopback), filter(sshHosts, host.LoopbackName)
}

// NoveltyQuery is what the read step asks the project baseline for one run:
// the baseline is keyed by the run's working directory, owned per session id
// from the session's start, and told the hosts the session named. SSH marks
// the hosts among them that came from an ssh destination
// (baseline.UpdateOrigin's sshHosts), so a host named only over ssh is
// marked (entry.SSH) rather than folded as an IP literal on every later
// load; that is a fact only the run still has at this point.
type NoveltyQuery struct {
	CWD       string
	SessionID string
	Start     time.Time
	Hosts     []string
	SSH       map[string]bool
}

// NoveltyQueryOf is the run's question for the project baseline under
// storeRoot, or, when it cannot be asked, the Novelty that says why.
//
// The project key comes from the cwd the RUN recorded, never from the process
// rendering the report: a report run from a different directory must not key a
// past session to the reader's project. A session with no recorded cwd has no
// project, and novelty is unavailable rather than guessed.
//
// The hosts asked about are the ones the destinations lists show:
// buildDestinations' own forgotten predicate applies to them, so a host a
// failed forget left in a declaration is not fed back into the baseline the
// forget cleared, and the ssh hosts are candidates as the https ones are.
func NoveltyQueryOf(run *store.Run, storeRoot string, forgotten func(host string, declaredAtMS int64) bool) (NoveltyQuery, *Novelty) {
	n := Novelty{Hosts: []string{}}
	cwd := runCWD(run)
	if cwd == "" {
		n.Reason = "the run recorded no working directory, so its project is unknown"
		return NoveltyQuery{}, &n
	}
	if storeRoot == "" {
		n.Reason = "no store root, so the baseline could not be located"
		return NoveltyQuery{}, &n
	}
	// Ownership is recorded per session id, so an empty one makes every
	// comparison vacuous: two unidentified runs would each appear to own the
	// other's hosts and every host would read as novel. Found by a fixture that
	// omitted the id, which is exactly how a real run with an unattributed
	// session would arrive.
	if run.SessionID() == "" {
		n.Reason = "the run has no session id, so baseline ownership cannot be attributed"
		return NoveltyQuery{}, &n
	}
	wire, ssh := novelHostCandidates(declaredHostsFiltered(run, forgotten), sshDeclaredFiltered(run, forgotten), madeMCPCall(run))
	sshSet := make(map[string]bool, len(ssh))
	for _, h := range ssh {
		sshSet[h] = true
	}
	return NoveltyQuery{
		CWD:       cwd,
		SessionID: run.SessionID(),
		Start:     time.UnixMilli(runStartMS(run)),
		Hosts:     append(append([]string{}, wire...), ssh...),
		SSH:       sshSet,
	}, nil
}

// AnsweredNovelty is the section from the baseline's answer to a NoveltyQuery:
// whether this render established the baseline, its size after the session,
// how many hosts it aged out at its cap, the hosts that were new, and, when
// it could not answer, failure: the reason the read step names for the
// error, never the error's own text (see gather's baselineErrorReason).
func AnsweredNovelty(established bool, known, evicted int, novel []string, failure string) Novelty {
	n := Novelty{Hosts: []string{}}
	if failure != "" {
		n.Reason = failure
		return n
	}
	n.Available = true
	n.Established = established
	n.KnownHosts = known
	n.Evicted = evicted
	if novel != nil {
		n.Hosts = novel
	}
	return n
}

// runCWD is the working directory the run recorded, preferring the start-phase
// record because that is the one written before any tool call could change it.
func runCWD(run *store.Run) string {
	for _, c := range run.Coverage {
		if c.Phase == store.PhaseStart && c.CWD != "" {
			return c.CWD
		}
	}
	for _, c := range run.Coverage {
		if c.CWD != "" {
			return c.CWD
		}
	}
	return ""
}

// runStartMS is the session's own start instant, which decides baseline
// ownership. Falling back to the earliest record keeps a run with no start
// probe comparable rather than sorting it before everything.
func runStartMS(run *store.Run) int64 {
	var earliest int64
	for _, c := range run.Coverage {
		if c.Phase == store.PhaseStart {
			return c.RecordedAtMS
		}
		if earliest == 0 || c.RecordedAtMS < earliest {
			earliest = c.RecordedAtMS
		}
	}
	return earliest
}

type Rewritten struct {
	ToolUseID string `json:"tool_use_id"`
	ToolName  string `json:"tool_name"`
	// Program is the executable a Bash call named, empty for every other tool.
	Program string `json:"program"`
	// VerbClass is the derived class of the action: read, write, network...
	VerbClass string `json:"verb_class"`
}

// inputCompletedByHarness names the tools whose tool_input Claude Code itself
// completes between PreToolUse and PostToolUse: the user's answers are written
// into AskUserQuestion's input, and the approved plan into ExitPlanMode's. A
// different digest there is the harness doing its job, not a rewrite. Measured
// on a real 2,353-call session: every one of its four "executed differently"
// rows was one of these two tools, and no other tool produced one.
var inputCompletedByHarness = map[string]bool{
	"AskUserQuestion": true,
	"ExitPlanMode":    true,
}

// rewrittenCalls is executedNotAsDeclared with the ids kept.
//
// The two walk the same pairs under the same rule, and the count is defined as
// the length of this list so they cannot disagree.
func rewrittenCalls(run *store.Run) []Rewritten {
	type decl struct {
		digest string
		tool   string
		prog   string
		verb   string
	}
	declared := make(map[string]decl, len(run.Declarations))
	for _, d := range run.Declarations {
		if d.Shape.Digest == "" || inputCompletedByHarness[d.ToolName] {
			continue
		}
		e := decl{digest: d.Shape.Digest, tool: d.ToolName, verb: d.Shape.VerbClass}
		if d.Shape.Program != nil {
			e.prog = *d.Shape.Program
		}
		declared[d.ToolUseID] = e
	}

	out := []Rewritten{}
	for _, x := range run.Executions {
		// An execution with no digest carried no tool_input to derive one
		// from. Unknown is not a difference, and never has been here.
		if x.ExecutedDigest == "" {
			continue
		}
		d, ok := declared[x.ToolUseID]
		// Not secret: a digest is an equality key between records, so == is right (shape.digest, S1).
		if !ok || x.ExecutedDigest == d.digest {
			continue
		}
		out = append(out, Rewritten{
			ToolUseID: x.ToolUseID,
			ToolName:  d.tool,
			Program:   d.prog,
			VerbClass: d.verb,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ToolUseID < out[j].ToolUseID })
	return out
}

func executedNotAsDeclared(run *store.Run) int {
	// DEFINED AS the length of the row list, not a second walk of the same
	// pairs under the same rule. Two implementations of one predicate is how a
	// count and a list come to disagree, and a reader who sees "2" above one
	// row has no way to tell which is wrong.
	return len(rewrittenCalls(run))
}

// sshDeclaredFiltered is the sorted, de-duplicated set of ssh hosts the
// session named, with forgotten applied per declaration before the hosts are
// merged into one set: a host is dropped only for the
// declaration(s) that named it at or before the latest forget of that name,
// never for a later declaration that happens to share it. forgotten == nil
// passes every host through, matching the report's nil-means-no-filter
// convention (see suppressTrail). Always non-nil, so the JSON carries an empty
// list rather than null: "no ssh host was named" is a fact, and null would
// read as "we did not look".
//
// Canonicalised before forgotten is asked, the same correction
// declaredHostsFiltered applies and for the same reason: forget --host
// records its digest under the canonical spelling, never the one a
// declaration happens to hold, so asking forgotten about the stored spelling
// let a legacy or differently-cased ssh host -- "GitHub.com" where today's
// extraction writes "github.com" -- survive a forget of its canonical name
// and kept it out of the one spelling every current declaration uses. A host
// that no longer canonicalises, or that host.Shaped now refuses, is dropped
// rather than carried forward under whatever it used to be.
//
// host.CanonicalName, not Canonical: these are ssh destinations, and an
// ssh-config alias made only of digits is not a URL authority a client
// folds (host.CanonicalName's doc). Store.ForgetHost records a forget's
// digest under both spellings for exactly this reason, so a forget of
// either still finds this one.
func sshDeclaredFiltered(run *store.Run, forgotten func(host string, declaredAtMS int64) bool) []string {
	set := map[string]bool{}
	for _, d := range run.Declarations {
		for _, h := range d.SSHHosts {
			canon, ok := host.CanonicalName(h)
			if !ok || !host.Shaped(canon) {
				continue
			}
			if forgotten != nil && forgotten(canon, d.RecordedAtMS) {
				continue
			}
			set[canon] = true
		}
	}
	out := make([]string, 0, len(set))
	for h := range set {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// declaredHosts is the sorted, de-duplicated set of hostnames the session's
// tool calls named. Never nil, for the same reason as sshDeclaredFiltered.
//
// Unfiltered: buildNono's own caller needs the full named set regardless of
// what has been forgotten, to decide which trail hosts are the client's own.
// See declaredHostsFiltered for the view that applies forgotten.
func declaredHosts(run *store.Run) []string {
	return declaredHostsFiltered(run, nil)
}

// declaredHostsFiltered is declaredHosts with forgotten applied per
// declaration; see sshDeclaredFiltered for why that is per declaration and
// not over the merged set.
func declaredHostsFiltered(run *store.Run, forgotten func(host string, declaredAtMS int64) bool) []string {
	set := map[string]bool{}
	for _, d := range run.Declarations {
		for _, h := range d.Hosts {
			// A run stored before host.Canonical folded an IP literal to
			// its one spelling holds the raw string -- "167838211" where a
			// current build now writes "10.1.2.3". Re-canonicalising here
			// is a no-op for every run shape.Hosts already wrote the
			// canonical form into, and is what lets an old run compare
			// equal to a new one in the baseline novelty walks this feeds
			// (buildNovelty). A host that no longer canonicalises, or that
			// host.Shaped now refuses, is dropped rather than carried
			// forward under whatever it used to be.
			//
			// Canonicalised BEFORE forgotten is asked, not after: forget
			// --host records its digest under the canonical spelling, never
			// the stored one, so asking forgotten about h here let a legacy
			// declaration -- 167838211 where a forget named 10.1.2.3 --
			// escape the check and reappear in every later render of the
			// same old run.
			canon, ok := host.Canonical(h)
			if !ok || !host.Shaped(canon) {
				continue
			}
			if forgotten != nil && forgotten(canon, d.RecordedAtMS) {
				continue
			}
			set[canon] = true
		}
	}
	out := make([]string, 0, len(set))
	for h := range set {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// madeMCPCall reports whether the session declared any MCP tool call.
//
// The declaration is the evidence, not the host: an MCP call names no hostname
// of its own, so the transport is the only place it can be seen, and this is
// what connects the two.
func madeMCPCall(run *store.Run) bool {
	for _, d := range run.Declarations {
		if strings.HasPrefix(d.ToolName, mcpTool) {
			return true
		}
	}
	return false
}
