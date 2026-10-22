package shape

import (
	"reflect"
	"testing"
	"unsafe"
)

// TestRubricLLD9_WordResetClearsEveryField: the lexer's per-word state is one
// value, and emitting a word resets all of it at once. Every field of word is
// set to a non-zero value through reflection -- so a field added later is set
// too, without editing this test -- and reset must leave exactly the fresh
// word. The hand-written reset this replaced cleared four fields by name in
// one closure and set a fifth in another, and a field missed there carries
// one word's quoting, expansion or backtick count into the next.
func TestRubricLLD9_WordResetClearsEveryField(t *testing.T) {
	var w word
	v := reflect.ValueOf(&w).Elem()
	for i := 0; i < v.NumField(); i++ {
		sf := v.Type().Field(i)
		f := reflect.NewAt(sf.Type, unsafe.Pointer(v.Field(i).UnsafeAddr())).Elem()
		switch f.Kind() {
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int:
			f.SetInt(7)
		case reflect.Struct:
			if sf.Type != reflect.TypeOf(w.text) {
				t.Fatalf("word.%s is a %s; teach this test to set it", sf.Name, sf.Type)
			}
			w.text.WriteString("carried")
		default:
			t.Fatalf("word.%s is a %s; teach this test to set it", sf.Name, sf.Type)
		}
	}
	w.reset()
	if fresh := newWord(); !reflect.DeepEqual(w, fresh) {
		t.Fatalf("after reset the word is %+v, want the fresh word %+v: a field survived into the next word", w, fresh)
	}
	if w.text.Len() != 0 || w.quotedAt != -1 {
		t.Fatalf("after reset: %d bytes of text, quotedAt %d; want none and -1", w.text.Len(), w.quotedAt)
	}
}
