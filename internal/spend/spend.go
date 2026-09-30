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
	// SavingsNotComputed names the kinds of saving the list never holds
	// (SavingNotComputedRefusals), so its absence is not a finding.
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
const CacheHeuristic = "a cache write on a response whose previous response by the same agent in the same transcript started more than the TTL earlier (5m, or 1h for a 1h write)"

// Refusals is responses that ended with stop_reason "refusal".
//
// WithoutUsage counts the refusal lines Claude Code writes with no usage at
// all -- a pre-output classifier refusal is one zero-usage "<synthetic>" line
// -- which Build otherwise drops with every zero-token response. A count only:
// whether such a refusal was billed depends on its category (the API bills a
// pre-output refusal in some categories, at the rates of the model that ran
// it), and the line carries no usage to read that from. Dropping them printed
// "refusals none" beside a transcript that held one.
type Refusals struct {
	Responses    int  `json:"responses"`
	Cost         Cost `json:"cost"`
	WithoutUsage int  `json:"without_usage"`
}

// ExtraAttempts is responses whose usage.iterations holds more than one
// attempt.
//
// Tokens, never dollars, and not in the total. The API documents every
// iteration entry as carrying a type and the model that ran it, and each
// attempt as billed at that model's rates -- a declined attempt at the
// declining model's, the fallback that served at the fallback's -- so the
// dollars CAN be read from the transcript. This read does not decode the
// entries' type or model yet: no real transcript with more than one entry has
// been seen to check the decode against. Until one is, the dollars are
// unknown, the headline says the total leaves the attempts out, and the
// document says why rather than guessing a model.
//
// "Extra", never "declined": an entry's type is not read here, so calling
// every earlier attempt a declined one would be a claim about each entry the
// read does not make.
type ExtraAttempts struct {
	Responses         int    `json:"responses"`
	Attempts          int    `json:"attempts"`
	Tokens            Tokens `json:"tokens"`
	CostUnknownReason string `json:"cost_unknown_reason"`
}

// AttemptsUnpriced is ExtraAttempts' reason, verbatim.
const AttemptsUnpriced = "each iteration entry names the model that ran it, and an attempt bills at that model's rates, but this read does not decode the entries' model yet, so extra attempts are not priced and not in the total"

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
	Cost Cost   `json:"cost"`
	// Hint is a closed word for advice the figure supports, or empty.
	Hint string `json:"hint,omitempty"`
}

// Saving kinds.
const (
	SavingColdCache     = "cold_cache_rewrites"
	SavingSilentFailure = "silently_failed_turns"
)

// SavingNotComputedRefusals names the savings the list does not compute:
// what refusals and fallback routing (classifier hits) cost that a different
// model or setup would not have. The design asks for them; they need the
// iteration entries' model and the refusals' category, which this read does
// not decode (ExtraAttempts, Refusals). Stated in savings_not_computed, and
// in the text beside any refusal or extra attempt, so an empty list is not
// read as "nothing to save there".
const SavingNotComputedRefusals = "refusals_and_routing"

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
// ended in a refusal is a pre-output refusal, counted in
// Refusals.WithoutUsage.
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
		SavingsNotComputed: []string{SavingNotComputedRefusals},
		PerSession:         []SessionSpend{},
		ExtraAttempts: ExtraAttempts{
			CostUnknownReason: AttemptsUnpriced,
		},
		CacheExpiry: CacheExpiry{Heuristic: CacheHeuristic},
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
			}
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

		if w, ok := cold[r]; ok {
			s.CacheExpiry.Responses++
			s.CacheExpiry.Tokens += w.CacheWrite5m + w.CacheWrite1h
			s.CacheExpiry.Tokens5m += w.CacheWrite5m
			s.CacheExpiry.Tokens1h += w.CacheWrite1h
			if key, ok := PriceKey(r.Model); ok {
				rt, _ := RatesFor(key)
				s.CacheExpiry.Cost.addPriced(w.CacheWrite5m*rt.CacheWrite5m() + w.CacheWrite1h*rt.CacheWrite1h())
			} else {
				s.CacheExpiry.Cost.addUnpriced(w.CacheWrite5m + w.CacheWrite1h)
			}
		}

		if r.StopReason == "refusal" {
			s.Refusals.Responses++
			costOf(&s.Refusals.Cost, r)
		}
		if r.Fast {
			s.FastMode.Responses++
		}
		if r.ExtraAttempts > 0 {
			s.ExtraAttempts.Responses++
			s.ExtraAttempts.Attempts += r.ExtraAttempts
			s.ExtraAttempts.Tokens.add(r.ExtraTokens)
		}
	}
	s.Sessions = len(sessions)

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

// buildSavings lists the suggestions a number supports, and only those.
//
// Two of the design's three have a figure here. The third -- subagents on
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
		k := stream{r.file, r.Subagent}
		byFile[k] = append(byFile[k], r)
	}
	out := map[*Response]Tokens{}
	for _, rs := range byFile {
		sort.SliceStable(rs, func(i, j int) bool { return rs[i].StartMS < rs[j].StartMS })
		for i := 1; i < len(rs); i++ {
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
