package spec

import "strconv"

// Reason is why a spec was refused: a code from a closed list, never text
// taken from the file, so an error can be shown to a user or logged without
// carrying what an untrusted repository wrote (archive 20.2.8, Trust).
type Reason string

// The closed list of reasons (archive 20.2.8, Schema). A spec that fails
// validation applies nothing, and the first of these it trips, in schema walk
// order, is the one reported.
const (
	ReasonTooLarge       Reason = "file_too_large"            // over MaxBytes
	ReasonNotUTF8        Reason = "invalid_utf8"              // a byte sequence that is not UTF-8
	ReasonSurrogate      Reason = "unpaired_surrogate"        // a \uD800-\uDFFF escape without its pair
	ReasonSyntax         Reason = "json_syntax"               // not one JSON value
	ReasonTooDeep        Reason = "nesting_too_deep"          // over MaxDepth
	ReasonDuplicateKey   Reason = "duplicate_key"             // one key twice in an object
	ReasonNotInteger     Reason = "number_not_integer"        // a number with a fraction or exponent
	ReasonIntegerRange   Reason = "integer_out_of_range"      // beyond +-(2^53-1)
	ReasonType           Reason = "wrong_type"                // a value of another JSON type
	ReasonRequired       Reason = "required_missing"          // a required key is absent
	ReasonVersion        Reason = "unsupported_version"       // rashomon_spec is not 1
	ReasonUnknownKey     Reason = "unknown_key"               // a key the schema does not list
	ReasonTooMany        Reason = "too_many"                  // a count over its cap
	ReasonEmpty          Reason = "empty"                     // an empty string or list where one is required
	ReasonTooLong        Reason = "too_long"                  // text over its rune cap
	ReasonControl        Reason = "control_character"         // a control character in a string
	ReasonEnum           Reason = "not_in_closed_list"        // a value outside its closed list
	ReasonID             Reason = "id_charset"                // an id outside the id pattern
	ReasonDuplicateID    Reason = "duplicate_id"              // an id, or a gate's supplies kind, twice in one list
	ReasonPatternSyntax  Reason = "pattern_syntax"            // not RE2
	ReasonPatternLong    Reason = "pattern_too_long"          // over MaxPatternBytes
	ReasonRepeatTooLarge Reason = "repeat_count_too_large"    // a counted repeat over MaxRepeat
	ReasonNestedRepeat   Reason = "nested_counted_repeat"     // a counted repeat inside another
	ReasonPatternComplex Reason = "pattern_too_complex"       // over MaxPatternInstructions
	ReasonClassNotAuthn  Reason = "class_not_authentication"  // an identity fact's class is not an authentication class
	ReasonNotIdentifier  Reason = "value_not_identifier"      // a fact value that has a space
	ReasonSpecComplex    Reason = "spec_patterns_too_complex" // over MaxSpecInstructions in all
	ReasonHoldoutRate    Reason = "holdout_rate_out_of_range" // a holdout rate_bp outside 500 to 2000
	ReasonHoldoutWindow  Reason = "holdout_window_invalid"    // started without window_days or the converse, a window outside 1 to 365 days, or a started that is not a YYYY-MM-DD date
)

// Error is the one error Parse returns (archive 20.2.8, Schema): the first
// error, with its RFC 6901 JSON pointer. Invariant: every field is built from
// a Reason, a cap and the pointer, and a pointer segment that is not in
// [A-Za-z0-9._-] or is over 48 bytes is written "?", so no byte of a hostile
// key reaches a flag and the pointer's length is bounded by MaxDepth.
type Error struct {
	// Pointer locates the first error; "" is the whole file.
	Pointer string
	// Reason is the closed reason.
	Reason Reason
	// Limit is the cap that was exceeded, 0 when the reason names none.
	Limit int
}

// Error renders "spec: <reason> at <pointer> (limit N)".
func (e *Error) Error() string {
	s := "spec: " + string(e.Reason)
	if e.Pointer != "" {
		s += " at " + e.Pointer
	}
	if e.Limit > 0 {
		s += " (limit " + strconv.Itoa(e.Limit) + ")"
	}
	return s
}
