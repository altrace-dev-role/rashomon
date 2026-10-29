// Package share builds the bundles a user may choose to hand on (PRD PF-SH1,
// PF-SH2). It reads the store and returns a document; the CLI prints and
// writes it, and sending it is not rashomon's (Altrace out of rashomon,
// 10-23): no endpoint, no client, no network package in this graph (TRU-40b).
package share

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// SchemaOutcomes is the outcomes bundle's schema. Version 1 counts fired lines
// alone: fixed, ignored, overridden and precision are the user's labels,
// which MEASURE does not write yet, so they are 0 and print as under the
// floor. The version says so, not a free-text field (D114).
const SchemaOutcomes = "outcomes/1"

// countFloor is the smallest count a bundle prints as a number (PF-SH2).
const countFloor = 5

// Count is a count as a bundle prints it: a number at countFloor or above,
// the string "<5" below it, so a small count does not single out a session.
type Count int64

// MarshalJSON prints c as a JSON number at countFloor or above and as "<5"
// below it.
func (c Count) MarshalJSON() ([]byte, error) {
	if c < countFloor {
		return []byte(`"<5"`), nil
	}
	return strconv.AppendInt(nil, int64(c), 10), nil
}

// Encode is a bundle as it is printed, hashed and written (PF-SH2): indented,
// with a final line feed, and "<5" as written. encoding/json escapes < for
// HTML unless told not to, and a printed \u003c5 is not what the schema
// says the user will see.
func Encode(o Outcomes) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(o); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Outcomes is the outcomes bundle, an allowlist document: its strings are the
// schema, pattern ids from the closed set, "<5" and the install digest's hex,
// and every other value is a count or a time.
type Outcomes struct {
	Schema        string         `json:"schema"`
	InstallDigest string         `json:"install_digest"`
	Period        Period         `json:"period"`
	Counts        Counts         `json:"counts"`
	ByPattern     []PatternCount `json:"by_pattern"`
}

// Period spans the counted lines' at_unix_ms, first to last; both are 0 when
// no line counted.
type Period struct {
	FromUnixMS int64 `json:"from_unix_ms"`
	ToUnixMS   int64 `json:"to_unix_ms"`
}

// Counts are the bundle's totals. Fixed, Ignored and Overridden are the user's
// labels: 0 in schema 1.
type Counts struct {
	Fired      Count `json:"fired"`
	Fixed      Count `json:"fixed"`
	Ignored    Count `json:"ignored"`
	Overridden Count `json:"overridden"`
}

// PatternCount is one pattern's row: its fired lines, and of its labelled
// lines how many the user labelled true (0 in schema 1).
type PatternCount struct {
	Pattern   pattern.ID `json:"pattern"`
	Fired     Count      `json:"fired"`
	Precision Precision  `json:"precision"`
}

// Precision is a pattern's labelled lines and the true ones among them.
type Precision struct {
	Labelled Count `json:"labelled"`
	True     Count `json:"true"`
}

// lineFields is all a bundle reads of a steer line. Nothing else is decoded,
// so nothing else can reach a bundle.
type lineFields struct {
	Pattern  pattern.ID      `json:"pattern"`
	Delivery *steer.Delivery `json:"delivery"`
	AtUnixMS int64           `json:"at_unix_ms"`
}

// BuildOutcomes counts every run's steer log into an outcomes bundle. A line
// is fired when its delivery is delivered or flag_only and its pattern is one
// of the closed set; a holdout line never reached the agent and counts
// nowhere, nor does a suppressed, shadow or undecided one. A line ReadSteer
// could not read, a torn tail's included, is left out and not reported: the
// bundle counts what the log holds whole. by_pattern lists the patterns with a
// fired line, in pattern.All's order.
//
// One writer appends steer lines (route.go), one line per minted id, and an
// undecided line carries a null delivery, so no decision is counted twice.
func BuildOutcomes(st *store.Store) (Outcomes, error) {
	out := Outcomes{Schema: SchemaOutcomes, InstallDigest: st.InstallDigest(), ByPattern: []PatternCount{}}
	runs, err := st.Runs()
	if err != nil {
		return Outcomes{}, err
	}
	fired := map[pattern.ID]Count{}
	for _, run := range runs {
		lines, _, err := st.ReadSteer(run)
		if err != nil {
			return Outcomes{}, err
		}
		for _, raw := range lines {
			var l lineFields
			if json.Unmarshal(raw, &l) != nil || !isFired(l) {
				continue
			}
			fired[l.Pattern]++
			out.Counts.Fired++
			if out.Counts.Fired == 1 || l.AtUnixMS < out.Period.FromUnixMS {
				out.Period.FromUnixMS = l.AtUnixMS
			}
			out.Period.ToUnixMS = max(out.Period.ToUnixMS, l.AtUnixMS)
		}
	}
	for _, id := range pattern.All() {
		if fired[id] > 0 {
			out.ByPattern = append(out.ByPattern, PatternCount{Pattern: id, Fired: fired[id]})
		}
	}
	return out, nil
}

// isFired reports whether a line reached the agent, as delivered or flag_only,
// under a pattern of the closed set.
func isFired(l lineFields) bool {
	if l.Delivery == nil || !pattern.Valid(l.Pattern) {
		return false
	}
	return *l.Delivery == steer.DeliveryDelivered || *l.Delivery == steer.DeliveryFlagOnly
}
