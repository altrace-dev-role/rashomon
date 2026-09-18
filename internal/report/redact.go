package report

import (
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
// Eight hex characters is 32 bits. That is NOT a privacy boundary and is not
// meant to be one: anyone holding a candidate hostname can hash it and compare,
// which is unavoidable for any scheme that keeps equal hosts equal. Its job is
// to be short enough to read and long enough that two hosts in one report do
// not collide. Treating it as secrecy against a determined reader would be the
// dangerous misreading, which is why it is documented here and in the rendered
// legend.
const redactedHostLen = 8

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
func redactHost(h string) string {
	if h == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(h))
	digest := hex.EncodeToString(sum[:])[:redactedHostLen]

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
func redactList(hosts []string) []string {
	if len(hosts) == 0 {
		return hosts
	}
	out := make([]string, len(hosts))
	for i, h := range hosts {
		out[i] = redactHost(h)
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
func Redact(rep *Report) *Report {
	if rep == nil {
		return nil
	}
	out := *rep
	out.Sessions = make([]Session, len(rep.Sessions))
	for i, sess := range rep.Sessions {
		s := sess
		d := sess.Destinations

		d.WireOnly = redactList(d.WireOnly)
		d.ClientPlane = redactList(d.ClientPlane)
		d.DeclaredNotObserved = redactList(d.DeclaredNotObserved)
		d.NotObservable = redactList(d.NotObservable)

		hosts := make([]wire.Destination, 0, len(d.Hosts))
		for _, h := range d.Hosts {
			h.Host = redactHost(h.Host)
			hosts = append(hosts, h)
		}
		d.Hosts = hosts

		n := d.Novelty
		n.Hosts = redactList(n.Hosts)
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
			t.Path = redactHost(t.Path)
			ts[j] = t
		}
		s.Transcripts = ts

		out.Sessions[i] = s
	}
	return &out
}

// accountRedacted is what stands in for the agent's summary in a shared
// report. It says that a summary existed, because "the agent said nothing" and
// "we are not showing you what it said" are different facts.
const accountRedacted = "[redacted: the agent's summary is prose and may name anything]"
