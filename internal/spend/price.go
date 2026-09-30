package spend

import "strings"

// PriceSnapshot is the date the rate table below was copied from Anthropic's
// published API list prices. Every rendering of a dollar figure names it,
// because a price table compiled into a binary is right on the day it was
// written and only on that day: a reader must be able to see how old the
// number's premise is without reading this file.
const PriceSnapshot = "2026-09-25"

// Rates are nanodollars per token, which is $/MTok x 1000.
//
// Integers, not float64 dollars: every rate in the published table, and every
// multiple of one this package applies (1.25x, 2x, 0.1x), is a whole number of
// nanodollars per token, so every cost here is exact integer arithmetic and a
// test can assert an exact figure rather than a tolerance. A float would make
// "the same usage priced twice" able to disagree with itself in the last
// digit, and a test that allowed for that would also allow for a real error of
// the same size.
type Rates struct {
	Input  int64
	Output int64
	// CacheRead is its own column rather than a multiple of Input: the table
	// states it absolutely for some models (0.25 against 10) and as 0.1x for
	// others, and those are not the same rule (0.25 is not 0.1 x 10).
	CacheRead int64
}

// CacheWrite5m and CacheWrite1h are the cache-write rates: 1.25x input for a
// 5-minute TTL and 2x input for a 1-hour TTL, the same multipliers for every
// model in the table. Derived rather than tabulated so the table cannot hold
// a write rate that disagrees with its own input rate.
func (r Rates) CacheWrite5m() int64 { return r.Input * 5 / 4 }
func (r Rates) CacheWrite1h() int64 { return r.Input * 2 }

// mtok converts a $/MTok figure to nanodollars per token. Written as a
// function of the published figure (in hundredths, so 0.25 is 25) so each row
// below reads as the table it was copied from.
func mtok(hundredths int64) int64 { return hundredths * 10 }

// table is the snapshot, keyed by the model id as the API names it.
//
// Out of scope and said to be: fast mode's premium, the Batch API's discount,
// long-context premiums, partner pricing (Bedrock and Vertex), and web
// search's per-search fee. Not every one is invisible in a transcript:
// usage.speed identifies a fast-mode response and usage.service_tier a Batch
// one. Fast-mode responses are counted and the output says they are priced
// here at standard rates (FastMode); Claude Code does not send its requests
// through the Batch API. Long-context premiums (which Sonnet 4.5 and Sonnet 4
// carry, unlike 4.6 and later) and partner pricing have no field, and
// usage.server_tool_use.web_search_requests is not read. Pricing at plain list
// rates is a stated basis where the premium would be a guess or is not read.
//
// Retired models keep their rows: a transcript inside the window can still
// carry one (they remain served on Bedrock and Google Cloud), and a row the
// table lacks prices a response as unknown. Keyed by the id the API returns
// in message.model, so Opus 4 and Sonnet 4 -- whose responses name the dated
// "claude-opus-4-20250514" -- are keyed without the "-0" of their aliases,
// and Haiku 3.5 by its older "claude-3-5-haiku" shape.
var table = map[string]Rates{
	"claude-fable-5-1": {Input: mtok(1000), Output: mtok(5000), CacheRead: mtok(25)},
	"claude-fable-5":   {Input: mtok(1000), Output: mtok(5000), CacheRead: mtok(100)},
	"claude-opus-5-5":  {Input: mtok(400), Output: mtok(2000), CacheRead: mtok(20)},

	"claude-opus-5":   {Input: mtok(500), Output: mtok(2500), CacheRead: mtok(50)},
	"claude-opus-4-8": {Input: mtok(500), Output: mtok(2500), CacheRead: mtok(50)},
	"claude-opus-4-7": {Input: mtok(500), Output: mtok(2500), CacheRead: mtok(50)},
	"claude-opus-4-6": {Input: mtok(500), Output: mtok(2500), CacheRead: mtok(50)},
	"claude-opus-4-5": {Input: mtok(500), Output: mtok(2500), CacheRead: mtok(50)},
	"claude-opus-4-1": {Input: mtok(1500), Output: mtok(7500), CacheRead: mtok(150)},
	"claude-opus-4":   {Input: mtok(1500), Output: mtok(7500), CacheRead: mtok(150)},

	"claude-sonnet-5-5": {Input: mtok(200), Output: mtok(1000), CacheRead: mtok(20)},
	"claude-sonnet-5":   {Input: mtok(200), Output: mtok(1000), CacheRead: mtok(20)},
	"claude-sonnet-4-6": {Input: mtok(300), Output: mtok(1500), CacheRead: mtok(30)},
	"claude-sonnet-4-5": {Input: mtok(300), Output: mtok(1500), CacheRead: mtok(30)},
	"claude-sonnet-4":   {Input: mtok(300), Output: mtok(1500), CacheRead: mtok(30)},

	"claude-haiku-4-5": {Input: mtok(100), Output: mtok(500), CacheRead: mtok(10)},
	"claude-3-5-haiku": {Input: mtok(80), Output: mtok(400), CacheRead: mtok(8)},
}

// PriceKey resolves a transcript's message.model to a row of the table, and
// reports whether one matched.
//
// CONSERVATIVE ON PURPOSE. A model id matches a row when it IS that row's id,
// or that id followed by exactly one dated snapshot suffix, "-YYYYMMDD" (real
// transcripts carry "claude-haiku-4-5-20251001"). Nothing looser. A prefix
// match is the obvious shortcut and it is wrong in this very table:
// "claude-opus-5-5" begins with "claude-opus-5", and a prefix rule would price
// Opus 5.5 at Opus 5's rates -- 25% high on input, silently, on the model most
// sessions use. Anything else -- a "[1m]" context marker, a provider-prefixed
// id, a model this table never heard of -- is left unmatched, and an unmatched
// model is priced as unknown, never as zero and never as its nearest
// neighbour. An unknown cost is a gap the reader can see; a wrong cost is not.
func PriceKey(model string) (string, bool) {
	if _, ok := table[model]; ok {
		return model, true
	}
	i := strings.LastIndexByte(model, '-')
	if i < 0 || !isDate(model[i+1:]) {
		return "", false
	}
	base := model[:i]
	if _, ok := table[base]; ok {
		return base, true
	}
	return "", false
}

// isDate reports whether s is exactly eight ASCII digits. It does not check
// the digits form a calendar date: the suffix is the API's snapshot label,
// matched for its shape, and a table key followed by eight digits has no other
// reading.
func isDate(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// RatesFor is the table row for a resolved key.
func RatesFor(key string) (Rates, bool) {
	r, ok := table[key]
	return r, ok
}
