package report

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/wire"
)

// Redaction exists so a report can be SHARED.
//
// A hostname is the most sensitive field this product holds: it can carry a
// tenant, a bucket, a customer name or an internal topology. That is exactly
// the report someone wants to paste into an issue or hand to a colleague, and
// without redaction the only safe answer is "do not share it", which makes the
// tool useless for the conversation it is most valuable in.
//
// What is kept is the SHAPE of the finding. A redacted report still says that
// three destinations were reached and never named, that two of them were the
// same host, and which public suffix each sat under -- enough to discuss, not
// enough to identify. The digest is stable within a report, so a reader can
// follow one host across sections.

// redactedHostLen is how much of the digest is kept.
//
// Eight hex characters is 32 bits, and the digest is now an HMAC under the
// per-install key rather than a bare hash. That is the difference between a
// recipient recovering every hostname and a recipient recovering none: the
// space of hostnames is tiny and guessable, so an UNKEYED digest was a
// dictionary lookup -- sha256("pypi.org")[:8] is a4aa2ac2, which is what this
// function used to print. Without the key, a candidate cannot be tested at all.
//
// What 32 bits still costs: two hosts in one report can collide (birthday, so
// it becomes likely in the tens of thousands), and the last label is kept in
// clear on purpose. Neither is secrecy against someone holding the key --
// anyone who can read the store can compute these. The legend in the redacted
// render says so, because the render is the artifact that gets shared and the
// README is not.
const redactedHostLen = 8

// redactDomain separates this digest from every other use of the install key.
// store.HostDigest uses "forget-host\x00" for gap records; reusing it here
// would make a shared report's digests testable against a store's gaps.
const redactDomain = "redact\x00"

// redactHost renders a hostname as a digest plus its public suffix.
//
// The suffix is kept because it is the part that carries meaning without
// carrying identity: ".org" and ".internal" and ".amazonaws.com" tell a reader
// something real about where traffic went, while the label in front of them is
// the part that names a customer.
//
// LIMITATION, stated rather than hidden: the "public suffix" here is the last
// label, not a Public Suffix List lookup. So a.example.co.uk renders as
// <digest>.uk rather than <digest>.co.uk, and an IP address renders as a
// digest plus its last octet. Doing it properly needs the PSL, which is a
// dependency and a data file that goes stale; the last label is honest about
// what it is and never reveals more than a true PSL lookup would.
func redactHost(h string, key []byte) string {
	if h == "" {
		return ""
	}
	// No key means no store, which means no hosts -- so this is unreachable in
	// practice. It says so rather than falling back to an unkeyed hash, because
	// a silent fall back to the exact primitive being removed is how a fix
	// becomes a regression nobody notices.
	if len(key) == 0 {
		return "[redacted: no install key available to digest with]"
	}
	mac := hmac.New(sha256.New, key)
	// Domain separator, and NOT the one store.HostDigest uses. The same key
	// serves both, so without separation a shared report would carry a token
	// that can be tested directly against a store's gap records.
	mac.Write([]byte(redactDomain))
	mac.Write([]byte(h))
	digest := hex.EncodeToString(mac.Sum(nil))[:redactedHostLen]

	// A bracketed IPv6 literal has no meaningful suffix to keep, and its last
	// hextet is not one -- it is part of the address.
	if strings.HasPrefix(h, "[") {
		return digest + ".ipv6"
	}
	if i := strings.LastIndex(h, "."); i >= 0 && i < len(h)-1 {
		return digest + h[i:]
	}
	// A single-label host (localhost, an internal short name) has no suffix.
	return digest
}

// redactList redacts a list of hostnames, preserving order.
func redactList(hosts []string, key []byte) []string {
	if len(hosts) == 0 {
		return hosts
	}
	out := make([]string, len(hosts))
	for i, h := range hosts {
		out[i] = redactHost(h, key)
	}
	return out
}

// Redact returns a copy of the report with every hostname replaced.
//
// It works on a COPY and mutates nothing, so a caller cannot accidentally
// render the redacted form first and the plain form second from the same
// value -- and so that the redaction can never be partially applied.
//
// Every field that can hold a hostname is covered here. The risk this function
// carries is not that it redacts wrongly but that a LATER field is added and
// nobody adds it here, which would leak one host into an otherwise redacted
// report. A test walks the rendered output for the plain hostnames rather than
// checking the fields this function happens to know about.
func Redact(rep *Report, key []byte) *Report {
	if rep == nil {
		return nil
	}
	out := *rep
	// The report says of itself that it is redacted, so the renderer can carry
	// the legend and a JSON consumer is not left inferring it from the shape of
	// the hostnames.
	out.Redacted = true
	out.Sessions = make([]Session, len(rep.Sessions))
	for i, sess := range rep.Sessions {
		s := sess
		d := sess.Destinations

		d.WireOnly = redactList(d.WireOnly, key)
		d.ClientPlane = redactList(d.ClientPlane, key)
		d.DeclaredNotObserved = redactList(d.DeclaredNotObserved, key)
		d.NotObservable = redactList(d.NotObservable, key)

		hosts := make([]wire.Destination, 0, len(d.Hosts))
		for _, h := range d.Hosts {
			h.Host = redactHost(h.Host, key)
			hosts = append(hosts, h)
		}
		d.Hosts = hosts

		n := d.Novelty
		n.Hosts = redactList(n.Hosts, key)
		d.Novelty = n

		s.Destinations = d

		// The agent's account is CONTENT and cannot be redacted host by host:
		// it is prose that may name anything at all. It is dropped entirely
		// rather than partially cleaned, because a summary with some names
		// removed reads as safe while being exactly as revealing as before.
		if s.Account.Available {
			s.Account = Account{Available: true, Text: accountRedacted, Truncated: false}
		}

		// Transcript paths carry the project directory, which is often the
		// customer's name.
		ts := make([]Transcript, len(sess.Transcripts))
		for j, t := range sess.Transcripts {
			t.Path = redactHost(t.Path, key)
			ts[j] = t
		}
		s.Transcripts = ts
		s.Chains = redactChains(sess.Chains, key)

		out.Sessions[i] = s
	}
	return &out
}

// redactChains digests the names in the causal view.
//
// EVERY LEVEL IS REBUILT, and that is the whole substance of this function.
// Redact returns a new report so the caller can still render the original --
// `rashomon report --redact` and a plain `report` are the same code path with a
// different flag -- but Go's value copy of a struct shares its slices. So
// assigning into `chain.Links[j].Hosts[k].Host` through a shallow copy writes
// the digest into the ORIGINAL report's backing array: the unredacted render
// would then print digests, and, far worse for a function whose users are
// deciding what to send someone, a second render of the same in-memory report
// would look correctly redacted while sharing state with an object the caller
// believes is untouched.
//
// Three levels of slice, three allocations. The transcript path is digested
// with the same keyed helper the Transcript section uses -- one path, not a
// second one that could drift from it.
func redactChains(c Chains, key []byte) Chains {
	out := Chains{Prompts: make([]Chain, 0, len(c.Prompts)), Unchained: c.Unchained}
	for _, chain := range c.Prompts {
		ch := chain
		ch.TranscriptPath = redactHost(chain.TranscriptPath, key)
		ch.Links = make([]Link, 0, len(chain.Links))
		for _, link := range chain.Links {
			l := link
			l.Hosts = make([]LinkHost, 0, len(link.Hosts))
			for _, h := range link.Hosts {
				// The state is carried through untouched: it is a verdict, not
				// a name, and it is the only thing left worth reading.
				l.Hosts = append(l.Hosts, LinkHost{Host: redactHost(h.Host, key), State: h.State})
			}
			ch.Links = append(ch.Links, l)
		}
		out.Prompts = append(out.Prompts, ch)
	}
	return out
}

// accountRedacted is what stands in for the agent's summary in a shared
// report. It says that a summary existed, because "the agent said nothing" and
// "we are not showing you what it said" are different facts.
const accountRedacted = "[redacted: the agent's summary is prose and may name anything]"
