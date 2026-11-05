package graph

import "strings"

// A repository's rows are keyed two ways (PLAN B2, AD14). Inside its own
// store an id is local (unit:, file:, sym:, ...) and the store's header
// names the repository. A row that outlives a session or leaves the
// repository (a knowledge applies_to, a label, a frontier's touched set, a
// workspace edge, a report) carries the global form, g:<rid>#<local>, where
// rid is baseline.RepoID's form: the normalized origin url, else an
// ephemeral path: form. Two repositories with identical paths and symbol
// names therefore never share an id, and nothing joins across repositories
// by name.

// GlobalID is local under the repository rid. A rid never holds a hash
// sign (the origin form drops a fragment; the ephemeral form is hex), so the
// first one after the prefix is the separator whatever the local id holds.
func GlobalID(rid string, local ID) ID { return ID("g:" + rid + "#" + string(local)) }

// SplitGlobal is a global id's repository and local id; false for any other
// id, an empty repository or an empty local id.
func SplitGlobal(id ID) (rid string, local ID, ok bool) {
	rest, found := strings.CutPrefix(string(id), "g:")
	if !found {
		return "", "", false
	}
	r, l, found := strings.Cut(rest, "#")
	if !found || r == "" || l == "" {
		return "", "", false
	}
	return r, ID(l), true
}

// RepoNodeID is repo:<rid>, the node that stands for the repository in its
// own store; the index asserts it with the header's id.
func RepoNodeID(rid string) ID { return ID("repo:" + rid) }
