package hook

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestHook_AWithheldPathReachesTheWriterWhole: the record-scope gate and the
// caps withhold a declaration's file_path from the record, never from the
// overlay writer, which takes the path the declaration decoded before either
// ran (handle.go's declared). At v5 a path over store.MaxPathBytes is left
// out as oversize (an absolute path padded with "./" segments, which the
// writer cleans to the file, so no file system refuses it for its length),
// and the writer's one group names the file, at the declaration's seq. At v4
// the record leaves the path out as no_consent and the writer keeps nothing,
// since only v5 keeps content: no log, no group. Break: cut the path in place
// where the caps withhold it; the v5 row's writer then names no Go file and
// writes no group. Break: keep content below v5; the v4 row finds a group.
func TestHook_AWithheldPathReachesTheWriterWhole(t *testing.T) {
	for _, tc := range []struct {
		name, scope, reason string
		pad, kept           bool
	}{
		{"v4", store.RecordScopeV4, "no_consent", false, false},
		{"v5 over MaxPathBytes", store.RecordScopeV5, "oversize", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOvFix(t, "1\n")
			if err := store.WriteRecordScopeMarker(f.st.Root(), tc.scope); err != nil {
				t.Fatal(err)
			}
			path := f.file
			if tc.pad {
				dot := "." + string(filepath.Separator)
				path = f.repo + string(filepath.Separator) + strings.Repeat(dot, store.MaxPathBytes/len(dot)+1) + filepath.FromSlash(ovRel)
				if len(path) <= store.MaxPathBytes || filepath.Clean(path) != f.file {
					t.Fatalf("padded path of %d bytes cleans to %s; want over %d bytes and %s", len(path), filepath.Clean(path), store.MaxPathBytes, f.file)
				}
			}
			in := f.edit("func Old() {}", "func Old() {}\n\nfunc Added() {}")
			in["file_path"] = path
			f.pre("toolu_wh", "Edit", in)

			run, err := f.st.ReadRun(ovSess)
			if err != nil {
				t.Fatal(err)
			}
			var seq int64
			for _, d := range run.Declarations {
				if d.ToolUseID != "toolu_wh" {
					continue
				}
				seq = d.Seq
				if d.FilePath != nil || d.FilePathWithheld == nil || *d.FilePathWithheld != tc.reason {
					t.Errorf("declaration file_path %v withheld %v; want none in the record, withheld as %s", d.FilePath, d.FilePathWithheld, tc.reason)
				}
			}
			if seq == 0 {
				t.Fatal("no declaration of toolu_wh")
			}
			groups, _, ok := f.read()
			if !tc.kept {
				if ok || len(groups) != 0 {
					t.Errorf("at %s the log exists %v with %d groups; want no log and no group", tc.scope, ok, len(groups))
				}
				return
			}
			if !ok || len(groups) != 1 {
				t.Fatalf("log exists %v, %d groups; want the writer's one group", ok, len(groups))
			}
			if g := groups[0]; g.File != graph.FileID(ovRel) || g.Seq != seq || g.Kind != graph.OverlayDeclared {
				t.Errorf("group file %s seq %d kind %s; want %s at the declaration's seq %d, kind %s", g.File, g.Seq, g.Kind, graph.FileID(ovRel), seq, graph.OverlayDeclared)
			}
		})
	}
}
