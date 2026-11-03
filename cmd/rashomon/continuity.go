package main

import (
	"io"

	"github.com/altrace-dev-role/rashomon/internal/directive"
	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// readDirectives is DIRECTIVE-KEEP1's Read, a variable so a test can stand
// in for it until that row's body lands.
var readDirectives = directive.Read

// printContinuity is the continuity briefing at a compact start. Red: it
// prints nothing.
func printContinuity(args []string, payload *hookio.Payload, st *store.Store, lead string, id harness.Identity, stdout io.Writer) bool {
	return false
}
