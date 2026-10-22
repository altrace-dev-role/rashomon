package acceptance

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
)

// marshalerType is encoding/json.Marshaler, checked against both t and *t:
// a type can implement it on either receiver, and encoding/json dispatches
// to whichever one matches.
var marshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()

// typeKeyPaths lists every key path a document's GO TYPE can carry, with
// each leaf's kind appended after a colon, from the type alone -- no
// instance, no fixture. A map's key is never a real one: it is always
// "{}", since any key a run's own data happened to produce is DATA, not
// SCHEMA, and pinning it as a path is what let a renamed or re-typed field
// inside a map's value pass unnoticed. An array or slice is "[]" the same
// way docKeyPaths already writes it, so the two agree on everything but
// the two things an instance cannot show at all: a branch the fixture left
// empty or null, and a leaf's type.
//
// A type with its own MarshalJSON (spend.Cost, spend.SilentFailureTurns) is
// walked from its ZERO VALUE's actual marshalled bytes instead of its
// fields: the struct's own field names are not what reaches the wire, and
// reading them directly named "Nano" and "Priced" for a document that has
// never written either. Both of this tree's marshalers happen to produce a
// fixed shape regardless of the value marshalled, which is what makes a
// zero value a safe stand-in -- unlike a map, nothing here is answering
// with THIS RUN's data.
func typeKeyPaths(t reflect.Type) []byte {
	seen := map[string]bool{}
	walkType("", t, seen)
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return []byte(strings.Join(paths, "\n") + "\n")
}

func walkType(prefix string, t reflect.Type, seen map[string]bool) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Implements(marshalerType) || reflect.PointerTo(t).Implements(marshalerType) {
		walkMarshaler(prefix, t, seen)
		return
	}
	switch t.Kind() {
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" && !f.Anonymous {
				continue // unexported, never marshalled
			}
			name, rest, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" && rest == "" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			p := name
			if prefix != "" {
				p = prefix + "." + p
			}
			walkType(p, f.Type, seen)
		}
	case reflect.Slice, reflect.Array:
		walkType(prefix+"[]", t.Elem(), seen)
	case reflect.Map:
		key := prefix + ".{}"
		if prefix == "" {
			key = "{}"
		}
		walkType(key, t.Elem(), seen)
	default:
		seen[prefix+": "+t.Kind().String()] = true
	}
}

// walkMarshaler marshals t's zero value and walks the resulting bytes, for
// a type whose own field reflection would not match what it writes.
func walkMarshaler(prefix string, t reflect.Type, seen map[string]bool) {
	b, err := json.Marshal(reflect.New(t).Elem().Interface())
	if err != nil {
		seen[prefix+": marshal-error"] = true
		return
	}
	var doc any
	if json.Unmarshal(b, &doc) != nil {
		seen[prefix+": unmarshal-error"] = true
		return
	}
	walkJSONValue(prefix, doc, seen)
}

// walkJSONValue is walkMarshaler's own walk, over a decoded JSON value
// rather than a reflect.Type: a marshaler's output is read as data, since
// there is no type left to read it from once MarshalJSON has run.
func walkJSONValue(prefix string, v any, seen map[string]bool) {
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			walkJSONValue(p, child, seen)
		}
	case []any:
		for _, child := range v {
			walkJSONValue(prefix+"[]", child, seen)
		}
	case nil:
		seen[prefix+": null"] = true
	case float64:
		seen[prefix+": number"] = true
	case string:
		seen[prefix+": string"] = true
	case bool:
		seen[prefix+": bool"] = true
	}
}
