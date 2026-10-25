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
// This first slice carries ingest, PROVEN facts, trust tiers, the log and the
// offline loader. Class hierarchy analysis, per-package invalidation, blast
// radius, the Impact and Obligations interface and cross-commit identity are
// later slices.
package pgraph
