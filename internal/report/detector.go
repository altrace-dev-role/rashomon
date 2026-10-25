package report

// DetectorID names one detector: a check that puts a judgement in front of a
// user, either as a sentence of the end-of-turn line or as a finding in a
// report section. It is the one vocabulary the detectors are told apart by.
//
// An id is not a printed word. Wording is presentation and is rewritten when
// the copy is (the end-of-turn line's sentences, the report's headings), so
// nothing may decide which detector fired by reading a sentence; it asks the
// structured fields the sentence is itself rendered from, through
// digest.Digest.Fired and Destinations.Fired. The ids are also what the
// precision corpus (test/corpus/detectors) names its fixtures, label rules
// and labelling sheet by.
//
// Ids are never written to a record, a digest or a report: they are in code
// only, so adding the vocabulary changed no output. A detector may also have a
// steer pattern id (internal/steer/pattern), and most do not yet; that is a
// separate, later mapping and nothing here depends on it.
type DetectorID string

// The ten detectors, in the order the end-of-turn line states the first eight,
// then the two that exist only in `rashomon report`. A new detector adds a
// constant here, an entry in DetectorIDs, and a row to the corpus's label
// rules; the strings are stable once shipped, as they are fixture directory
// names.
const (
	DetectorDigestUnknown                   DetectorID = "digest_unknown"
	DetectorCoverageUnverified              DetectorID = "coverage_unverified"
	DetectorDigestTruncated                 DetectorID = "digest_truncated"
	DetectorSilentFailure                   DetectorID = "silent_failure"
	DetectorMaskedRun                       DetectorID = "masked_run"
	DetectorTestsOnlyThenGreen              DetectorID = "tests_only_then_green"
	DetectorFlakyPair                       DetectorID = "flaky_pair"
	DetectorDeclarationWithoutExecution     DetectorID = "declaration_without_execution"
	DetectorNovelDestination                DetectorID = "novel_destination"
	DetectorExecutedDifferentlyFromDeclared DetectorID = "executed_differently_from_declared"
)

// DetectorIDs returns the ten ids in their stated order.
func DetectorIDs() []DetectorID {
	return []DetectorID{
		DetectorDigestUnknown, DetectorCoverageUnverified, DetectorDigestTruncated,
		DetectorSilentFailure, DetectorMaskedRun, DetectorTestsOnlyThenGreen,
		DetectorFlakyPair, DetectorDeclarationWithoutExecution,
		DetectorNovelDestination, DetectorExecutedDifferentlyFromDeclared,
	}
}

// Fired reports whether one of the two report-only detectors fires on these
// destinations: DetectorNovelDestination when the project baseline was readable, was
// not just established by this render, and lists a host this session named
// that it did not hold; DetectorExecutedDifferentlyFromDeclared when a call's executed
// input digest differs from its declared one. Any other id is not decided
// here and reports false.
func (d Destinations) Fired(id DetectorID) bool {
	switch id {
	case DetectorNovelDestination:
		return d.Novelty.Available && !d.Novelty.Established && len(d.Novelty.Hosts) > 0
	case DetectorExecutedDifferentlyFromDeclared:
		return d.ExecutedNotAsDeclared > 0
	}
	return false
}
