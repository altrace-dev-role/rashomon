package main

import (
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// inferredGate is the Stop's inferred test gate (card GATE-INFER1); not yet
// built.
func inferredGate(root string, sp *spec.Compiled, idx *verify.ProjectIndex) *verify.InferredGate {
	return nil
}

// replayInferred is the replay's inferred test gate (card GATE-INFER1); not
// yet built.
func replayInferred(root string, sp *spec.Compiled, idx *verify.ProjectIndex, required map[string]bool) *verify.InferredGate {
	return nil
}
