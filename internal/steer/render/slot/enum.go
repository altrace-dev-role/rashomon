package slot

import (
	"errors"
	"slices"
)

// EnumTable names one of the per-pattern closed tables an Enum value is
// drawn from (archive 40.3.8: "a per-pattern closed table (failure class
// labels, verb labels, mechanism kinds, gate kinds)"). The tables are the
// literals in enumTables below, so a value can never bring its own table:
// a pattern's PR adds its table here, where TestSTR11c checks every value
// against the charset, the deny list and the entropy test (P07N-8).
type EnumTable uint8

const (
	// EnumGateKinds is the subtask-gate card's {gates} fallback when no
	// Gate slot is known (archive 40.4.1: "else the Enum `test, build or
	// lint`").
	EnumGateKinds EnumTable = iota + 1
	// EnumTrust is the trust word a preflight item prints beside its id:
	// never "untrusted", whose rules a note counts and does not name.
	EnumTrust
	// EnumEvidenceKinds is spec.EvidenceKinds, in its order.
	EnumEvidenceKinds
	// EnumEvidenceSource is where a preflight's expected evidence came from.
	EnumEvidenceSource
	// EnumPreflightUnknown is what a preflight could not read or count.
	EnumPreflightUnknown
)

// enumTables is every Enum table. Nothing writes to it after this
// declaration; TestSTR11c holds that by AST.
var enumTables = map[EnumTable][]string{
	EnumGateKinds:      {"test, build or lint"},
	EnumTrust:          {"trusted", "managed", "builtin"},
	EnumEvidenceKinds:  {"real_data_run", "hostile_input_run", "cross_check", "judgment_disclosed", "mechanism_stated"},
	EnumEvidenceSource: {"task file", "inferred from the request"},
	EnumPreflightUnknown: {
		"no spec found", "spec present, untrusted", "spec unreadable", "no gate history for this project",
		"symbols not matched, no platform graph log", "request over 16 KiB, the rest not read",
		"no named path exists in the project", "no go.mod, packages not counted",
		"package walk capped, count is a lower bound",
	},
}

// Enum is one value of a closed table. It renders unquoted, so only a
// value of a registered table is valid, and the zero Enum is in none.
type Enum struct {
	table EnumTable
	value string
}

// NewEnum returns value as a member of the registered table; Validate
// checks it.
func NewEnum(table EnumTable, value string) Enum {
	return Enum{table: table, value: value}
}

// enumRE keeps quotes, angle brackets and control characters out of a table
// value; the gate-kinds value the subtask-gate card names, "test, build or
// lint" (archive 40.4.1), passes.
var enumRE = &lazyRegexp{pattern: `^[A-Za-z0-9 ,./_-]{1,64}$`}

func (e Enum) Validate() error {
	if err := charset("enum value", e.value, enumRE); err != nil {
		return err
	}
	if !slices.Contains(enumTables[e.table], e.value) {
		return errors.New("slot: enum value is not in its registered table")
	}
	return nil
}

func (e Enum) Text() string         { return e.value }
func (e Enum) bind() (Value, error) { return checked(e, e.Validate()) }
