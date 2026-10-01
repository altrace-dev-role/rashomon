// Package spend estimates what the last N days of Claude Code usage would
// cost at Anthropic's API list prices, from Claude Code's own transcripts.
//
// ZERO SETUP. It cannot depend on `watch` having run, so its source is
// Claude Code's transcripts (see scan.go for the exact, narrow read of usage,
// and for the one read of a turn's final message the silent-failure line
// needs), not rashomon's store. The store is consulted for exactly one line -- spend in
// turns with a failed call the summary never mentioned -- and only
// read, through store.OpenExisting, so asking what was spent cannot mint an
// install identity on a machine that never recorded anything (H-87's rule).
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
	var usd *float64
	if c.Wholly() {
		v := c.USD()
		usd = &v
	}
	return json.Marshal(struct {
		USD            *float64 `json:"usd"`
		Unpriced       int      `json:"unpriced_responses"`
		UnpricedTokens int64    `json:"unpriced_tokens"`
	}{usd, c.Unpriced, c.UnpricedTokens})
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

	Savings []Saving `json:"savings"`
	// SavingsNotComputed names the savings that were billed but carry no
	// amount in the transcript (SavingNotComputedRefusals,
	// SavingNotComputedAttempts), so their absence from Savings is not read
	// as "nothing to save there". Empty when there are none.
	SavingsNotComputed []string       `json:"savings_not_computed"`
	PerSession         []SessionSpend `json:"per_session"`

	// Read is how the transcripts went, over everything read (not only the
	// window): the dedupe measurement, and what could not be counted.
	Read ReadStats `json:"read"`

	window []*Response
	scan   *Scan
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
	// with. Only a 5m write could have been kept by a longer TTL, so only
	// Tokens5m supports that advice (SavingHintLongerTTL).
	Tokens5m int64 `json:"tokens_5m"`
	Tokens1h int64 `json:"tokens_1h"`
	Cost     Cost  `json:"cost"`
}

// CacheHeuristic is the rule, stated wherever its number is.
const CacheHeuristic = "a cache write on a response that read nothing from the cache and whose previous response by the same agent in the same transcript started more than the TTL earlier (5m, or 1h for a 1h write), priced as the write over a cache read of the same tokens"

// Refusals is responses that ended with stop_reason "refusal", in all and by
// category and model (ByCategory).
//
// Whether a refusal came before any output is read from output_tokens == 0,
// never from the shape of the line, and such a refusal is billed by its
// category (BilledBeforeOutput). Responses and Cost are the refusals in the
// total: every one that produced output -- the page bills a mid-stream
// refusal "at normal rates", whatever its category -- every pre-output one
// in a billed category, and a pre-output one in a category this read does
// not know, with its tokens shown and its cost unknown. NotBilled counts the
// pre-output refusals with usage in a category the page says is not billed
// (cyber, general_harms, uncategorized): out of every figure.
//
// WithoutUsage counts the zero-usage refusal lines that report no response
// with usage (Scan.foldRefusalMessages folds the rest into theirs), which
// Build otherwise drops with every zero-token response. A count, no dollars:
// in a category the API bills before any output such a refusal WAS billed, at
// the rates of the model that ran it, and the amount is not in the
// transcript. Dropping them printed "refusals none" beside a transcript that
// held a billed bio refusal.
type Refusals struct {
	Responses    int            `json:"responses"`
	Cost         Cost           `json:"cost"`
	NotBilled    int            `json:"not_billed"`
	WithoutUsage int            `json:"without_usage"`
	ByCategory   []RefusalGroup `json:"by_category"`
}

// RefusalGroup is one category's refusals on one model: a classifier decline
// in a named category is told apart from the rest. BilledBeforeOutput is
// whether the API bills a pre-output refusal in the category, null for a
// category this read does not know. Model is ModelNotRecorded for Claude
// Code's "<synthetic>" refusal line.
type RefusalGroup struct {
	Category           string `json:"category"`
	Model              string `json:"model"`
	Responses          int    `json:"responses"`
	Cost               Cost   `json:"cost"`
	NotBilled          int    `json:"not_billed"`
	WithoutUsage       int    `json:"without_usage"`
	BilledBeforeOutput *bool  `json:"billed_before_output"`
}

// ExtraAttempts is responses whose usage.iterations holds more than one
// attempt, and the responses a fallback model served.
//
// The refusals-and-fallback page documents every iteration entry as carrying
// a type and the model that ran it, bills "every attempt that produced
// output, including one that declined partway through its response" at the
// rates of the model that ran it, and bills an attempt declined before any
// output "only when its refusal category is billed". So an extra attempt
// that produced output is priced at its own model's rates and is in the
// total, the breakdowns and its model's by-model row (Cost's priced part).
// One with no output is tokens with the dollars unknown and not in the total
// (Cost's unpriced part, which counts attempts): its category -- which
// decides whether it was billed -- is on no entry, and the response's own
// stop_details describes only the attempt that produced it. So is one on a
// model the table lacks.
//
// Declined is the attempts that declined -- the "message" entries before the
// "fallback_message" that served -- by the model that ran them. Fallback is
// every response a fallback served, as the model asked and the model that
// served, sticky-routed ones included (route).
type ExtraAttempts struct {
	Responses         int                `json:"responses"`
	Attempts          int                `json:"attempts"`
	Tokens            Tokens             `json:"tokens"`
	Cost              Cost               `json:"cost"`
	CostUnknownReason string             `json:"cost_unknown_reason"`
	Declined          []DeclinedAttempts `json:"declined"`
	Fallback          []FallbackRoute    `json:"fallback_served"`
}

// AttemptsUnpriced is ExtraAttempts' reason, verbatim.
const AttemptsUnpriced = "an extra attempt that produced output is priced at the rates of the model its iteration entry names and is in the total; one declined before any output is billed only in some refusal categories, which the transcript does not record for it, and one on a model the price table lacks has no known rate, so those are tokens with the cost unknown and not in the total"

// DeclinedAttempts is one model's declined attempts. Cost's priced part is the
// attempts that produced output (in the total); its unpriced part counts the
// rest, attempts rather than responses. NoOutput counts the attempts declined
// before any output.
type DeclinedAttempts struct {
	Model    string `json:"model"`
	Attempts int    `json:"attempts"`
	Tokens   Tokens `json:"tokens"`
	Cost     Cost   `json:"cost"`
	NoOutput int    `json:"no_output"`
}

// FallbackRoute is the responses a fallback served, by the model asked and
// the model that served. Sticky is a route with no "message" entry: the
// request went straight to the fallback, and Requested is "" because the
// transcript does not say which model was asked.
type FallbackRoute struct {
	Requested string `json:"requested"`
	Served    string `json:"served"`
	Sticky    bool   `json:"sticky"`
	Responses int    `json:"responses"`
}

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
type SessionSpend struct {
	SessionID string `json:"session_id"`
	Main      Cost   `json:"main"`
	Subagents Cost   `json:"subagents"`
	// Coverage says whether rashomon recorded the session's transcripts
	// (CoverageRecorded, CoveragePartly, CoverageNotRecorded). Join sets
	// it; it is absent when no store was consulted.
	Coverage string `json:"coverage,omitempty"`

	id string // the session id as read, which SessionID may have replaced
}

// Saving is one suggestion, with the figure it rests on. A suggestion with
// no figure is not made.
type Saving struct {
	Kind string `json:"kind"`
	// Category and Model narrow a refusal or declined-attempt saving to one
	// refusal category (a closed word) and one model (displayModel).
	Category string `json:"category,omitempty"`
	Model    string `json:"model,omitempty"`
	Cost     Cost   `json:"cost"`
	// Hint is a closed word for advice the figure supports, or empty. It is
	// given only where a Claude Code user can act on it.
	Hint string `json:"hint,omitempty"`
}

// Saving kinds.
const (
	SavingColdCache     = "cold_cache_rewrites"
	SavingSilentFailure = "silently_failed_turns"
	// SavingBilledRefusals is what refusals with usage cost, by category and
	// model: the design's "classifier hits".
	SavingBilledRefusals = "billed_refusals"
	// SavingDeclinedAttempts is what the priced declined attempts before a
	// fallback cost, by the model that declined.
	SavingDeclinedAttempts = "declined_attempts"
)

// Savings that were billed but carry no amount in the transcript, named in
// savings_not_computed rather than left out without a word. (A pre-output
// refusal in a billed category written with no usage is not here: its
// category and model are known, so it is a billed_refusals saving whose cost
// is unknown.)
const (
	// SavingNotComputedAttempts: declined attempts with no output, billed
	// only in some refusal categories, which no entry records.
	SavingNotComputedAttempts = "declined_attempts_without_output"
)

// ModelNotRecorded names the model of a refusal Claude Code wrote as a
// "<synthetic>" line: the line does not say which model refused.
const ModelNotRecorded = "not_recorded"

// SavingHintReasoningInReply: the refusals were in the reasoning_extraction
// category, which the refusals-and-fallback page describes as a request that
// "asks the model to reproduce its internal reasoning in the response text"
// and answers with thinking. A Claude Code user can stop asking for that.
// The other categories name a policy area, and benign work can trigger them:
// no lever a user holds, so no hint.
const SavingHintReasoningInReply = "reasoning_in_reply"

// SavingHintServedModel: a fallback served requests this model declined, and
// a Claude Code user can choose that model (/model) for such work, which
// skips the declined attempt. Given only when a route names this model as
// the model asked: a sticky-routed turn does not say which model was asked.
const SavingHintServedModel = "served_model"

// SavingHintLongerTTL: part of the re-written cache was written with the 5m
// TTL, which the 1h TTL would have kept across a pause under an hour. Never
// given when every re-write was already 1h: on real data every one was, after
// gaps of hours to days, and advising the TTL they already had was advice
// with no figure under it.
const SavingHintLongerTTL = "1h_ttl"

// priced is a response's cost broken down by kind, or ok=false for a model
// the table does not know.
type priced struct {
	input, output, cacheWrite, cacheRead int64
	ok                                   bool
}

func (p priced) total() int64 { return p.input + p.output + p.cacheWrite + p.cacheRead }

// price prices a response at its model's rates; a pre-output refusal in a
// category whose billing is unknown is priced as unknown, whatever its model.
func price(r *Response) priced {
	if r.costUnknown {
		return priced{}
	}
	return priceTokens(r.Model, r.Tokens)
}

func priceTokens(model string, t Tokens) priced {
	key, ok := PriceKey(model)
	if !ok {
		return priced{}
	}
	rt, _ := RatesFor(key)
	return priced{
		input:      t.Input * rt.Input,
		output:     t.Output * rt.Output,
		cacheWrite: t.CacheWrite5m*rt.CacheWrite5m() + t.CacheWrite1h*rt.CacheWrite1h(),
		cacheRead:  t.CacheRead * rt.CacheRead,
		ok:         true,
	}
}

// costOf adds one response to c -- its priced total, or an unpriced mark
// carrying its tokens -- and the priced part of each of its extra attempts
// (attemptPrice), which the API bills beside it.
func costOf(c *Cost, r *Response) {
	costOne(c, r)
	for _, a := range r.Attempts {
		if p, ok := attemptPrice(a); ok {
			c.addPriced(p.total())
		}
	}
}

// costOne adds the response alone, without its extra attempts: a by-model
// row, where each attempt is its own model's.
func costOne(c *Cost, r *Response) {
	p := price(r)
	if p.ok {
		c.addPriced(p.total())
		return
	}
	c.addUnpriced(r.Tokens.Total())
}

// attemptPrice prices an extra attempt at the rates of the model that ran
// it, when it produced output and the table knows that model. An attempt
// with no output was billed only if its refusal category is billed, which no
// entry records, so it is never priced here (ExtraAttempts).
func attemptPrice(a Attempt) (priced, bool) {
	if a.Tokens.Output == 0 {
		return priced{}, false
	}
	p := priceTokens(a.Model, a.Tokens)
	return p, p.ok
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
// in a category that is not billed is likewise counted, in
// Refusals.NotBilled, and nowhere else.
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
		ByModel:            []ModelSpend{},
		Savings:            []Saving{},
		SavingsNotComputed: []string{},
		PerSession:         []SessionSpend{},
		ExtraAttempts: ExtraAttempts{
			CostUnknownReason: AttemptsUnpriced,
			Declined:          []DeclinedAttempts{},
			Fallback:          []FallbackRoute{},
		},
		CacheExpiry: CacheExpiry{Heuristic: CacheHeuristic},
		Refusals:    Refusals{ByCategory: []RefusalGroup{}},
		FastMode:    FastMode{Pricing: FastModePricing},
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
	declined := map[string]*DeclinedAttempts{}
	routes := map[FallbackRoute]int{}
	refusals := map[[2]string]*RefusalGroup{}
	refusal := func(r *Response) *RefusalGroup {
		k := [2]string{r.Category, refusalModel(r.Model)}
		g := refusals[k]
		if g == nil {
			g = &RefusalGroup{Category: k[0], Model: k[1]}
			if billed, known := BilledBeforeOutput(k[0]); known {
				g.BilledBeforeOutput = &billed
			}
			refusals[k] = g
		}
		return g
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
			}
			continue
		}
		if r.notBilled {
			s.Refusals.NotBilled++
			refusal(r).NotBilled++
			continue
		}
		s.window = append(s.window, r)
		s.Responses++
		s.Tokens.add(r.Tokens)
		costOf(&s.Total, r)

		sess, ok := sessions[r.SessionID]
		if !ok {
			sess = &SessionSpend{SessionID: displaySession(r.SessionID), id: r.SessionID}
			sessions[r.SessionID] = sess
		}
		if r.Subagent {
			costOf(&s.ByAgent.Subagents, r)
			costOf(&sess.Subagents, r)
		} else {
			costOf(&s.ByAgent.Main, r)
			costOf(&sess.Main, r)
		}

		model := func(raw string) *ModelSpend {
			name, known := displayModel(raw)
			m, ok := models[name]
			if !ok {
				m = &ModelSpend{Model: name, Priced: known}
				models[name] = m
			}
			return m
		}
		m := model(r.Model)
		m.Responses++
		m.Tokens.add(r.Tokens)
		costOne(&m.Cost, r)

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

		// Each extra attempt that produced output, at the rates of the model
		// that ran it: in the total (costOf, above), its model's row and the
		// kinds. The rest are tokens with the cost unknown.
		for _, a := range r.Attempts {
			ap, ok := attemptPrice(a)
			if !ok {
				continue
			}
			s.Tokens.add(a.Tokens)
			am := model(a.Model)
			am.Tokens.add(a.Tokens)
			am.Cost.addPriced(ap.total())
			for _, k := range []struct {
				c    *Cost
				nano int64
				toks int64
			}{
				{&s.ByKind.Input, ap.input, a.Tokens.Input},
				{&s.ByKind.Output, ap.output, a.Tokens.Output},
				{&s.ByKind.CacheWrite, ap.cacheWrite, a.Tokens.CacheWrite5m + a.Tokens.CacheWrite1h},
				{&s.ByKind.CacheRead, ap.cacheRead, a.Tokens.CacheRead},
			} {
				if k.toks > 0 {
					k.c.addPriced(k.nano)
				}
			}
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

		if r.StopReason == "refusal" {
			s.Refusals.Responses++
			costOne(&s.Refusals.Cost, r)
			g := refusal(r)
			g.Responses++
			costOne(&g.Cost, r)
		}
		if r.Fast {
			s.FastMode.Responses++
		}
		s.ExtraAttempts.add(r, declined, routes)
	}
	s.Sessions = len(sessions)
	s.Refusals.ByCategory = sortedRefusals(refusals)
	s.ExtraAttempts.Declined = sortedDeclined(declined)
	s.ExtraAttempts.Fallback = sortedRoutes(routes)

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

// add counts one windowed response's extra attempts and its route.
func (e *ExtraAttempts) add(r *Response, declined map[string]*DeclinedAttempts, routes map[FallbackRoute]int) {
	if r.Fallback {
		k := FallbackRoute{Served: displayName(r.Model), Sticky: r.Requested == ""}
		if !k.Sticky {
			k.Requested = displayName(r.Requested)
		}
		routes[k]++
	}
	if len(r.Attempts) == 0 {
		return
	}
	e.Responses++
	e.Attempts += len(r.Attempts)
	for _, a := range r.Attempts {
		e.Tokens.add(a.Tokens)
		p, ok := attemptPrice(a)
		if ok {
			e.Cost.addPriced(p.total())
		} else {
			e.Cost.addUnpriced(a.Tokens.Total())
		}
		// A "message" entry before the "fallback_message" that served is a
		// hop that declined.
		if !r.Fallback || a.Type != IterMessage {
			continue
		}
		name := displayName(a.Model)
		d := declined[name]
		if d == nil {
			d = &DeclinedAttempts{Model: name}
			declined[name] = d
		}
		d.Attempts++
		d.Tokens.add(a.Tokens)
		if ok {
			d.Cost.addPriced(p.total())
		} else {
			d.Cost.addUnpriced(a.Tokens.Total())
		}
		if a.Tokens.Output == 0 {
			d.NoOutput++
		}
	}
}

// refusalModel is how a refusal group names its model: ModelNotRecorded for
// Claude Code's "<synthetic>" line, which names none, and displayModel's name
// otherwise -- "other" read as a model of that name.
func refusalModel(model string) string {
	if model == "<synthetic>" {
		return ModelNotRecorded
	}
	return displayName(model)
}

// BilledWithoutAmount is the pre-output refusals in a billed category written
// with no usage: billed, and not in the total, since the transcript does not
// hold the amount.
func (r Refusals) BilledWithoutAmount() int {
	n := 0
	for _, g := range r.ByCategory {
		if g.BilledBeforeOutput != nil && *g.BilledBeforeOutput {
			n += g.WithoutUsage
		}
	}
	return n
}

// displayName is displayModel's name alone.
func displayName(model string) string {
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

func sortedDeclined(m map[string]*DeclinedAttempts) []DeclinedAttempts {
	out := []DeclinedAttempts{}
	for _, d := range m {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	return out
}

func sortedRoutes(m map[FallbackRoute]int) []FallbackRoute {
	out := []FallbackRoute{}
	for k, n := range m {
		k.Responses = n
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Requested != b.Requested {
			return a.Requested < b.Requested
		}
		return a.Served < b.Served
	})
	return out
}

// buildSavings lists the suggestions a number supports, and only those.
//
// Cold-cache re-writes, spend in silently failed turns, and -- the design's
// classifier hits -- billed refusals and declined attempts, by category and
// model. The design's last one -- subagents on
// the top model where their tool pattern is read-heavy -- does not: which
// tools a subagent called is in message.content, which this package never
// reads, so "read-heavy" would be a claim with no number under it, and a
// suggestion printed without its figure is exactly what this list refuses.
func (s *Summary) buildSavings() {
	s.Savings = s.Savings[:0]
	if s.CacheExpiry.Cost.Nano > 0 {
		sv := Saving{Kind: SavingColdCache, Cost: s.CacheExpiry.Cost}
		if s.CacheExpiry.Tokens5m > 0 {
			sv.Hint = SavingHintLongerTTL
		}
		s.Savings = append(s.Savings, sv)
	}
	if s.SilentFailureTurns.Cost.Nano > 0 {
		s.Savings = append(s.Savings, Saving{Kind: SavingSilentFailure, Cost: s.SilentFailureTurns.Cost})
	}

	// Refusals and declined attempts, by category and model. A billed
	// refusal's figure is its priced part; a pre-output refusal in a billed
	// category written with no usage was billed too, so it is in the same
	// entry as an unpriced count -- the cost unknown, the category, model and
	// lever known. What else was billed with no amount in the transcript is
	// named in SavingsNotComputed.
	s.SavingsNotComputed = s.SavingsNotComputed[:0]
	for _, g := range s.Refusals.ByCategory {
		c := Cost{Nano: g.Cost.Nano, Priced: g.Cost.Priced}
		if g.BilledBeforeOutput != nil && *g.BilledBeforeOutput {
			c.Unpriced = g.WithoutUsage
		}
		if c.Nano == 0 && c.Unpriced == 0 {
			continue
		}
		sv := Saving{Kind: SavingBilledRefusals, Category: g.Category, Model: g.Model, Cost: c}
		if g.Category == CategoryReasoningExtraction {
			sv.Hint = SavingHintReasoningInReply
		}
		s.Savings = append(s.Savings, sv)
	}
	asked := map[string]bool{}
	for _, r := range s.ExtraAttempts.Fallback {
		if !r.Sticky {
			asked[r.Requested] = true
		}
	}
	noOutput := false
	for _, d := range s.ExtraAttempts.Declined {
		if d.NoOutput > 0 {
			noOutput = true
		}
		if d.Cost.Nano == 0 {
			continue
		}
		// Only the priced part: the unpriced attempts are not in the figure.
		c := Cost{Nano: d.Cost.Nano, Priced: d.Cost.Priced}
		sv := Saving{Kind: SavingDeclinedAttempts, Model: d.Model, Cost: c}
		if asked[d.Model] {
			sv.Hint = SavingHintServedModel
		}
		s.Savings = append(s.Savings, sv)
	}
	if noOutput {
		s.SavingsNotComputed = append(s.SavingsNotComputed, SavingNotComputedAttempts)
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
// stream, which can only make a gap look shorter, never invent one. The window is
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
// A write on a response that read anything from the cache is not cold: the
// cache was warm, and the write only added the new tokens after the cached
// prefix. Counting it whole priced a 1k write on a warm cache as an expiry,
// and on real data overstated the figure by single-digit percent to about
// 15%. So a write counts only when cache_read_input_tokens is zero, and it
// is priced as the write rate minus the read rate (Build): what re-reading
// those tokens would have cost is not a saving.
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
			if rs[i].Tokens.CacheRead > 0 {
				continue
			}
			gap := time.Duration(rs[i].StartMS-rs[i-1].StartMS) * time.Millisecond
			var w Tokens
			if gap > ttl5m {
				w.CacheWrite5m = rs[i].Tokens.CacheWrite5m
			}
			if gap > ttl1h {
				w.CacheWrite1h = rs[i].Tokens.CacheWrite1h
			}
			if w.CacheWrite5m+w.CacheWrite1h > 0 {
				out[rs[i]] = w
			}
		}
	}
	return out
}
