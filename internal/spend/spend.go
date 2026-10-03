// Package spend estimates what the last N days of Claude Code usage would
// cost at Anthropic's API list prices, from Claude Code's own transcripts.
//
// ZERO SETUP. It cannot depend on `watch` having run, so its source is
// Claude Code's transcripts (see scan.go for the exact, narrow read of usage,
// and for the read of the text blocks of a turn's tied assistant lines the
// silent-failure line needs), not rashomon's store. The store is consulted
// for exactly one line -- spend in turns with a failed call the summary never
// mentioned -- and only read, through store.OpenExisting, so asking what was
// spent cannot mint an install identity on a machine that never recorded
// anything (H-87's rule).
// Nothing is written anywhere.
//
// AN ESTIMATE, AND SAID TO BE. The rates are a dated snapshot compiled into
// the binary (price.go), every rendering names the snapshot date, and a plan
// subscription (Pro, Max) is not billed per token at all: the figure is what
// the same usage would cost on the API, never what anyone was charged. A
// model the table does not know has its tokens shown and its cost unknown --
// never $0, which would read as "free" rather than "not priced".
package spend

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// SchemaVersion is the JSON document's own version.
const SchemaVersion = 1

// DefaultDays is the window when none is asked for.
const DefaultDays = 30

// MaxDays is the longest window asked for: a century, longer than Claude
// Code has existed. The bound is not about the data. It is that the window
// used to be computed as a time.Duration, which overflows past 106,751 days
// and wrapped into a start date AFTER now -- a confident "$0.00, no
// transcripts" about a window that contained everything. WindowStart no
// longer uses a Duration, and a day count past any transcript's age is
// refused rather than taken on trust.
const MaxDays = 36500

// futureSlack is how far past the run's own clock a response may be dated
// and still count. A transcript being written while spend reads it (this very
// session's) holds lines dated after the moment `now` was taken; anything
// later than this is a clock or a line that is wrong, and it is counted as
// future-dated rather than as "the last N days".
const futureSlack = 10 * time.Minute

// WindowStart is the first moment of a `days`-day window ending at now,
// computed by calendar arithmetic rather than a time.Duration, which cannot
// hold much more than 292 years.
func WindowStart(now time.Time, days int) time.Time {
	return now.AddDate(0, 0, -days)
}

// Cold-cache TTLs, the API's two cache lifetimes.
const (
	ttl5m = 5 * time.Minute
	ttl1h = time.Hour
)

// Cost is an amount that may be partly or wholly unpriced.
//
// It never collapses "unpriced" into zero. Nano holds only what the table
// could price; Unpriced and UnpricedTokens say how much was left out and so
// how far Nano is from the whole. A cost with no priced part and an unpriced
// one marshals its usd as null -- not 0 -- so a JSON consumer cannot read
// "unknown" as "free" either.
type Cost struct {
	Nano           int64
	Priced         int
	Unpriced       int
	UnpricedTokens int64
}

// Known reports whether every response behind this cost was priced.
func (c Cost) Known() bool { return c.Unpriced == 0 }

// Wholly reports whether there is a priced part to show at all. A cost with
// no responses behind it is a known zero, which is a fact and shown as one.
func (c Cost) Wholly() bool { return c.Priced > 0 || c.Unpriced == 0 }

func (c *Cost) addPriced(nano int64) { c.Nano += nano; c.Priced++ }
func (c *Cost) addUnpriced(tokens int64) {
	c.Unpriced++
	c.UnpricedTokens += tokens
}

func (c *Cost) addCost(o Cost) {
	c.Nano += o.Nano
	c.Priced += o.Priced
	c.Unpriced += o.Unpriced
	c.UnpricedTokens += o.UnpricedTokens
}

// USD is the priced part in dollars.
func (c Cost) USD() float64 { return float64(c.Nano) / 1e9 }

// MarshalJSON writes {"usd": <number|null>, "unpriced_responses": n,
// "unpriced_tokens": n}.
func (c Cost) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		USD            *float64 `json:"usd"`
		Unpriced       int      `json:"unpriced_responses"`
		UnpricedTokens int64    `json:"unpriced_tokens"`
	}{c.usdOrNull(), c.Unpriced, c.UnpricedTokens})
}

// usdOrNull is the usd a cost marshals: its priced part, or null when it has
// none and an unpriced part.
func (c Cost) usdOrNull() *float64 {
	if !c.Wholly() {
		return nil
	}
	v := c.USD()
	return &v
}

// Pricing states the basis of every dollar figure in the document.
type Pricing struct {
	Snapshot string `json:"snapshot"`
	Basis    string `json:"basis"`
	Note     string `json:"note"`
}

// PlanNote is the sentence every rendering carries about subscriptions.
const PlanNote = "plan subscriptions (Pro, Max) are not billed per token: this is what the same usage would cost on the API, not what was charged"

// Summary is the whole answer.
type Summary struct {
	SchemaVersion     int     `json:"schema_version"`
	GeneratedAtUnixMS int64   `json:"generated_at_unix_ms"`
	Days              int     `json:"days"`
	FromUnixMS        int64   `json:"from_unix_ms"`
	Pricing           Pricing `json:"pricing"`

	Sessions  int    `json:"sessions"`
	Responses int    `json:"responses"`
	Tokens    Tokens `json:"tokens"`
	Total     Cost   `json:"total"`

	ByAgent AgentSplit   `json:"by_agent"`
	ByModel []ModelSpend `json:"by_model"`
	ByKind  KindSplit    `json:"by_kind"`

	CacheExpiry   CacheExpiry   `json:"cache_expiry"`
	Refusals      Refusals      `json:"refusals"`
	ExtraAttempts ExtraAttempts `json:"extra_attempts"`
	FastMode      FastMode      `json:"fast_mode"`

	SilentFailureTurns SilentFailureTurns `json:"silent_failure_turns"`

	Savings    []Saving       `json:"savings"`
	PerSession []SessionSpend `json:"per_session"`
	// SharedResponses counts the windowed responses with more than one
	// owner (a /branch tie): each is in every owner's row and once in the
	// total, so the rows can add up to more than the total.
	SharedResponses int `json:"shared_responses"`

	// Read is how the transcripts went, over everything read (not only the
	// window): the dedupe measurement, and what could not be counted.
	Read ReadStats `json:"read"`

	window []*Response
	// refused is the window's pre-output refusals: in no figure, but their
	// sessions have rows and their transcripts are the window's (Join).
	refused []*Response
	scan    *Scan
}

// ReadStats is what the read itself found.
type ReadStats struct {
	Files int `json:"files"`
	// UsageLines against DistinctResponses is the dedupe, measured on the
	// reader's own machine: their ratio is how much a line-summing reader
	// would have overstated.
	UsageLines        int `json:"usage_lines"`
	DistinctResponses int `json:"distinct_responses"`
	UnreadableFiles   int `json:"unreadable_files"`
	UndatedResponses  int `json:"undated_responses"`
	// FilesBeforeWindow counts transcripts not read because they were last
	// written before the window (Found.Stale).
	FilesBeforeWindow int `json:"files_before_window"`
	// UnparsedUsageLines counts lines that may carry usage and could not be
	// counted (Scan.Unparsed).
	UnparsedUsageLines int `json:"unparsed_usage_lines"`
	// UnreadableDirs counts folders under projects/ that could not be listed.
	UnreadableDirs int `json:"unreadable_dirs"`
	// FutureDatedResponses counts responses dated more than futureSlack
	// after this run. They are not "the last N days" and are not counted.
	FutureDatedResponses int `json:"future_dated_responses"`
}

// AgentSplit is main-agent spend against subagent spend.
type AgentSplit struct {
	Main      Cost `json:"main"`
	Subagents Cost `json:"subagents"`
}

// ModelSpend is one model's share.
type ModelSpend struct {
	// Model is the table key for a priced model (a dated suffix folded into
	// it), and for an unpriced one see displayModel.
	Model     string `json:"model"`
	Priced    bool   `json:"priced"`
	Responses int    `json:"responses"`
	Tokens    Tokens `json:"tokens"`
	Cost      Cost   `json:"cost"`
}

// KindSplit is spend by token kind.
type KindSplit struct {
	Input      Cost `json:"input"`
	Output     Cost `json:"output"`
	CacheWrite Cost `json:"cache_write"`
	CacheRead  Cost `json:"cache_read"`
}

// CacheExpiry is the cold-cache heuristic's result.
//
// Tokens are the cold writes' tokens; Cost is what writing them cost over
// reading the same tokens from a warm cache (write rate minus read rate),
// because the alternative to a re-write was a cache read, not nothing.
type CacheExpiry struct {
	Heuristic string `json:"heuristic"`
	Responses int    `json:"responses"`
	Tokens    int64  `json:"tokens"`
	// Tokens5m and Tokens1h split Tokens by the TTL the write was made
	// with.
	Tokens5m int64 `json:"tokens_5m"`
	Tokens1h int64 `json:"tokens_1h"`
	Cost     Cost  `json:"cost"`
}

// CacheHeuristic is the rule, stated wherever its number is.
const CacheHeuristic = "a cache write on a response whose previous response by the same agent in the same transcript, on the same model, started more than the TTL earlier (5m, or 1h for a 1h write), counting only the shortfall -- what that previous response read and wrote to the cache, less what this one read from it -- and nothing when this request is smaller than that cache, priced as the write over a cache read of the same tokens; it skips a model switch and any request smaller than the previous cache, so it misses some true expiries, and can still count new content in a request that grew past the previous cache"

// Refusals is responses that ended with stop_reason "refusal", in all and
// counted by category and model (ByCategory).
//
// A refusal that produced output is priced like any other response: the
// refusals-and-fallback page says "A mid-stream refusal bills the input
// tokens and the output already streamed at normal rates". Responses and
// Cost are those, and they are in the total.
//
// A refusal before any output is read from output_tokens == 0, never from
// the shape of the line. Whether it was billed depends on its category, and
// this read does not price by category, so BeforeOutput counts those with
// usage and BeforeOutputTokens holds their tokens: shown with the cost
// unknown, out of the total and every breakdown, and the header says so.
//
// WithoutUsage counts the zero-usage refusal lines that report no response
// with usage (Scan.foldRefusalMessages folds the rest into theirs), which
// Build otherwise drops with every zero-token response: pre-output refusals
// whose tokens the transcript does not hold. Dropping them printed "refusals
// none" beside a transcript that held one.
type Refusals struct {
	Responses          int   `json:"responses"`
	Cost               Cost  `json:"cost"`
	BeforeOutput       int   `json:"before_output"`
	BeforeOutputTokens int64 `json:"before_output_tokens"`
	// BeforeOutputPricing is PreOutputRefusalPricing, so a JSON reader is
	// told BeforeOutputTokens are out of total and tokens.
	BeforeOutputPricing string         `json:"before_output_pricing"`
	WithoutUsage        int            `json:"without_usage"`
	ByCategory          []RefusalGroup `json:"by_category"`
}

// PreOutputRefusalPricing is Refusals.BeforeOutputPricing, verbatim.
const PreOutputRefusalPricing = "tokens only, cost unknown, out of total and tokens: whether a refusal before any output was billed depends on its category"

// RefusalGroup counts one category's refusals on one model: a classifier
// decline in a named category is told apart from the rest. Model is
// ModelNotRecorded for Claude Code's "<synthetic>" refusal line.
type RefusalGroup struct {
	Category     string `json:"category"`
	Model        string `json:"model"`
	Responses    int    `json:"responses"`
	BeforeOutput int    `json:"before_output"`
	WithoutUsage int    `json:"without_usage"`
}

// ExtraAttempts is responses whose usage.iterations holds more than one
// attempt, and the responses a fallback model served.
//
// The extra attempts -- every entry before the last, which produced the
// message and is the top-level usage -- are tokens only: their cost is
// unknown, and they are out of the total and every breakdown, which the
// header says. FallbackServed counts the responses a fallback chain served:
// never a chain that ended in a refusal, which the page calls the last
// model's refusal.
type ExtraAttempts struct {
	Responses      int    `json:"responses"`
	Attempts       int    `json:"attempts"`
	Tokens         Tokens `json:"tokens"`
	FallbackServed int    `json:"fallback_served"`
	// Pricing is ExtraAttemptsPricing, so a JSON reader is told Tokens are
	// out of total and tokens.
	Pricing string `json:"pricing"`
}

// ExtraAttemptsPricing is ExtraAttempts.Pricing, verbatim.
const ExtraAttemptsPricing = "tokens only, cost unknown, out of total and tokens"

// FastMode is responses that ran in fast mode (usage.speed "fast"). Fast
// mode bills at a premium -- Opus 5.5 at $8/$40 per MTok against $4/$20 --
// that the table does not hold, so these are priced at standard rates and
// the figure is low by that premium; the count says how many.
type FastMode struct {
	Responses int    `json:"responses"`
	Pricing   string `json:"pricing"`
}

// FastModePricing is FastMode's pricing, verbatim.
const FastModePricing = "priced at standard rates: fast mode's premium is not in the price table"

// SessionSpend is one session's split.
//
// Every session holding a windowed response has a row, a session holding
// only pre-output refusals included, and a response's cost
// is in the rows of the sessions it belongs to (Response.owners). A response
// two transcripts start with at the same moment -- a /branch copy keeps the
// original's timestamps -- belongs to both, so the rows can sum to more than
// the total, which counts it once.
type SessionSpend struct {
	SessionID string `json:"session_id"`
	Main      Cost   `json:"main"`
	Subagents Cost   `json:"subagents"`
	// Coverage says whether rashomon recorded the session's own transcripts
	// and whether the row holds dollars that are not covered
	// (CoverageRecorded, CoveragePartly, CoverageNotRecorded): recorded only
	// when every transcript of its own was recorded and it holds no
	// not-covered dollar, and not recorded when no transcript of its own was
	// recorded. Join sets it; it is absent when no store was consulted.
	Coverage string `json:"coverage,omitempty"`

	id string // the session id as read, which SessionID may have replaced
}

// Saving is one suggestion, with the figure it rests on. A suggestion with
// no figure is not made.
type Saving struct {
	Kind string `json:"kind"`
	Cost Cost   `json:"cost"`
}

// SavingSilentFailure is the one saving: spend in silently failed turns.
const SavingSilentFailure = "silently_failed_turns"

// ModelNotRecorded names a model the transcript does not record: a refusal
// Claude Code wrote as a "<synthetic>" line, which does not say which model
// refused.
const ModelNotRecorded = "not_recorded"

// priced is a response's cost broken down by kind, or ok=false for a model
// the table does not know.
type priced struct {
	input, output, cacheWrite, cacheRead int64
	ok                                   bool
}

func (p priced) total() int64 { return p.input + p.output + p.cacheWrite + p.cacheRead }

// price prices a response at its model's rates.
func price(r *Response) priced {
	key, ok := PriceKey(r.Model)
	if !ok {
		return priced{}
	}
	rt, _ := RatesFor(key)
	t := r.Tokens
	return priced{
		input:      t.Input * rt.Input,
		output:     t.Output * rt.Output,
		cacheWrite: t.CacheWrite5m*rt.CacheWrite5m() + t.CacheWrite1h*rt.CacheWrite1h(),
		cacheRead:  t.CacheRead * rt.CacheRead,
		ok:         true,
	}
}

// costOf adds one response to c: its priced total, or an unpriced mark
// carrying its tokens.
func costOf(c *Cost, r *Response) {
	p := price(r)
	if p.ok {
		c.addPriced(p.total())
		return
	}
	c.addUnpriced(r.Tokens.Total())
}

// displayModel is how a model is named in the output.
//
// A priced model is named by its table key, which is closed vocabulary. An
// unpriced one is named only when it has the shape of an Anthropic model id
// -- "claude-" and then lowercase letters, digits and hyphens, bounded -- and
// otherwise as "other". The model string comes from a transcript, and a
// custom deployment can put nearly anything there (a provider ARN carries an
// account number); the privacy rule this program keeps is that only closed-
// vocabulary words reach an output, and a claude-shaped id is the widest set
// that stays inside it while still telling a reader which model went unpriced.
func displayModel(model string) (string, bool) {
	if key, ok := PriceKey(model); ok {
		return key, true
	}
	if claudeShaped(model) {
		return model, false
	}
	return "other", false
}

func claudeShaped(s string) bool {
	return strings.HasPrefix(s, "claude-") && idShaped(s)
}

// idShaped is the closed shape an identifier must have to be printed:
// lowercase letters, digits and hyphens, at most 64 of them.
func idShaped(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// displaySession is how a session is named in the output: its id when the id
// has the closed shape a Claude Code session id has (a lowercase UUID fits
// idShaped), and "other" otherwise. The id is read from a transcript line's
// sessionId -- or, lacking one, from the file's name -- and either can carry
// anything: a path-like value would otherwise reach the JSON verbatim. The
// same rule displayModel applies to a model string, for the same reason.
func displaySession(id string) string {
	if idShaped(id) {
		return id
	}
	return "other"
}

// Build summarises a scan over the last `days` days before now.
//
// A response with no tokens at all is not counted anywhere but one place.
// Claude Code writes synthetic assistant lines (model "<synthetic>") for
// local errors with an all-zero usage; nothing was billed for them, and
// listing a model named "other" with zero tokens and an unknown cost would be
// noise that looks like a finding. The one place: a zero-usage line that
// ended in a refusal and reports no response with usage is a pre-output
// refusal, counted in Refusals.WithoutUsage. A pre-output refusal with usage
// is likewise out of every figure but the refusals, the header's caveat, the
// extra attempts it carried and the cache-expiry heuristic, where its write
// is shown with the cost unknown. The session of a pre-output refusal of
// either kind still counts and has a row, with none as its priced spend, and
// its transcript is one of the window's.
func Build(sc *Scan, now time.Time, days int) *Summary {
	from := WindowStart(now, days)
	latest := now.Add(futureSlack).UnixMilli()
	s := &Summary{
		SchemaVersion:     SchemaVersion,
		GeneratedAtUnixMS: now.UnixMilli(),
		Days:              days,
		FromUnixMS:        from.UnixMilli(),
		Pricing: Pricing{
			Snapshot: PriceSnapshot,
			Basis:    "estimated at API list prices",
			Note:     PlanNote,
		},
		ByModel:       []ModelSpend{},
		Savings:       []Saving{},
		PerSession:    []SessionSpend{},
		CacheExpiry:   CacheExpiry{Heuristic: CacheHeuristic},
		Refusals:      Refusals{BeforeOutputPricing: PreOutputRefusalPricing, ByCategory: []RefusalGroup{}},
		ExtraAttempts: ExtraAttempts{Pricing: ExtraAttemptsPricing},
		FastMode:      FastMode{Pricing: FastModePricing},
		Read: ReadStats{
			Files:              len(sc.Files),
			UsageLines:         sc.UsageLines,
			DistinctResponses:  len(sc.Responses),
			UnreadableFiles:    sc.Unreadable,
			UndatedResponses:   sc.Undated,
			FilesBeforeWindow:  sc.Stale,
			UnparsedUsageLines: sc.Unparsed,
			UnreadableDirs:     sc.UnreadableDirs,
		},
		scan: sc,
	}
	s.SilentFailureTurns = SilentFailureTurns{Store: StoreNotConsulted}

	cold := coldWrites(sc)

	models := map[string]*ModelSpend{}
	sessions := map[string]*SessionSpend{}
	refusals := map[[2]string]*RefusalGroup{}
	refusal := func(r *Response) *RefusalGroup {
		k := [2]string{r.Category, refusalModel(r.Model)}
		g := refusals[k]
		if g == nil {
			g = &RefusalGroup{Category: k[0], Model: k[1]}
			refusals[k] = g
		}
		return g
	}
	// Every session holding a windowed response has a row, a pre-output
	// refusal's included; its cost is in the rows of the sessions it belongs
	// to (ownerSessions).
	row := func(id string) *SessionSpend {
		sess, ok := sessions[id]
		if !ok {
			sess = &SessionSpend{SessionID: displaySession(id), id: id}
			sessions[id] = sess
		}
		return sess
	}
	for _, r := range sc.Responses {
		if r.StartMS > latest && r.Tokens.Total() > 0 {
			s.Read.FutureDatedResponses++
			continue
		}
		if r.StartMS == 0 || r.StartMS < s.FromUnixMS {
			continue
		}
		if r.Tokens.Total() == 0 {
			if r.StopReason == "refusal" && r.StartMS <= latest {
				s.Refusals.WithoutUsage++
				refusal(r).WithoutUsage++
				for _, f := range r.files {
					row(f.session)
				}
				s.refused = append(s.refused, r)
			}
			continue
		}

		if w, ok := cold[r]; ok {
			s.CacheExpiry.Responses++
			s.CacheExpiry.Tokens += w.CacheWrite5m + w.CacheWrite1h
			s.CacheExpiry.Tokens5m += w.CacheWrite5m
			s.CacheExpiry.Tokens1h += w.CacheWrite1h
			if key, ok := PriceKey(r.Model); ok && !r.costUnknown {
				rt, _ := RatesFor(key)
				s.CacheExpiry.Cost.addPriced(w.CacheWrite5m*(rt.CacheWrite5m()-rt.CacheRead) + w.CacheWrite1h*(rt.CacheWrite1h()-rt.CacheRead))
			} else {
				s.CacheExpiry.Cost.addUnpriced(w.CacheWrite5m + w.CacheWrite1h)
			}
		}
		s.ExtraAttempts.add(r)
		for _, f := range r.files {
			row(f.session)
		}
		if r.costUnknown {
			s.Refusals.BeforeOutput++
			s.Refusals.BeforeOutputTokens += r.Tokens.Total()
			refusal(r).BeforeOutput++
			s.refused = append(s.refused, r)
			continue
		}

		s.window = append(s.window, r)
		s.Responses++
		s.Tokens.add(r.Tokens)
		costOf(&s.Total, r)

		if r.Subagent {
			costOf(&s.ByAgent.Subagents, r)
		} else {
			costOf(&s.ByAgent.Main, r)
		}
		if len(r.owners) > 1 {
			s.SharedResponses++
		}
		for _, id := range r.owners {
			if sess := row(id); r.Subagent {
				costOf(&sess.Subagents, r)
			} else {
				costOf(&sess.Main, r)
			}
		}

		name, known := displayModel(r.Model)
		m, ok := models[name]
		if !ok {
			m = &ModelSpend{Model: name, Priced: known}
			models[name] = m
		}
		m.Responses++
		m.Tokens.add(r.Tokens)
		costOf(&m.Cost, r)

		p := price(r)
		kind := func(c *Cost, nano, toks int64) {
			if toks == 0 {
				return
			}
			if p.ok {
				c.addPriced(nano)
			} else {
				c.addUnpriced(toks)
			}
		}
		kind(&s.ByKind.Input, p.input, r.Tokens.Input)
		kind(&s.ByKind.Output, p.output, r.Tokens.Output)
		kind(&s.ByKind.CacheWrite, p.cacheWrite, r.Tokens.CacheWrite5m+r.Tokens.CacheWrite1h)
		kind(&s.ByKind.CacheRead, p.cacheRead, r.Tokens.CacheRead)

		if r.StopReason == "refusal" {
			s.Refusals.Responses++
			costOf(&s.Refusals.Cost, r)
			refusal(r).Responses++
		}
		if r.Fast {
			s.FastMode.Responses++
		}
	}
	s.Sessions = len(sessions)
	s.Refusals.ByCategory = sortedRefusals(refusals)

	for _, m := range models {
		s.ByModel = append(s.ByModel, *m)
	}
	// Most expensive first; unpriced after every priced model, by tokens,
	// since there is no dollar figure to order them by; name breaks ties so
	// the order is stable.
	sort.Slice(s.ByModel, func(i, j int) bool {
		a, b := s.ByModel[i], s.ByModel[j]
		if a.Priced != b.Priced {
			return a.Priced
		}
		if a.Priced && a.Cost.Nano != b.Cost.Nano {
			return a.Cost.Nano > b.Cost.Nano
		}
		if !a.Priced && a.Tokens.Total() != b.Tokens.Total() {
			return a.Tokens.Total() > b.Tokens.Total()
		}
		return a.Model < b.Model
	})
	for _, p := range sessions {
		s.PerSession = append(s.PerSession, *p)
	}
	sort.Slice(s.PerSession, func(i, j int) bool {
		a := s.PerSession[i].Main.Nano + s.PerSession[i].Subagents.Nano
		b := s.PerSession[j].Main.Nano + s.PerSession[j].Subagents.Nano
		if a != b {
			return a > b
		}
		return s.PerSession[i].SessionID < s.PerSession[j].SessionID
	})
	s.buildSavings()
	return s
}

// add counts one windowed response's extra attempts, and whether a fallback
// served it.
func (e *ExtraAttempts) add(r *Response) {
	// A chain that ended in a refusal served nothing: every model declined.
	if r.Fallback && r.StopReason != "refusal" {
		e.FallbackServed++
	}
	if len(r.Attempts) == 0 {
		return
	}
	e.Responses++
	e.Attempts += len(r.Attempts)
	for _, a := range r.Attempts {
		e.Tokens.add(a)
	}
}

// refusalModel is how a refusal group names its model: ModelNotRecorded for
// Claude Code's "<synthetic>" line, which names none, and displayModel's name
// otherwise -- "other" read as a model of that name.
func refusalModel(model string) string {
	if model == "<synthetic>" {
		return ModelNotRecorded
	}
	name, _ := displayModel(model)
	return name
}

func sortedRefusals(m map[[2]string]*RefusalGroup) []RefusalGroup {
	out := []RefusalGroup{}
	for _, g := range m {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		return a.Model < b.Model
	})
	return out
}

// buildSavings lists the suggestions a number supports, and only those: the
// spend in silently failed turns.
//
// The design's others are not made. The cache re-write figure is a
// heuristic: it skips a model switch and any request smaller than the
// previous cache, so it misses some true expiries, and can still count new
// content in a request that grew past the previous cache. So it is shown as
// a figure and not offered as a saving. Pre-output refusals and extra
// attempts are tokens with the cost unknown, so they carry no saving; a
// refusal with output is priced like any response. Subagents on the top
// model where their tool pattern is read-heavy would need which tools a subagent called, which is in
// message.content, which this package never reads: a suggestion printed
// without its figure is exactly what this list refuses.
func (s *Summary) buildSavings() {
	s.Savings = s.Savings[:0]
	if s.SilentFailureTurns.Cost.Nano > 0 {
		s.Savings = append(s.Savings, Saving{Kind: SavingSilentFailure, Cost: s.SilentFailureTurns.Cost})
	}
}

// coldWrites applies the cache-expiry heuristic to every response in the
// scan (the window is applied by the caller) and returns, per response, the
// cache-write tokens it attributes to a cold cache.
//
// Per AGENT PER TRANSCRIPT FILE, in start order: a response's previous
// response is the one before it in the same file by the same kind of agent,
// which is the same conversation's previous request -- a subagent's first
// request follows nothing, because a new context is a cold start by
// construction, not an expiry. Per agent because a main transcript also
// carries its subagents' lines (isSidechain): taken as one stream, a subagent
// working through the main agent's pause made the main agent's next write
// look warm, and the subagent's first write look like the main cache
// expiring -- a cold write mis-attributed on real data. Several sidechain
// agents in one main file cannot be told apart by these fields and share one
// stream: a gap can look shorter than it was, and a second agent's first
// write follows the first agent's cache, which it never held. The size guard
// below skips that write whenever the second agent's request is smaller than
// what the first had cached, and otherwise it can be counted. The window is
// deliberately NOT applied before ordering: the previous response of the
// first one inside the window may lie outside it, and treating that one as
// having no predecessor would hide the very gap being measured.
//
// Each TTL is judged on its own tokens: a 5m write is cold when the gap
// exceeds 5 minutes, a 1h write only when it exceeds an hour. The gap is
// start to start, which is longer than the idle time the cache actually saw,
// so this can over-attribute near the threshold -- one reason it is labelled
// a heuristic wherever its number appears.
//
// ONLY THE SHORTFALL IS COLD. The previous response left read+write tokens
// in the cache; what this response read back of them was still warm, and a
// write past that much is new content, not an expiry. So the cold part of a
// write is min(write, max(0, (read_prev + write_prev) - read)): the cached
// prefix it had to write again. A response that read a still-warm prefix (a
// 1h breakpoint, or one a parallel session kept warm) and re-wrote the
// expired rest is counted for the rest. Counting every write whole priced a
// 1k write on a warm cache as an expiry; the rule that replaced it -- a write
// with any cache read is not cold -- dropped every partial expiry instead,
// and most re-writes after a long gap had also read a still-warm prefix. The
// cold part is the 5m write first and then the 1h write, the cheaper first,
// so a mixed write is priced at the lower rate; it is priced as the write
// rate minus the read rate (Build): what re-reading those tokens would have
// cost is not a saving.
//
// THE SHORTFALL ASSUMES THE PROMPT EXTENDS THE PREVIOUS ONE, so a write up to
// it re-writes what the previous response cached. That is false after
// compaction, after a model switch (opusplan switches models within a
// session, and one model's cache never held the other's conversation), after
// a rewind, and for a second sidechain agent in the stream: each writes new
// content, which the shortfall counted cold. So a response is not judged when
// its model is not the previous response's, or when its whole prompt (input,
// cache read and cache write) is smaller than what the previous response
// cached -- it cannot be re-writing all of that. So the heuristic skips a
// model switch and any request smaller than the previous cache, so it misses
// some true expiries, and can still count new content in a request that grew
// past the previous cache: a rewind followed by a paste past the old cache,
// or a second agent whose first request is larger than the first agent's
// cache. Measured on one real machine's last 30 days to 2026-10-02: $92.06
// of $288.94.
//
// EVERY FILE HOLDING A RESPONSE IS A STREAM, and the response is judged once,
// in the file it was first seen in. A resumed conversation carries the
// original's responses into its own file: a stream of only the responses
// first seen there left the resumed file's first new response with no
// predecessor in one path order and a predecessor in the other, so the cold
// figure followed the sort order.
//
// Only a billed response is a predecessor. A zero-token line (Build's
// "<synthetic>" local error) sent nothing, so it refreshed no cache; taking
// it as the previous request would reset the gap and hide the cold write
// right after it.
func coldWrites(sc *Scan) map[*Response]Tokens {
	type stream struct {
		file     int
		subagent bool
	}
	byFile := map[stream][]*Response{}
	for _, r := range sc.Responses {
		if r.StartMS == 0 || r.Tokens.Total() == 0 {
			continue
		}
		for _, f := range r.files {
			k := stream{f.idx, r.Subagent}
			byFile[k] = append(byFile[k], r)
		}
	}
	out := map[*Response]Tokens{}
	for k, rs := range byFile {
		sort.SliceStable(rs, func(i, j int) bool { return rs[i].StartMS < rs[j].StartMS })
		for i := 1; i < len(rs); i++ {
			if rs[i].file != k.file {
				continue
			}
			prev, cur := rs[i-1].Tokens, rs[i].Tokens
			cached := prev.CacheRead + prev.CacheWrite5m + prev.CacheWrite1h
			if rs[i].Model != rs[i-1].Model || cur.Input+cur.CacheRead+cur.CacheWrite5m+cur.CacheWrite1h < cached {
				continue
			}
			short := max(0, cached-cur.CacheRead)
			gap := time.Duration(rs[i].StartMS-rs[i-1].StartMS) * time.Millisecond
			var w Tokens
			if gap > ttl5m {
				w.CacheWrite5m = min(cur.CacheWrite5m, short)
			}
			if gap > ttl1h {
				w.CacheWrite1h = min(cur.CacheWrite1h, short-w.CacheWrite5m)
			}
			if w.CacheWrite5m+w.CacheWrite1h > 0 {
				out[rs[i]] = w
			}
		}
	}
	return out
}
