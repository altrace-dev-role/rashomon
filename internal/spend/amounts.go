package spend

// nanoUSD is an amount in nanodollars and tokenCount a number of tokens.
// Both used to be bare int64, and they meet in one place -- a kind of
// token's price beside its count -- where a call that passed the count as the
// price compiled, passed and printed tokens as dollars. As two types, and
// made together by price alone, they cannot be taken for each other.
type (
	nanoUSD    int64
	tokenCount int64
)

// kindCost is one kind of token in a response: its count, and its price when
// the model is priced (zero when it is not).
type kindCost struct {
	nano   nanoUSD
	tokens tokenCount
}

// priced is a response's tokens and cost broken down by kind, with ok=false
// for a model the table does not know.
type priced struct {
	input, output, cacheWrite, cacheRead kindCost
	ok                                   bool
}

func (p priced) total() nanoUSD {
	return p.input.nano + p.output.nano + p.cacheWrite.nano + p.cacheRead.nano
}

func (p priced) tokens() tokenCount {
	return p.input.tokens + p.output.tokens + p.cacheWrite.tokens + p.cacheRead.tokens
}

// price prices a response at its model's rates. It is the one place a token
// count becomes a tokenCount and a rate times a count becomes a nanoUSD.
func price(r *Response) priced {
	t := r.Tokens
	p := priced{
		input:      kindCost{tokens: tokenCount(t.Input)},
		output:     kindCost{tokens: tokenCount(t.Output)},
		cacheWrite: kindCost{tokens: tokenCount(t.CacheWrite5m + t.CacheWrite1h)},
		cacheRead:  kindCost{tokens: tokenCount(t.CacheRead)},
	}
	key, ok := PriceKey(r.Model)
	if !ok {
		return p
	}
	// PriceKey only returns a key the table holds, so the row is there.
	rt, _ := RatesFor(key)
	p.input.nano = nanoUSD(t.Input * rt.Input)
	p.output.nano = nanoUSD(t.Output * rt.Output)
	p.cacheWrite.nano = nanoUSD(t.CacheWrite5m*rt.CacheWrite5m() + t.CacheWrite1h*rt.CacheWrite1h())
	p.cacheRead.nano = nanoUSD(t.CacheRead * rt.CacheRead)
	p.ok = true
	return p
}

// priceColdWrites prices what a response's cold cache writes cost over the
// cache reads they replaced (CacheExpiry), the second place a count becomes
// money, and the only other. ok is false for a model the table does not
// know, or a response whose cost is unknown.
func priceColdWrites(r *Response, w Tokens) (nanoUSD, tokenCount, bool) {
	tokens := tokenCount(w.CacheWrite5m + w.CacheWrite1h)
	key, ok := PriceKey(r.Model)
	if !ok || r.costUnknown {
		return 0, tokens, false
	}
	rt, _ := RatesFor(key) // PriceKey resolved it, so the row exists
	return nanoUSD(w.CacheWrite5m*(rt.CacheWrite5m()-rt.CacheRead) + w.CacheWrite1h*(rt.CacheWrite1h()-rt.CacheRead)), tokens, true
}

// costOf adds one response to c: its priced total, or an unpriced mark
// carrying its tokens.
func costOf(c *Cost, r *Response) {
	p := price(r)
	if p.ok {
		c.addPriced(p.total())
		return
	}
	c.addUnpriced(p.tokens())
}
