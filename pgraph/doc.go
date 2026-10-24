// Package pgraph is a platform graph for Go code: what the code is, as facts a
// reader can trust to a stated degree.
//
// Boundary. pgraph reads Go modules (scan roots) through go/packages and
// go/types and emits facts; it imports nothing from the rashomon module and the
// rashomon module imports nothing from it, so it can move to its own
// repository unchanged. Its only dependency outside the standard library is
// golang.org/x/tools, which is why it is a separate module: the hook path stays
// standard library only. The loader runs offline (see LoaderEnv).
//
// Vocabulary. A Fact is a Triple (subject, predicate, object) plus the Tier of
// the Producer that made it, the build Config it was read under, the commit,
// and the extractor. A Log is an append-only sequence of Records (assert or
// retract); its View is the sorted set of live facts, a function of that set
// alone. Sync reads a Program and reconciles the log to it.
//
// Symbols are named by identity.go's scheme: names only, never a position, with
// an ordinal for the few names Go lets repeat in a package. Link carries
// identity across a commit when a name changed. Dispatch writes class
// hierarchy analysis as ALGORITHMIC may-call facts through SyncCHA, beside the
// PROVEN facts of Sync; Score and ParseLabels measure it against a labelled
// corpus.
//
// Per-package invalidation, blast radius, the Impact and Obligations interface,
// unknown fan-in and the untracked-configuration warning are later slices.
package pgraph
