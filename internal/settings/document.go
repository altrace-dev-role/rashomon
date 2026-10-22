// Package settings reads and edits Claude Code settings files.
//
// The user's settings.json is written by Claude Code itself during normal use:
// every "always allow" the user accepts appends to permissions.allow. It is
// also where other tools install their hooks. This package therefore never
// round-trips the file through a generic JSON encoder, which would reorder
// keys, restyle whitespace and drop nothing while changing everything. It holds
// every value it does not own as the raw bytes it was read as, and re-encodes
// only the levels it edits: hooks, one event under it, and that event's array.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/fault"
)

// member is one key of a JSON object, with its value held raw.
type member struct {
	key string
	raw json.RawMessage
	// at is where raw starts in the document's source, for an error about
	// it; nowhere for a value this package wrote (set).
	at pos
}

// set replaces a member's value. The new bytes are not in the source, so the
// member no longer has a place there.
func (m *member) set(raw json.RawMessage) { m.raw, m.at = raw, 0 }

// Document is a settings file as ordered raw members.
type Document struct {
	members []member
	// path and src are what an error names: the file, and the bytes a line
	// and column are counted in.
	path string
	src  []byte
}

// Load reads a settings file. An absent file and an empty one are both an
// empty document, because a machine that has never run Claude Code has
// neither, and refusing to install on such a machine would be refusing the
// first user.
func Load(path string) (*Document, error) {
	fault.Inject(fault.PointSettingsLoad)
	// Fail is the one injection that returns instead of panicking: what it
	// exercises is the caller's retry, which a panic would never let run.
	if err := fault.Fail(fault.PointSettingsLoad); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		// A dangling symlink's open fails with the same ErrNotExist a path
		// with nothing at it gives, and the two are not the same fact: one
		// has nothing to read, the other names a target that is not there to
		// read. writeTarget already draws the line Write and ReadOriginal hold
		// to; Load asking it means a settings file that is a broken link
		// reads as unreadable, not as an absent one watch would then install
		// straight over, and status would then report as "not installed"
		// rather than the link it could not resolve. It is asked only once
		// the read has failed: a file that reads has resolved, and resolving
		// it again on every Load walked each element of the path, about 42
		// of a hook call's allocations (BenchmarkRunHook).
		if _, err := writeTarget(path); err != nil {
			return nil, err
		}
		return &Document{}, nil
	}
	if err != nil {
		return nil, err
	}
	// data is os.ReadFile's own buffer, which nothing else holds, so the
	// document takes it rather than copying it: ParseNamed's copy, on every
	// hook call's Load, was the size of the settings file again.
	return parseOwned(path, data)
}

// Parse builds a Document from bytes that have no file name. Its errors say
// "settings" where a named document's say the path.
func Parse(data []byte) (*Document, error) {
	return ParseNamed("", data)
}

// ParseNamed builds a Document from the bytes of the file at path. Every error
// the document raises, now or from a later edit, is an *Error naming path.
// The document keeps its own copy of data: every value it carries, and every
// position a later error reports, is in those bytes, so a caller that reuses
// its buffer changes neither.
func ParseNamed(path string, data []byte) (*Document, error) {
	return parseOwned(path, bytes.Clone(data))
}

// parseOwned is ParseNamed over bytes the document may keep as they are:
// nothing else holds them, or will write into them.
func parseOwned(path string, data []byte) (*Document, error) {
	d := &Document{path: path, src: data}
	if len(bytes.TrimSpace(data)) == 0 {
		return d, nil
	}
	members, err := parseObject(data, true)
	if err != nil {
		return nil, d.fail(posAt(0), err)
	}
	d.members = members
	return d, nil
}

// Get returns a copy of a top-level value's raw bytes. A copy, because the
// document renders those bytes again in Bytes, and a caller that wrote into
// them would change a file it never asked to edit (lld-8).
func (d *Document) Get(key string) (json.RawMessage, bool) {
	if m := d.find(key); m != nil {
		return bytes.Clone(m.raw), true
	}
	return nil, false
}

// Bytes renders the document. Values not edited are emitted as they were read.
func (d *Document) Bytes() []byte {
	return append(writeObject(d.members, 0), '\n')
}

const hooksKey = "hooks"

// ErrEventNotUTF8 is a hook event name that is not valid UTF-8, refused by
// SetHookEntries and RemoveHookEvent. JSON cannot carry such a key: written,
// it would read back as U+FFFD, a different event.
var ErrEventNotUTF8 = errors.New("hook event name is not valid UTF-8")

// HookEntries returns the raw matcher groups under hooks.<event>, in order.
func (d *Document) HookEntries(event string) ([]json.RawMessage, error) {
	events, err := d.hookEvents()
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		if e.key == event {
			entries, err := parseArray(event, e.raw)
			if err != nil {
				return nil, d.fail(e.at, err)
			}
			return entries, nil
		}
	}
	return nil, nil
}

// SetHookEntries replaces hooks.<event> with entries. Every other event under
// hooks, and every other top-level key, is carried over byte for byte.
//
// event is a name Claude Code defines. One that is not valid UTF-8 is refused
// with ErrEventNotUTF8 and the document left as it was. A key read from a file
// always is valid.
func (d *Document) SetHookEntries(event string, entries []json.RawMessage) error {
	if err := d.checkEvent(event); err != nil {
		return err
	}
	events, err := d.hookEvents()
	if err != nil {
		return err
	}

	arr := writeArray(entries, 2)
	replaced := false
	for i := range events {
		if events[i].key == event {
			events[i].raw = arr
			replaced = true
			break
		}
	}
	if !replaced {
		events = append(events, member{key: event, raw: arr})
	}

	obj := writeObject(events, 1)
	if h := d.find(hooksKey); h != nil {
		h.set(obj)
	} else {
		d.members = append(d.members, member{key: hooksKey, raw: obj})
	}
	return nil
}

// RemoveHookEvent deletes one event key from the hooks object, and the hooks
// object itself when that leaves it empty.
//
// Writing an empty array instead parses identically and READS differently, and
// the difference is the whole promise detach makes. watch creates the hooks
// object and one key per event it installs; a detach that only emptied them
// left a hooks object and four event keys in a file that had none before, and
// the file had to be repaired by hand. A user checks "as found" by looking at
// the file, or at git diff on a dotfiles repository.
//
// Only a key emptied by the caller is removed. An event another tool owns keeps
// its entries and its key, and an events object that still holds anything keeps
// both -- tidying those would be editing settings nobody asked us to touch.
//
// An event name that is not valid UTF-8 is refused as SetHookEntries refuses
// it: no key in the file can be that name.
func (d *Document) RemoveHookEvent(event string) error {
	if err := d.checkEvent(event); err != nil {
		return err
	}
	events, err := d.hookEvents()
	if err != nil {
		return err
	}
	kept := make([]member, 0, len(events))
	for _, m := range events {
		if m.key == event {
			continue
		}
		kept = append(kept, m)
	}
	if len(kept) == len(events) {
		// Not present. Nothing to remove, and nothing to rewrite: an
		// unconditional rewrite here would reformat a hooks object this call was
		// not asked to change.
		return nil
	}
	h := d.find(hooksKey)
	if h == nil {
		return nil
	}
	if len(kept) == 0 {
		d.removeMember(hooksKey)
		return nil
	}
	h.set(writeObject(kept, 1))
	return nil
}

// checkEvent refuses an event name JSON cannot carry (ErrEventNotUTF8).
func (d *Document) checkEvent(event string) error {
	if utf8.ValidString(event) {
		return nil
	}
	return &Error{Path: d.path, Err: fmt.Errorf("%w: %q", ErrEventNotUTF8, event)}
}

func (d *Document) removeMember(key string) {
	for i := range d.members {
		if d.members[i].key == key {
			d.members = append(d.members[:i], d.members[i+1:]...)
			return
		}
	}
}

func (d *Document) hookEvents() ([]member, error) {
	h := d.find(hooksKey)
	if h == nil || isNull(h.raw) {
		return nil, nil
	}
	events, err := parseObject(h.raw, false)
	if err != nil {
		return nil, d.fail(h.at, fmt.Errorf("hooks: %w", err))
	}
	for i := range events {
		events[i].at = events[i].at.in(h.at)
	}
	return events, nil
}

func (d *Document) find(key string) *member {
	for i := range d.members {
		if d.members[i].key == key {
			return &d.members[i]
		}
	}
	return nil
}

func isNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// parseObject splits an object into ordered members with raw values. With
// whole set, anything after the closing brace is an error. Duplicate keys are
// refused: an encoder would silently keep one of them, and editing a document
// whose meaning depends on which is not editing, it is guessing.
//
// Every error comes back as an offsetError holding where in data the decoder
// stopped, and every member holds where in data its value starts.
func parseObject(data []byte, whole bool) (_ []member, err error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	closed := false
	defer func() {
		if err != nil {
			err = refusal(data, dec.InputOffset(), closed, err)
		}
	}()

	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, errors.New("not a JSON object")
	}

	var members []member
	seen := map[string]bool{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, errors.New("object key is not a string")
		}
		if seen[key] {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		seen[key] = true

		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		// The decoder stands at the end of the value it just returned.
		members = append(members, member{key: key, raw: raw, at: posAt(int(dec.InputOffset()) - len(raw))})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	closed = true
	if whole {
		if _, err := dec.Token(); err != io.EOF {
			return nil, errors.New("trailing data after the top-level object")
		}
	}
	return members, nil
}

// parseArray splits the value of hooks.<event> into its entries.
func parseArray(event string, data []byte) ([]json.RawMessage, error) {
	if isNull(data) {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))

	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '[' {
		return nil, fmt.Errorf("hook event %q is not an array", event)
	}

	var items []json.RawMessage
	for dec.More() {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		items = append(items, raw)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return items, nil
}

// Rendering uses two-space indentation, which is what Claude Code itself
// writes, so a file this program has touched looks like one Claude Code has.

func writeObject(members []member, depth int) []byte {
	if len(members) == 0 {
		return []byte("{}")
	}
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, m := range members {
		b.WriteString(indent(depth + 1))
		// Marshalling a string cannot fail: invalid UTF-8 is replaced, not refused.
		key, _ := json.Marshal(m.key)
		b.Write(key)
		b.WriteString(": ")
		b.Write(m.raw)
		if i < len(members)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(indent(depth))
	b.WriteByte('}')
	return b.Bytes()
}

func writeArray(items []json.RawMessage, depth int) []byte {
	if len(items) == 0 {
		return []byte("[]")
	}
	var b bytes.Buffer
	b.WriteString("[\n")
	for i, item := range items {
		b.WriteString(indent(depth + 1))
		b.Write(item)
		if i < len(items)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(indent(depth))
	b.WriteByte(']')
	return b.Bytes()
}

// Indent returns the indentation for one nesting depth.
func Indent(depth int) string { return indent(depth) }

func indent(depth int) string { return strings.Repeat("  ", depth) }
