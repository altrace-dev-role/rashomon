package fold

import (
	"reflect"
	"strings"
	"testing"
)

// jsonTags lists a struct type's JSON keys in field order.
func jsonTags(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		out = append(out, strings.Split(t.Field(i).Tag.Get("json"), ",")[0])
	}
	return out
}

func TestFoldFileCarriesTheSpecsKeys(t *testing.T) {
	cases := map[reflect.Type][]string{
		reflect.TypeOf(File{}):     {"v", "writer", "dev", "inode", "records", "spill", "coverage", "prompts", "digest", "verify"},
		reflect.TypeOf(Cursor{}):   {"offset", "seq", "size_seen", "generation"},
		reflect.TypeOf(Mark{}):     {"prompt_id", "first_offset", "first_seq", "last_seq", "stopped"},
		reflect.TypeOf(DigestSt{}): {"start_recorded", "hook_entry_at_start", "skipped", "calls"},
	}
	for typ, want := range cases {
		if got := jsonTags(typ); !reflect.DeepEqual(got, want) {
			t.Errorf("%s keys = %v, want %v", typ.Name(), got, want)
		}
	}
	if MaxBytes != 256<<10 || VerifyMaxBytes != 64<<10 || MaxPrompts != 64 || Version != 1 {
		t.Errorf("caps changed: MaxBytes %d VerifyMaxBytes %d MaxPrompts %d Version %d", MaxBytes, VerifyMaxBytes, MaxPrompts, Version)
	}
}
