package disclose

import (
	"reflect"
	"strings"
	"testing"
)

func jsonTags(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		out = append(out, strings.Split(t.Field(i).Tag.Get("json"), ",")[0])
	}
	return out
}

// The inventory's keys, a literal copy of 7.2.2.
func TestInventoryCarriesTheSpecsKeys(t *testing.T) {
	cases := map[reflect.Type][]string{
		reflect.TypeOf(Inventory{}):     {"version", "scope_version", "entries", "files", "settings_writes", "outputs", "never_emit", "never_write", "switches", "env_vars", "screen_paragraphs"},
		reflect.TypeOf(HookEntry{}):     {"event", "matcher", "subcommand", "timeout_s", "why", "prints", "owner"},
		reflect.TypeOf(OutputRule{}):    {"event", "field", "audience", "mirror_verified", "mirror_source", "template_ids"},
		reflect.TypeOf(FileRule{}):      {"base", "pattern", "mode", "purpose", "holds", "never_holds", "removed_by", "owner"},
		reflect.TypeOf(SettingsWrite{}): {"file", "json_ptr", "writer"},
		reflect.TypeOf(Forbidden{}):     {"event", "field", "why", "except"},
		reflect.TypeOf(Switch{}):        {"name", "default", "command", "stored", "ratchet"},
		reflect.TypeOf(EnvVar{}):        {"name", "effect", "test_only"},
		reflect.TypeOf(Paragraph{}):     {"owner", "text", "cite"},
	}
	for typ, want := range cases {
		if got := jsonTags(typ); !reflect.DeepEqual(got, want) {
			t.Errorf("%s keys = %v, want %v", typ.Name(), got, want)
		}
	}
}

func TestTRU8b_ScopeVersionIsPinned(t *testing.T) {
	if ScopeVersion != 3 {
		t.Errorf("ScopeVersion = %d; a bump re-asks consent and must be deliberate", ScopeVersion)
	}
}

// A caller that ignores the error must not see a scope version attached to
// an otherwise-empty inventory: that reads as consent recorded for a build
// that has nothing in it.
func TestDefaultIsTheZeroInventoryUntilFilled(t *testing.T) {
	inv, err := Default()
	if err == nil {
		t.Error("Default() is a stub in this build and must say so")
	}
	if !reflect.DeepEqual(inv, Inventory{}) {
		t.Errorf("Default() = %+v, want the zero Inventory", inv)
	}
}
