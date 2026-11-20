package lang

// Reason is why a configuration step refused the file it reads, from a
// closed set: the class of the step's error, never its path or text. The
// zero value is ReadFailed, the reason for any refusal no other one names.
type Reason uint8

const (
	// ReadFailed is a refusal no other reason names: the open or the read
	// failed.
	ReadFailed Reason = iota
	// NotRegular is a file that is not a regular one (regfile.ErrNotRegular):
	// a named pipe, a directory, a device.
	NotRegular
	// OverCap is a file over the most bytes its step reads.
	OverCap
	// NotJSON is a file that does not parse as JSON.
	NotJSON
	// WrongShape is a file that parses as JSON whose values are not of the
	// kinds the step reads.
	WrongShape
)

// Refusal is a configuration step's error where it refused the file it
// reads under a root: File, that file's slash path relative to the root,
// and Reason, why. Its text and what it wraps are Err's, the step's own
// error, so a caller that matches the cause (errors.Is) or reads the text
// sees what the step returned. A registry's caller says which file was not
// read and why without naming the language whose step read it.
type Refusal struct {
	File   string
	Reason Reason
	Err    error
}

// Error is Err's text.
func (r *Refusal) Error() string { return r.Err.Error() }

// Unwrap is Err.
func (r *Refusal) Unwrap() error { return r.Err }
