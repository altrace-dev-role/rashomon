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
)

// member is one key of a JSON object, with its value held raw.
type member struct {
	key string
	raw json.RawMessage
}

// Document is a settings file as ordered raw members.
type Document struct {
	members []member
}

// Load reads a settings file. An absent file and an empty one are both an
// empty document, because a machine that has never run Claude Code has
// neither, and refusing to install on such a machine would be refusing the
// first user.
func Load(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Document{}, nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse builds a Document from bytes.
func Parse(data []byte) (*Document, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return &Document{}, nil
	}
	members, err := parseObject(data, true)
	if err != nil {
		return nil, err
	}
	return &Document{members: members}, nil
}

// Get returns a top-level value's raw bytes.
func (d *Document) Get(key string) (json.RawMessage, bool) {
	if m := d.find(key); m != nil {
		return m.raw, true
	}
	return nil, false
}

// Bytes renders the document. Values not edited are emitted as they were read.
func (d *Document) Bytes() []byte {
	return append(writeObject(d.members, 0), '\n')
}

const hooksKey = "hooks"

// HookEntries returns the raw matcher groups under hooks.<event>, in order.
func (d *Document) HookEntries(event string) ([]json.RawMessage, error) {
	events, err := d.hookEvents()
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		if e.key == event {
			return parseArray(e.raw)
		}
	}
	return nil, nil
}

// SetHookEntries replaces hooks.<event> with entries. Every other event under
// hooks, and every other top-level key, is carried over byte for byte.
func (d *Document) SetHookEntries(event string, entries []json.RawMessage) error {
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
		h.raw = obj
	} else {
		d.members = append(d.members, member{key: hooksKey, raw: obj})
	}
	return nil
}

func (d *Document) hookEvents() ([]member, error) {
	h := d.find(hooksKey)
	if h == nil || isNull(h.raw) {
		return nil, nil
	}
	events, err := parseObject(h.raw, false)
	if err != nil {
		return nil, fmt.Errorf("settings: hooks is not an object: %w", err)
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
func parseObject(data []byte, whole bool) ([]member, error) {
	dec := json.NewDecoder(bytes.NewReader(data))

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
		members = append(members, member{key: key, raw: raw})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if whole {
		if _, err := dec.Token(); err != io.EOF {
			return nil, errors.New("trailing data after the top-level object")
		}
	}
	return members, nil
}

func parseArray(data []byte) ([]json.RawMessage, error) {
	if isNull(data) {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))

	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '[' {
		return nil, errors.New("settings: hook event is not an array")
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
