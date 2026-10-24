package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// Reason is why a spec was refused: a closed code, never text taken from the
// file. A caller counts refusals by Reason and shows the user the Reason, the
// Pointer and the Limit, so one flag per digest can name the first error
// (20.2.8).
type Reason string

// The closed set of reasons, in the order Parse can raise them.
const (
	ReasonTooLarge        Reason = "file_too_large"
	ReasonNotUTF8         Reason = "not_utf8"
	ReasonSyntax          Reason = "json_syntax"
	ReasonTooDeep         Reason = "nesting_too_deep"
	ReasonTooManyValues   Reason = "too_many_values"
	ReasonDuplicateKey    Reason = "duplicate_key"
	ReasonBadEscape       Reason = "unpaired_surrogate"
	ReasonNotInteger      Reason = "number_not_integer"
	ReasonIntegerRange    Reason = "integer_out_of_range"
	ReasonType            Reason = "wrong_type"
	ReasonRequired        Reason = "required_missing"
	ReasonVersion         Reason = "unsupported_version"
	ReasonUnknownKey      Reason = "unknown_key"
	ReasonFieldForKind    Reason = "field_not_for_kind"
	ReasonOneOfRequired   Reason = "one_of_required"
	ReasonExclusive       Reason = "fields_exclusive"
	ReasonEmpty           Reason = "empty"
	ReasonTooMany         Reason = "too_many"
	ReasonTooLong         Reason = "too_long"
	ReasonEnum            Reason = "not_in_closed_list"
	ReasonID              Reason = "id_charset"
	ReasonDuplicateID     Reason = "duplicate_id"
	ReasonChars           Reason = "disallowed_characters"
	ReasonShape           Reason = "wrong_shape"
	ReasonPatternSyntax   Reason = "pattern_syntax"
	ReasonRepeatTooLarge  Reason = "repeat_count_too_large"
	ReasonNestedRepeat    Reason = "nested_counted_repeat"
	ReasonPatternTooLong  Reason = "pattern_too_long"
	ReasonPatternComplex  Reason = "pattern_too_complex"
	ReasonSpecComplex     Reason = "spec_patterns_too_complex"
	ReasonAuthzClass      Reason = "authorization_class"
	ReasonClassNotAllowed Reason = "class_not_authentication"
	ReasonMechanism       Reason = "mechanism_mismatch"
)

// Error is the one error Parse returns. Invariant: every field is built from
// the closed Reason set, the caller's path, a bounded JSON pointer and a
// cap, never from the file's text, so the error can be shown or logged
// without carrying what an untrusted repository wrote (OWASP input
// handling).
type Error struct {
	// Path is the file the caller named.
	Path string
	// Pointer is the RFC 6901 pointer of the first error, "" for the whole
	// file. A key segment outside [A-Za-z0-9._-], or over 48 bytes, shows as
	// "?" so a hostile key never reaches a flag.
	Pointer string
	// Reason is the closed reason.
	Reason Reason
	// Limit is the cap that was exceeded, 0 when the reason names none.
	Limit int
	// Digest is the digest to key the one flag per digest by: DigestPrefix
	// and the hex SHA-256 of the file's JCS form when the file is JSON that
	// passes the lexical checks (Canonical true), else of its raw bytes. It
	// is never empty.
	Digest string
	// Canonical says which of the two Digest is.
	Canonical bool
}

// Error renders "spec <path>: <reason> at <pointer> (limit N)".
func (e *Error) Error() string {
	s := "spec " + e.Path + ": " + string(e.Reason)
	if e.Pointer != "" {
		s += " at " + e.Pointer
	}
	if e.Limit > 0 {
		s += " (limit " + strconv.Itoa(e.Limit) + ")"
	}
	return s
}

// rawDigest is DigestPrefix and the SHA-256 of b, the digest of a file that
// could not be canonicalised.
func rawDigest(b []byte) string {
	sum := sha256.Sum256(b)
	return DigestPrefix + hex.EncodeToString(sum[:])
}
