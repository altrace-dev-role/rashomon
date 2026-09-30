// Package spend estimates what the last N days of Claude Code usage would
// cost at Anthropic's API list prices, from Claude Code's own transcripts.
//
// ZERO SETUP. It cannot depend on `watch` having run, so its source is
// Claude Code's transcripts (see scan.go for the exact, narrow read of usage,
// and for the one read of a turn's final message the silent-failure line
// needs), not rashomon's store. The store is consulted for exactly one line -- spend in
// turns that ended with a failure the summary never mentioned -- and only
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

	SilentFailureTurns SilentFailureTurns `json:"silent_failure_turns"`

	Savings    []Saving       `json:"savings"`
	PerSession []SessionSpend `json:"per_session"`

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
	Cost      Cost   `json:"cost"`
}

// CacheHeuristic is the rule, stated wherever its number is.
const CacheHeuristic = "a cache write on a response whose previous response in the same transcript started more than the TTL earlier (5m, or 1h for a 1h write)"

// Refusals is responses that ended with stop_reason "refusal".
type Refusals struct {
	Responses int  `json:"responses"`
	Cost      Cost `json:"cost"`
}

// ExtraAttempts is responses whose usage.iterations holds more than one
// attempt.
//
// Tokens, never dollars. The iteration entries seen so far carry a type but
// no model, and a declined attempt retried on a fallback model bills at the
// FALLBACK's rates, so which rate applies to an attempt cannot be read from
// the transcript. Until a real transcript shows that shape, the dollars are
// unknown and the document says so rather than guessing a model.
type ExtraAttempts struct {
	Responses         int    `json:"responses"`
	Attempts          int    `json:"attempts"`
	Tokens            Tokens `json:"tokens"`
	CostUnknownReason string `json:"cost_unknown_reason"`
}

// AttemptsUnpriced is ExtraAttempts' reason, verbatim.
const AttemptsUnpriced = "iteration entries carry no model, so the rate a declined attempt billed at cannot be read"

// SessionSpend is one session's split.
type SessionSpend struct {
	SessionID string `json:"session_id"`
	Main      Cost   `json:"main"`
	Subagents Cost   `json:"subagents"`
}

// Saving is one suggestion, with the figure it rests on. A suggestion with
// no figure is not made.
type Saving struct {
	Kind string `json:"kind"`
	Cost Cost   `json:"cost"`
}

// Saving kinds.
const (
	SavingColdCache     = "cold_cache_rewrites"
	SavingSilentFailure = "silently_failed_turns"
)

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
// A response with no tokens at all is not counted anywhere. Claude Code
// writes synthetic assistant lines (model "<synthetic>") for local errors
// with an all-zero usage; nothing was billed for them, and listing a model
// named "other" with zero tokens and an unknown cost would be noise that
// looks like a finding.
func Build(sc *Scan, now time.Time, days int) *Summary {
	from := now.Add(-time.Duration(days) * 24 * time.Hour)
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
		ByModel:    []ModelSpend{},
		Savings:    []Saving{},
		PerSession: []SessionSpend{},
		ExtraAttempts: ExtraAttempts{
			CostUnknownReason: AttemptsUnpriced,
		},
		CacheExpiry: CacheExpiry{Heuristic: CacheHeuristic},
		Read: ReadStats{
			Files:             len(sc.Files),
			UsageLines:        sc.UsageLines,
			DistinctResponses: len(sc.Responses),
			UnreadableFiles:   sc.Unreadable,
			UndatedResponses:  sc.Undated,
		},
		scan: sc,
	}
	s.SilentFailureTurns = SilentFailureTurns{Store: StoreNotConsulted}

	cold := coldWrites(sc)

	models := map[string]*ModelSpend{}
	sessions := map[string]*SessionSpend{}
	for _, r := range sc.Responses {
		if r.StartMS == 0 || r.StartMS < s.FromUnixMS || r.Tokens.Total() == 0 {
			continue
		}
		s.window = append(s.window, r)
		s.Responses++
		s.Tokens.add(r.Tokens)
		costOf(&s.Total, r)

		sess, ok := sessions[r.SessionID]
		if !ok {
			sess = &SessionSpend{SessionID: displaySession(r.SessionID)}
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
		s.Savings = append(s.Savings, Saving{Kind: SavingColdCache, Cost: s.CacheExpiry.Cost})
	}
	if s.SilentFailureTurns.Cost.Nano > 0 {
		s.Savings = append(s.Savings, Saving{Kind: SavingSilentFailure, Cost: s.SilentFailureTurns.Cost})
	}
}

// coldWrites applies the cache-expiry heuristic to every response in the
// scan (the window is applied by the caller) and returns, per response, the
// cache-write tokens it attributes to a cold cache.
//
// Per TRANSCRIPT FILE, in start order: a response's previous response is the
// one before it in the same file, which is the same conversation's previous
// request -- a subagent's first request follows nothing, because a new
// context is a cold start by construction, not an expiry. The window is
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
	byFile := map[int][]*Response{}
	for _, r := range sc.Responses {
		if r.StartMS == 0 || r.Tokens.Total() == 0 {
			continue
		}
		byFile[r.file] = append(byFile[r.file], r)
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
