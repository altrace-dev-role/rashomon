package render

// The wrapped forms of regression-location and stale-evidence: the gate's
// runner ran inside another command, a script or make (WRAPPER-RUN1), and the
// red or green was read from that command's output (WRAPPER-RED-STEP1). Their
// bindings and rows live here so render.go and templates stay inside Law 10;
// templates copies this table in beside templatesLater and templatesRide.

import (
	"reflect"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// RegressionLocationWrappedBinding is regression-location's slots for a red
// read from the output of a command that ran the gate's runner inside it, a
// script or make (WRAPPER-RUN1): the output form's slots, and a row that
// says where the run was read, never a pipe or the wrapper's exit
// (WRAPPER-RED-STEP1).
type RegressionLocationWrappedBinding struct {
	Gate    slot.Gate     `slot:"gate"`
	Minutes slot.Minutes  `slot:"minutes"`
	Paths   slot.PathList `slot:"paths"`
}

// RegressionLocationWrappedDegradedBinding is the wrapped form's no-paths row.
type RegressionLocationWrappedDegradedBinding struct {
	Gate    slot.Gate    `slot:"gate"`
	Minutes slot.Minutes `slot:"minutes"`
	Count   slot.Count   `slot:"count"`
}

// RegressionLocationWrappedGreenBinding is regression-location's slots for a
// red by the gate's own exit after a green read in a wrapper: the exit form's
// slots, in a row that does not say the green exited 0 (WRAPPER-RED-STEP1).
type RegressionLocationWrappedGreenBinding struct {
	Gate    slot.Gate     `slot:"gate"`
	Minutes slot.Minutes  `slot:"minutes"`
	Exit    slot.Exit     `slot:"exit"`
	Paths   slot.PathList `slot:"paths"`
}

// RegressionLocationWrappedGreenDegradedBinding is that form's no-paths row.
type RegressionLocationWrappedGreenDegradedBinding struct {
	Gate    slot.Gate    `slot:"gate"`
	Minutes slot.Minutes `slot:"minutes"`
	Exit    slot.Exit    `slot:"exit"`
	Count   slot.Count   `slot:"count"`
}

// StaleEvidenceWrappedBinding is stale-evidence's slots where the green was
// read from the output of a command that ran the gate inside it
// (WRAPPER-RED-STEP1).
type StaleEvidenceWrappedBinding struct {
	Gate    slot.Gate     `slot:"gate"`
	Minutes slot.Minutes  `slot:"minutes"`
	Paths   slot.PathList `slot:"paths"`
}

// StaleEvidenceWrappedDegradedBinding is the wrapped form's no-paths row.
type StaleEvidenceWrappedDegradedBinding struct {
	Gate    slot.Gate    `slot:"gate"`
	Minutes slot.Minutes `slot:"minutes"`
	Count   slot.Count   `slot:"count"`
}

// templatesWrapped is the wrapped rows of the table templates returns.
func templatesWrapped() map[templateKey]template {
	return map[templateKey]template{
		{pattern.RegressionLocation, reflect.TypeOf(RegressionLocationWrappedBinding{})}: {
			id: "regression-location-wrapped/v1",
			format: "rashomon record: gate {gate} passed earlier in this session ({minutes} min ago) and its output reports a failure on this run " +
				"(read from its output; it ran inside another command). Files written between the two runs: {paths}.",
		},
		{pattern.RegressionLocation, reflect.TypeOf(RegressionLocationWrappedDegradedBinding{})}: {
			id: "regression-location-wrapped-degraded/v1",
			format: "rashomon record: gate {gate} passed earlier in this session ({minutes} min ago) and its output reports a failure on this run " +
				"(read from its output; it ran inside another command). {count} file writes were recorded between the two runs.",
		},
		{pattern.RegressionLocation, reflect.TypeOf(RegressionLocationWrappedGreenBinding{})}: {
			id: "regression-location-wrapped-green/v1",
			format: "rashomon record: gate {gate} passed earlier in this session ({minutes} min ago; read from its output, it ran inside another command) " +
				"and exited {exit} on this run. Files written between the two runs: {paths}.",
		},
		{pattern.RegressionLocation, reflect.TypeOf(RegressionLocationWrappedGreenDegradedBinding{})}: {
			id: "regression-location-wrapped-green-degraded/v1",
			format: "rashomon record: gate {gate} passed earlier in this session ({minutes} min ago; read from its output, it ran inside another command) " +
				"and exited {exit} on this run. {count} file writes were recorded between the two runs.",
		},
		{pattern.StaleEvidence, reflect.TypeOf(StaleEvidenceWrappedBinding{})}: {
			id: "stale-evidence-wrapped/v1",
			format: "rashomon record: gate {gate} passed earlier in this session ({minutes} min ago), read from its output as it ran inside another command; " +
				"this call then wrote {paths}. No later run of the gate is recorded.",
		},
		{pattern.StaleEvidence, reflect.TypeOf(StaleEvidenceWrappedDegradedBinding{})}: {
			id: "stale-evidence-wrapped-degraded/v1",
			format: "rashomon record: gate {gate} passed earlier in this session ({minutes} min ago), read from its output as it ran inside another command; " +
				"this call then wrote {count} files. No later run of the gate is recorded.",
		},
	}
}
