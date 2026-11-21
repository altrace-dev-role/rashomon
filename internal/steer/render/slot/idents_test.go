package slot

import (
	"testing"
)

var _ Value = IdentList{}

// The count clause at 0, 1 and 2, and a list longer than any other list type
// renders: IdentList has no cap of its own.
func TestIdentList_TextQuotesEachNameThenCountsTheRest(t *testing.T) {
	cases := []struct {
		names []Ident
		more  int
		want  string
	}{
		{[]Ident{"place_order"}, 0, "'place_order'"},
		{[]Ident{"place_order", "refund.issue"}, 0, "'place_order', 'refund.issue'"},
		{[]Ident{"place_order"}, 1, "'place_order' and 1 more"},
		{[]Ident{"place_order", "refund.issue"}, 2, "'place_order', 'refund.issue' and 2 more"},
		{[]Ident{"a", "b", "c", "d", "e", "f", "g"}, 0, "'a', 'b', 'c', 'd', 'e', 'f', 'g'"},
	}
	for _, c := range cases {
		if got := NewIdentList(c.names, c.more).Text(); got != c.want {
			t.Errorf("NewIdentList(%q, %d).Text() = %q, want %q", c.names, c.more, got, c.want)
		}
	}
}

func TestIdentList_ValidateRefusesNoNameANegativeCountAndABadName(t *testing.T) {
	good := NewIdentList([]Ident{"place_order", "refund.issue"}, 3)
	if err := good.Validate(); err != nil {
		t.Fatalf("a good list: Validate = %v", err)
	}
	for name, l := range map[string]IdentList{
		"no name":        NewIdentList(nil, 2),
		"negative count": NewIdentList([]Ident{"place_order"}, -1),
		"a bad name":     NewIdentList([]Ident{"place_order", "a b"}, 0),
		"a numeral name": NewIdentList([]Ident{"1abc"}, 0),
	} {
		if err := l.Validate(); err == nil {
			t.Errorf("%s: Validate passed", name)
		}
		if _, err := Bind(l); err == nil {
			t.Errorf("%s: Bind passed", name)
		}
	}
}

func TestIdentList_BindHoldsItsOwnCopy(t *testing.T) {
	names := []Ident{"place_order", "refund.issue"}
	l := NewIdentList(names, 1)
	names[0] = "changed"
	got := l.Idents()
	got[1] = "changed"
	v, err := Bind(l)
	if err != nil {
		t.Fatalf("Bind = %v", err)
	}
	b, ok := v.(IdentList)
	if !ok {
		t.Fatalf("Bind returned %T", v)
	}
	if want := "'place_order', 'refund.issue' and 1 more"; b.Text() != want {
		t.Errorf("bound Text() = %q, want %q", b.Text(), want)
	}
	if b.More() != 1 || len(b.Idents()) != 2 {
		t.Errorf("bound More() = %d, len(Idents()) = %d, want 1 and 2", b.More(), len(b.Idents()))
	}
}
