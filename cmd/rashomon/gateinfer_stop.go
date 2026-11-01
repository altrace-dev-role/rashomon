package main

import (
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/gateinfer"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// inferredGate is the Stop's inferred test gate (card GATE-INFER1, W3'):
// the command gateinfer reads from the project at root while sp declares
// no test gate at any trust, as the brief's tests line reads it (a spec
// wins); Learned when idx holds a test family, which drops the line's label
// and never ends the gate, since a learned family that did not run is never
// a row (gates.go). nil for none, or with no root.
func inferredGate(root string, sp *spec.Compiled, idx *verify.ProjectIndex) *verify.InferredGate {
	if root == "" || sp != nil && slices.ContainsFunc(sp.Gates, func(d spec.Declared) bool { return d.Gate.Kind == "test" }) {
		return nil
	}
	in, ok := gateinfer.Infer(root)
	if !ok {
		return nil
	}
	g := &verify.InferredGate{Argv: in.Argv, Dir: in.Dir}
	if idx != nil {
		g.Learned = slices.ContainsFunc(idx.Entries, func(e verify.IndexEntry) bool { return e.Kind == "test" })
	}
	return g
}

// replayInferred is inferredGate for a replayed Stop (laststop.go). With
// required, the replayed checkpoint's gates, it is built only where that
// Stop judged the inferred gate, from the project's files as they are now;
// with none, there being no checkpoint, by the Stop's rule.
func replayInferred(root string, sp *spec.Compiled, idx *verify.ProjectIndex, required map[string]bool) *verify.InferredGate {
	if _, ok := required[verify.InferredGateID]; required != nil && !ok {
		return nil
	}
	return inferredGate(root, sp, idx)
}
