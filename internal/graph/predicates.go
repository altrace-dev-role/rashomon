package graph

import "slices"

// The process and lifecycle predicates (AD15, Part B4 and B6), fixed with
// the core set so the producers of M3 and M6 and the knowledge lifecycle of
// M5 write against one vocabulary: a route handler publishes to and a
// client subscribes to a contract:event; a cron or job registration
// schedules a unit; a migration file migrates a contract:table; same_as
// links a renamed symbol across commits (fingerprint, INFERRED);
// supersedes and contradicts join knowledge rows; advances joins a session
// goal to an objective; step joins a flow to the contracts and units it
// passes through, with order and direction as attrs.
const (
	Publishes   Pred = "publishes"
	Subscribes  Pred = "subscribes"
	Schedules   Pred = "schedules"
	Migrates    Pred = "migrates"
	SameAs      Pred = "same_as"
	Supersedes  Pred = "supersedes"
	Contradicts Pred = "contradicts"
	Advances    Pred = "advances"
	Step        Pred = "step"
)

// The session predicates (ruling 15, W-M2.11): the edges a session's own
// record writes into the install store's graph file. ReadsFile is named
// apart from Reads on purpose: the engine's walks read Reads, and a shared
// word would hand every session call node to them.
const (
	Asks        Pred = "asks"
	Acts        Pred = "acts"
	ReadsFile   Pred = "reads_file"
	Edits       Pred = "edits"
	Runs        Pred = "runs"
	Claims      Pred = "claims"
	EvidencedBy Pred = "evidenced_by"
	Delivered   Pred = "delivered"
	FollowedBy  Pred = "followed_by"
)

// Predicates is the closed vocabulary (AD15): every edge in every store
// carries one of these, and a producer that needs another changes this
// list and the test that pins it (TestPredicates_Closed). A fresh slice
// each call.
func Predicates() []Pred {
	return []Pred{
		Contains, Declares, Imports, Calls, MayCall, References, Reads, Writes, Exposes, Consumes,
		ResolvesTo, MayResolveTo, CoChangedWith, DuplicatesValue, Crosses, AppliesTo,
		Publishes, Subscribes, Schedules, Migrates, SameAs, Supersedes, Contradicts, Advances, Step,
		Asks, Acts, ReadsFile, Edits, Runs, Claims, EvidencedBy, Delivered, FollowedBy,
	}
}

// IsPredicate reports whether p is in the vocabulary.
func IsPredicate(p Pred) bool { return slices.Contains(Predicates(), p) }
