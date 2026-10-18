package report

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// keySet is a nested set of JSON keys: a nil value is a leaf, a map is an
// object or an array of objects.
type keySet map[string]keySet

// linkKeys is a chain link's keys, shared by the three places links appear.
var linkKeys = keySet{
	"seq": nil, "tool_use_id": nil, "tool_name": nil, "program": nil, "verb_class": nil,
	"outcome": nil, "outcomes": nil, "execution_records": nil, "declared_hosts": nil, "ssh_hosts": nil,
}

// allowlist is a literal copy of the keys each report section carries after
// the proxy era, as the specification records them. The point of a literal is
// that a key added to a struct lands here as a difference, and a reviewer
// reads it as a contract change rather than as a field.
var allowlist = map[string]keySet{
	"destinations": {
		"declared_hosts": nil, "ssh_hosts": nil, "executed_not_as_declared": nil,
		"rewritten": {"tool_use_id": nil, "tool_name": nil, "program": nil, "verb_class": nil},
		"novelty":   {"available": nil, "reason": nil, "established": nil, "known_hosts": nil, "hosts": nil, "evicted": nil},
	},
	"nono": {
		"configured": nil, "observed": nil, "reason": nil, "allowed": nil, "denied": nil,
		// The client's own traffic in the trail, split by decision (#53): read
		// from the trail alone, so it survives the proxy era's removal.
		"client_plane_allowed": nil, "client_plane_denied": nil,
		"unknown_decisions": nil, "unknown_modes": nil, "skipped": nil, "unparseable_targets": nil,
		"inherited": nil, "sessions": nil, "window_open": nil,
	},
	"families": {
		"families":      {"name": nil, "programs": nil, "calls": nil, "hosts_declared": nil},
		"not_exercised": nil,
	},
	"chains": {
		"prompts":      {"transcript_path": nil, "prompt_id": nil, "links": linkKeys},
		"unattributed": linkKeys,
		"dropped":      linkKeys,
	},
}

// omittable are the keys a marshal drops when empty, as the specification
// lists them, and no others. Everything else must be present in every
// rendering, so a consumer can tell an empty list from a section that was
// never built. Both reasons are empty on the healthy path; a link's program is
// empty for every tool but Bash and was omitted before this change.
// rewritten[].program is not among them: a non-Bash call shows "program": ""
// there and no program key on its link.
var omittable = map[string]bool{
	"destinations.novelty.reason":  true,
	"destinations.novelty.evicted": true,
	"nono.reason":                  true,
	"chains.prompts.links.program": true,
	"chains.unattributed.program":  true,
	"chains.dropped.program":       true,
}

var sectionTypes = map[string]reflect.Type{
	"destinations": reflect.TypeOf(Destinations{}),
	"nono":         reflect.TypeOf(Nono{}),
	"families":     reflect.TypeOf(FamilyCoverage{}),
	"chains":       reflect.TypeOf(Chains{}),
}

// TRU-57. The four report sections carry exactly the allowlisted keys, nested,
// read from the struct tags; and nothing but the listed keys is omitempty or
// omitzero.
func TestTRU57_ReportSectionsCarryExactlyTheAllowlistedKeys(t *testing.T) {
	for name, typ := range sectionTypes {
		got := jsonKeys(typ)
		if !reflect.DeepEqual(got, allowlist[name]) {
			t.Errorf("%s: keys differ from the allowlist\n got: %s\nwant: %s", name, render(got), render(allowlist[name]))
		}
		for _, path := range omitemptyPaths(typ, name) {
			if !omittable[path] {
				t.Errorf("%s is dropped when empty, and only %s may be", path, sortedNames(omittable))
			}
		}
	}
}

// TestTRU57_AMarshalledReportCarriesExactlyTheAllowlistedKeys reads the keys
// off real output rather than off the tags: a session is marshalled, decoded,
// and every object in the four sections is compared with the allowlist in
// both directions. A key dropped by omitzero, emitted by a tag of "-,",
// promoted from an embedded struct or written by a MarshalJSON method is a
// difference here, and the tag walk above sees none of them.
//
// Twice. With every leaf at its zero value, the keys missing are exactly the
// omittable ones: every other key is present at zero, and each omittable key
// really is dropped when empty, so the set holds no exemption the code does
// not use. With every omittable value filled, nothing is missing.
func TestTRU57_AMarshalledReportCarriesExactlyTheAllowlistedKeys(t *testing.T) {
	for _, tc := range []struct {
		name    string
		filled  string
		missing map[string]bool
	}{
		{"every leaf at zero", "", omittable},
		{"every omittable value filled", "filled", map[string]bool{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(sectionFixture(tc.filled))
			if err != nil {
				t.Fatal(err)
			}
			var doc map[string]any
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Fatal(err)
			}
			missing := map[string]bool{}
			for name := range sectionTypes {
				compareKeys(t, name, doc[name], allowlist[name], missing)
			}
			if !reflect.DeepEqual(missing, tc.missing) {
				t.Errorf("keys missing from the output: %s\nwant exactly: %s", sortedNames(missing), sortedNames(tc.missing))
			}
		})
	}
}

// sectionFixture is a session whose four sections hold one element in every
// list -- a link in prompts, unattributed and dropped, a rewritten call, a
// family -- and every leaf at its zero value, so a family names 0 hosts and
// novelty knows 0. Only the omittable values hold filled: empty, they are a
// non-Bash call and a healthy path; filled, a Bash call and a stated reason.
func sectionFixture(filled string) Session {
	link := Link{Program: filled}
	evicted := 0
	if filled != "" {
		evicted = 1
	}
	return Session{
		Destinations: Destinations{Rewritten: []Rewritten{{}}, Novelty: Novelty{Reason: filled, Evicted: evicted}},
		Nono:         Nono{Reason: filled},
		Families:     FamilyCoverage{Families: []Family{{}}},
		Chains: Chains{
			Prompts:      []Chain{{Links: []Link{link}}},
			Unattributed: []Link{link},
			Dropped:      []Link{link},
		},
	}
}

// compareKeys walks one decoded value against its allowlist entry. A key in
// the output that the list does not name fails here; a listed key the output
// does not carry is recorded in missing, by path, for the caller to judge.
func compareKeys(t *testing.T, path string, v any, want keySet, missing map[string]bool) {
	t.Helper()
	switch x := v.(type) {
	case []any:
		for _, el := range x {
			compareKeys(t, path, el, want, missing)
		}
	case map[string]any:
		for key := range x {
			if _, ok := want[key]; !ok {
				t.Errorf("%s.%s is in the output and not in the allowlist", path, key)
			}
		}
		for key, sub := range want {
			got, ok := x[key]
			if !ok {
				missing[path+"."+key] = true
				continue
			}
			if sub != nil {
				compareKeys(t, path+"."+key, got, sub, missing)
			}
		}
	default:
		t.Errorf("%s is %v in the output, where the allowlist has objects", path, v)
	}
}

// TestTRU57_EmptySectionsStillCarryTheirKeys marshals each section's zero
// value: every top-level key but an omittable one must be present, as a
// consumer relies on.
func TestTRU57_EmptySectionsStillCarryTheirKeys(t *testing.T) {
	for name, typ := range sectionTypes {
		data, err := json.Marshal(reflect.Zero(typ).Interface())
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]json.RawMessage
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		for key := range allowlist[name] {
			if _, ok := got[key]; !ok && !omittable[name+"."+key] {
				t.Errorf("%s: key %q is missing from the zero value's rendering: %s", name, key, data)
			}
		}
	}
}

// jsonKeys reads the JSON keys a type renders, descending through pointers,
// slices and nested structs. An exported field without a json tag renders
// under its Go name, so it is listed under that name rather than skipped: a
// field that slipped in untagged is a contract change too.
func jsonKeys(t reflect.Type) keySet {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	out := keySet{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		switch name {
		case "-":
			continue
		case "":
			name = f.Name
		}
		out[name] = jsonKeys(f.Type)
	}
	return out
}

// omitemptyPaths lists the dotted paths of every field under a type that a
// marshal drops when empty: omitempty, or omitzero, which encoding/json has
// honoured since Go 1.24.
func omitemptyPaths(t reflect.Type, prefix string) []string {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var out []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		parts := strings.Split(f.Tag.Get("json"), ",")
		name := parts[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		path := prefix + "." + name
		for _, opt := range parts[1:] {
			if opt == "omitempty" || opt == "omitzero" {
				out = append(out, path)
			}
		}
		out = append(out, omitemptyPaths(f.Type, path)...)
	}
	return out
}

func render(k keySet) string {
	if k == nil {
		return ""
	}
	names := make([]string, 0, len(k))
	for name := range k {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		if sub := render(k[name]); sub != "" {
			parts = append(parts, name+"{"+sub+"}")
		} else {
			parts = append(parts, name)
		}
	}
	return strings.Join(parts, " ")
}

func sortedNames(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
