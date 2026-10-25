// Package recordtest checks a copy between two record structs field by field,
// by reflection, so that a field added to either struct cannot be left out of
// a copy unnoticed: a grep for the copy sites finds the fields its author
// remembered, and this reads every field both structs have. It is for test
// files only; nothing outside a test imports it.
package recordtest

import (
	"fmt"
	"reflect"
	"strings"
)

// T is what AssertCopy reports through: a *testing.T, or a recorder in a test
// of AssertCopy itself.
type T interface {
	Helper()
	Errorf(format string, args ...any)
}

// Recorder is a T that keeps what it is told, for a test that checks
// AssertCopy fails where it should and says why.
type Recorder struct{ Errors []string }

func (r *Recorder) Helper() {}

func (r *Recorder) Errorf(format string, args ...any) {
	r.Errors = append(r.Errors, fmt.Sprintf(format, args...))
}

// fields are t's fields as a copy sees them, by name: an embedded struct's own
// fields stand in its place.
func fields(t reflect.Type) []reflect.StructField {
	var out []reflect.StructField
	for _, f := range reflect.VisibleFields(t) {
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			continue
		}
		out = append(out, f)
	}
	return out
}

// maxDepth bounds Fill's descent, so a type that holds itself through a
// pointer stops with its name rather than filling forever.
const maxDepth = 8

// Fill sets every exported field of the struct v, which must be settable, to
// a value that is not zero and that no other field it sets holds: a string
// holds the field's name (its path from the struct, as Execution.Writer), a
// number counts up from 1001 (from 1 to 100 where the type is too small for
// that), a bool is true, a pointer points at a value filled the same way, a
// slice or a map holds one such element, an empty interface holds the name,
// and a struct is filled field by field.
func Fill(v reflect.Value) {
	n := 1000
	fill(v, v.Type().Name(), &n, 0)
}

func fill(v reflect.Value, name string, n *int, depth int) {
	if depth > maxDepth {
		panic("recordtest: Fill is past its depth at " + name)
	}
	switch v.Kind() {
	case reflect.Struct:
		for _, f := range fields(v.Type()) {
			if f.IsExported() {
				fill(v.FieldByIndex(f.Index), name+"."+f.Name, n, depth+1)
			}
		}
	case reflect.Pointer:
		p := reflect.New(v.Type().Elem())
		fill(p.Elem(), name, n, depth+1)
		v.Set(p)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fill(s.Index(0), name, n, depth+1)
		v.Set(s)
	case reflect.Array:
		for i := range v.Len() {
			fill(v.Index(i), fmt.Sprintf("%s[%d]", name, i), n, depth+1)
		}
	case reflect.Map:
		m := reflect.MakeMapWithSize(v.Type(), 1)
		k := reflect.New(v.Type().Key()).Elem()
		fill(k, name+".key", n, depth+1)
		e := reflect.New(v.Type().Elem()).Elem()
		fill(e, name, n, depth+1)
		m.SetMapIndex(k, e)
		v.Set(m)
	case reflect.Interface:
		s := reflect.ValueOf(name)
		if !s.Type().AssignableTo(v.Type()) {
			panic("recordtest: Fill cannot fill the interface " + v.Type().String() + " at " + name)
		}
		v.Set(s)
	case reflect.String:
		v.SetString(name)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		*n++
		x := int64(*n)
		if v.OverflowInt(x) {
			x = x%100 + 1
		}
		v.SetInt(x)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		*n++
		x := uint64(*n)
		if v.OverflowUint(x) {
			x = x%100 + 1
		}
		v.SetUint(x)
	case reflect.Float32, reflect.Float64:
		*n++
		v.SetFloat(float64(*n) + 0.5)
	default:
		panic("recordtest: Fill cannot fill a " + v.Kind().String() + " at " + name)
	}
}

// AssertCopy checks one copy from the struct src into a struct that held
// before and holds after once the copy ran (any of the three may be a pointer
// to one). carried names each field of after the copy sets from src's field
// of the same name, or "Dst=Src" where the names differ; notCarried names
// each field of src the copy does not read; withheld names each field of
// after the copy does not set. Both sides are complete: every field of src's
// struct is carried or notCarried, and every field of after's is carried or
// withheld, exactly once, an embedded struct's fields by their own names. A
// field in neither list fails, naming it, which is what a field added to
// either struct does until someone says, at this copy, which list it is in.
//
// Each carried field of src is set, differs from what after's field held
// before -- otherwise a copy that missed it would pass -- and equals after's
// field once the copy ran. Each withheld field of after is what it was before.
func AssertCopy(t T, src, before, after reflect.Value, carried, notCarried, withheld []string) {
	t.Helper()
	src, before, after = reflect.Indirect(src), reflect.Indirect(before), reflect.Indirect(after)
	if before.Type() != after.Type() {
		t.Errorf("before is a %s and after a %s: one struct, before and after the copy", before.Type(), after.Type())
		return
	}
	var from, to []string
	for _, c := range carried {
		d, s := names(c)
		to, from = append(to, d), append(from, s)
	}
	complete(t, src.Type(), append(from, notCarried...), "say whether the copy carries it or not")
	complete(t, after.Type(), append(to, withheld...), "say whether the copy carries it or withholds it")
	for _, c := range carried {
		d, s := names(c)
		sv, bv, av := src.FieldByName(s), before.FieldByName(d), after.FieldByName(d)
		switch {
		case !sv.IsValid() || !av.IsValid():
			// complete named it.
		case sv.IsZero():
			t.Errorf("carried %s: %s.%s is zero, so a copy that missed it would pass", c, src.Type(), s)
		case equal(bv, sv):
			t.Errorf("carried %s: %s.%s already holds %s before the copy, so a copy that missed it would pass", c, after.Type(), d, show(sv))
		case !equal(av, sv):
			t.Errorf("%s.%s is %s, not %s.%s's %s: the copy does not carry it", after.Type(), d, show(av), src.Type(), s, show(sv))
		}
	}
	for _, w := range withheld {
		bv, av := before.FieldByName(w), after.FieldByName(w)
		if av.IsValid() && !equal(bv, av) {
			t.Errorf("%s.%s is %s, not %s as before: withheld, the copy must leave it", after.Type(), w, show(av), show(bv))
		}
	}
}

// complete fails for each field of typ that listed does not name exactly
// once, and for each name in listed that is no field of typ.
func complete(t T, typ reflect.Type, listed []string, ask string) {
	t.Helper()
	count := map[string]int{}
	for _, n := range listed {
		count[n]++
	}
	have := map[string]bool{}
	for _, f := range fields(typ) {
		have[f.Name] = true
		switch count[f.Name] {
		case 0:
			t.Errorf("%s.%s is in neither list: %s", typ, f.Name, ask)
		case 1:
		default:
			t.Errorf("%s.%s is listed %d times: name it once", typ, f.Name, count[f.Name])
		}
	}
	seen := map[string]bool{}
	for _, n := range listed {
		if !have[n] && !seen[n] {
			seen[n] = true
			t.Errorf("%s is not a field of %s", n, typ)
		}
	}
}

// equal reports two fields holding the same value, a pointer's target and
// not its address.
func equal(a, b reflect.Value) bool {
	return reflect.DeepEqual(a.Interface(), b.Interface())
}

// names splits a carried entry into dst's field name and src's: "Name" is the
// same name in both, "Dst=Src" a copy between two names.
func names(c string) (dst, src string) {
	if d, s, ok := strings.Cut(c, "="); ok {
		return d, s
	}
	return c, c
}

// show is a field's value for a message: a pointer's target, or nil.
func show(v reflect.Value) string {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return "nil"
		}
		v = v.Elem()
	}
	if v.IsZero() {
		return "zero"
	}
	return fmt.Sprintf("%q", fmt.Sprint(v.Interface()))
}
