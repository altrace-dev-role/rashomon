package store

// MaxLine is the longest store line this package reads, its newline
// included (maxLine). It is exported so a reader of a store outside this
// package holds the store's own bound, not a copy of its value.
const MaxLine = maxLine
