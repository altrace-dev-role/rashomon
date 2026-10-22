package recap

import (
	"reflect"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestForCodex: only hook_entry_unresolved is set aside. Any other reason
// still speaks, with the turn still unverified; the digest it was given is not
// changed, because the caller may still use it.
func TestForCodex(t *testing.T) {
	t.Run("the permanent reason alone", func(t *testing.T) {
		d := &digest.Digest{Unknown: true}
		d.Coverage.State = store.StateUnverified
		d.Coverage.Reasons = []string{store.ReasonHookEntryUnresolved}
		got := ForCodex(d)
		if got.Coverage.State != store.StateVerified || len(got.Coverage.Reasons) != 0 || got.Unknown {
			t.Errorf("ForCodex: state %q, reasons %v, unknown %v; want verified, none, false",
				got.Coverage.State, got.Coverage.Reasons, got.Unknown)
		}
		if d.Coverage.State != store.StateUnverified || !reflect.DeepEqual(d.Coverage.Reasons, []string{store.ReasonHookEntryUnresolved}) || !d.Unknown {
			t.Errorf("ForCodex changed the digest it was given: %+v", d.Coverage)
		}
	})

	t.Run("beside another reason", func(t *testing.T) {
		d := &digest.Digest{Unknown: true}
		d.Coverage.State = store.StateUnverified
		d.Coverage.Reasons = []string{store.ReasonHookEntryUnresolved, store.ReasonLockTimeout}
		got := ForCodex(d)
		if got.Coverage.State != store.StateUnverified || !reflect.DeepEqual(got.Coverage.Reasons, []string{store.ReasonLockTimeout}) || !got.Unknown {
			t.Errorf("ForCodex: state %q, reasons %v, unknown %v; want unverified, [lock_timeout], true",
				got.Coverage.State, got.Coverage.Reasons, got.Unknown)
		}
		if line, ok := Line(got, "s", false); !ok || line == "" {
			t.Errorf("a turn with another coverage reason said nothing")
		}
	})
}
