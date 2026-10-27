package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FileRecordScope is the marker under the store root that names the record
// scope the consent on file agreed to: "v4" or "v5" and a newline. It is a
// view of consent.json, which stays the record of truth: every consent write
// removes it, appends its line, and writes it again from consent.json's own
// answer (consent.RecordScope), so it can lag the log and never lead it.
//
// The hook reads this file, never consent.json: one small read per hook
// process, the same shape as the pause check, where consent.json is up to
// 64 KiB of user-editable NDJSON that would be parsed on every tool call.
//
// Exported so the disclosure inventory (internal/disclose) names the same
// constant rather than a second copy of the literal.
const FileRecordScope = "record_scope"

// The record scopes (Technical spec 1.2; 7.2.3). v4 is the shape every
// install records; v5 adds the content fields GateDeclaration and
// GateExecution withhold below it. internal/consent names the same two
// values for the records it writes.
const (
	RecordScopeV4 = "v4"
	RecordScopeV5 = "v5"
)

// withheldNoConsent is the reason a *_withheld key carries for a content
// field the record scope on file does not cover (record_v5.go's
// command_withheld list).
const withheldNoConsent = "no_consent"

// ErrRecordScopeWord is RecordScopeMarker's error for a marker whose content
// is neither scope word.
var ErrRecordScopeWord = errors.New("store: the record_scope marker holds neither v4 nor v5")

// RecordScopeMarker is the record scope root's marker names: v5 only when the
// marker holds exactly "v5" (surrounding white space aside), v4 for "v4" and
// v4 for everything else. A non-nil error says why the scope is the v4
// default: errors.Is(err, fs.ErrNotExist) for no marker, ErrRecordScopeWord
// for content that is neither word, the read's own error otherwise. A caller
// that only applies the scope may drop the error; the scope it comes with is
// already the one that withholds.
//
// It reads one small file directly, never through Open, for Paused's reason:
// Open mints an install identity and a key on first use, and a root must not
// acquire one by being asked what it may record.
func RecordScopeMarker(root string) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, FileRecordScope))
	if err != nil {
		return RecordScopeV4, err
	}
	switch strings.TrimSpace(string(b)) {
	case RecordScopeV5:
		return RecordScopeV5, nil
	case RecordScopeV4:
		return RecordScopeV4, nil
	}
	return RecordScopeV4, ErrRecordScopeWord
}

// WriteRecordScopeMarker sets root's marker to scope, which must be v4 or v5:
// written under a temporary name beside it, synced and renamed over it, so a
// reader finds the old marker or the new one and never a part of either.
// Like the reader, it never goes through Open.
func WriteRecordScopeMarker(root, scope string) error {
	if scope != RecordScopeV4 && scope != RecordScopeV5 {
		return fmt.Errorf("store: refusing to write record scope %q, which is neither v4 nor v5", scope)
	}
	return ReplaceFile(filepath.Join(root, FileRecordScope), []byte(scope+"\n"))
}

// RemoveRecordScopeMarker removes root's marker, so the hook reads v4 until
// it is written again. No marker is not an error.
func RemoveRecordScopeMarker(root string) error {
	if err := os.Remove(filepath.Join(root, FileRecordScope)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// GateDeclaration is the record-scope gate for a declaration. At any scope
// but v5 it nulls each v5 content field that holds a value -- the command
// with its cut and marker spans, cwd, project, file_path, written_paths and
// target -- and sets the field's *_withheld key to no_consent; at v5 it
// changes nothing. A field already null keeps its key as it was, so
// no_consent never stands for a value that was not there.
//
// The hook runs it before CapDeclaration: a value the user did not agree to
// record reads no_consent, not oversize, and the caps, finding it gone, do
// not report it on the call's coverage record. Which fields are content is
// TestRS7's table, and a *_withheld key added to the record without an entry
// there fails it.
func GateDeclaration(d *Declaration, scope string) {
	if scope == RecordScopeV5 {
		return
	}
	if d.Command != nil || d.CommandCut != nil || d.Masked != nil {
		d.Command, d.CommandCut, d.Masked, d.CommandWithheld = nil, nil, nil, noConsent()
	}
	d.CWD = unconsented(d.CWD, &d.CWDWithheld)
	d.Project = unconsented(d.Project, &d.ProjectWithheld)
	d.FilePath = unconsented(d.FilePath, &d.FilePathWithheld)
	if d.WrittenPaths != nil {
		incomplete := false
		d.WrittenPaths, d.WrittenPathsComplete, d.WrittenPathsWithheld = nil, &incomplete, noConsent()
	}
	if d.Target != nil {
		d.Target, d.TargetWithheld = nil, noConsent()
	}
}

// GateExecution is GateDeclaration for an execution: the executed command
// with its cut and marker spans, and the git state after the call, withheld
// whole since its root is a path and its branch a name.
func GateExecution(x *Execution, scope string) {
	if scope == RecordScopeV5 {
		return
	}
	if x.ExecutedCommand != nil || x.ExecutedCommandCut != nil || x.ExecutedMasked != nil {
		x.ExecutedCommand, x.ExecutedCommandCut, x.ExecutedMasked, x.ExecutedCommandWithheld = nil, nil, nil, noConsent()
	}
	if x.HeadAfter != nil {
		x.HeadAfter, x.HeadAfterWithheld = nil, noConsent()
	}
}

// unconsented is the gate for one nullable string: null, with key set to
// no_consent, when it holds a value.
func unconsented(s *string, key **string) *string {
	if s != nil {
		*key = noConsent()
	}
	return nil
}

func noConsent() *string {
	reason := withheldNoConsent
	return &reason
}
