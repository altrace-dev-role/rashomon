package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// The store-root lines (D173): integrity/store-touched for a write under
// rashomon's store, with the path relative to the store root and without.
const (
	stRootLine = "rashomon record: this call wrote 'spec-trust.json' in rashomon's store, which holds the trust bindings; " +
		"no repository spec counts as trusted for the rest of this session."
	stRootUnnamedLine = "rashomon record: this call wrote a file in rashomon's store, which holds the trust bindings; " +
		"no repository spec counts as trusted for the rest of this session."
)

// boundST is a steer fixture whose cwd is a .git project with specOneGate
// as its repository spec, bound in the store's bindings through
// WriteSpecBindings, as the verb binds it.
func boundST(t *testing.T) *stFix {
	t.Helper()
	s := newSpecTree(t)
	s.mkdir(".git")
	path := s.put(s.base, specOneGate)
	_, digest, err := spec.Parse([]byte(specOneGate))
	if err != nil {
		t.Fatal(err)
	}
	f := newST(t)
	f.cwd = s.base
	writeBindings(t, f.st.Root(), spec.Binding{ProjectRoot: s.base, SpecPath: path, SpecDigest: digest, BoundAtMS: 1}, false)
	return f
}

// trustAt is the trust the steer step reads the spec at, judging a red
// `go test ./...` landed under id.
func (f *stFix) trustAt(id string) spec.Trust {
	f.t.Helper()
	_, view, ok := f.call(id, "Bash", goTest, 1).judge(stEnv(0))
	if !ok || view.Spec == nil {
		f.t.Fatalf("judge %s: ok %v, spec %+v; want a spec", id, ok, view.Spec)
	}
	return view.Spec.Trust
}

// TestST1k_AHandWrittenBindingsFileReadsUntrusted (D173): a bindings file
// the verb did not sign binds nothing in discovery: the plain JSON a Write
// or an echo makes, and the verb's own file with a binding edited under its
// MAC. The verb's file, unedited, binds.
func TestST1k_AHandWrittenBindingsFileReadsUntrusted(t *testing.T) {
	for _, c := range []struct {
		name string
		// forge rewrites the signed bindings file data; nil keeps it.
		forge func(data []byte) []byte
		want  spec.Trust
	}{
		{"signed by the verb", nil, spec.Trusted},
		{"plain JSON", func(data []byte) []byte {
			var f struct {
				Bindings json.RawMessage `json:"bindings"`
			}
			if err := json.Unmarshal(data, &f); err != nil {
				t.Fatal(err)
			}
			return []byte(`{"bindings":` + string(f.Bindings) + "}\n")
		}, spec.Untrusted},
		{"edited under the MAC", func(data []byte) []byte {
			return bytes.Replace(data, []byte(`"bound_at_unix_ms":1`), []byte(`"bound_at_unix_ms":2`), 1)
		}, spec.Untrusted},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := boundST(t)
			if c.forge != nil {
				path := filepath.Join(f.st.Root(), spec.TrustFile)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				forged := c.forge(data)
				if bytes.Equal(forged, data) {
					t.Fatal("the forgery changed nothing")
				}
				if err := os.WriteFile(path, forged, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if got := f.trustAt("toolu_r"); got != c.want {
				t.Errorf("trust %q, want %q", got, c.want)
			}
		})
	}
}

// TestST1l_AMACUnderAnotherKeyReadsUntrusted (D173): the verb's file read
// under another key, a key of another length, or no key binds nothing.
func TestST1l_AMACUnderAnotherKeyReadsUntrusted(t *testing.T) {
	for _, c := range []struct {
		name string
		key  []byte // nil removes the key file
	}{
		{"another key", bytes.Repeat([]byte{7}, spec.TrustKeyBytes)},
		{"a key of another length", bytes.Repeat([]byte{7}, spec.TrustKeyBytes-1)},
		{"no key", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := boundST(t)
			path := filepath.Join(f.st.Root(), spec.TrustKeyFile)
			var err error
			if c.key == nil {
				err = os.Remove(path)
			} else {
				err = os.WriteFile(path, c.key, 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := f.trustAt("toolu_r"); got != spec.Untrusted {
				t.Errorf("trust %q, want untrusted", got)
			}
		})
	}
}

// TestST1m_AWriteUnderTheStoreRootDropsTrust (D173): a call that writes
// under the store root is raised at its own post as integrity/store-touched,
// naming the store and the path relative to its root, to the user only; the
// spec bound before it reads trusted at a red before the write and
// untrusted at every red after it, the bindings file unchanged. A Write and
// a shell redirect are each such a call.
func TestST1m_AWriteUnderTheStoreRootDropsTrust(t *testing.T) {
	for _, c := range []struct {
		name  string
		tool  string
		input func(target string) map[string]any
	}{
		{"Write", "Write", func(target string) map[string]any { return map[string]any{"file_path": target, "content": "{}"} }},
		{"shell redirect", "Bash", func(target string) map[string]any { return map[string]any{"command": "printf x > " + target} }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := boundST(t)
			if got := f.trustAt("toolu_r1"); got != spec.Trusted {
				t.Fatalf("trust before the write %q, want trusted", got)
			}
			b := f.call("toolu_w", c.tool, c.input(filepath.Join(f.st.Root(), spec.TrustFile)), 0).Steer(stEnv(0))
			var out map[string]any
			if err := json.Unmarshal(b, &out); err != nil {
				t.Fatalf("the write printed %q: %v", b, err)
			}
			if msg, _ := out["systemMessage"].(string); msg != stRootLine {
				t.Errorf("systemMessage %q, want %q", msg, stRootLine)
			}
			if _, model := out["hookSpecificOutput"]; model {
				t.Errorf("a model-visible key printed: %s", b)
			}
			if got := f.trustAt("toolu_r2"); got != spec.Untrusted {
				t.Errorf("trust after the write %q, want untrusted", got)
			}
		})
	}
}

// TestST1r_TheStoreRootLinesRender (D173): each store-root row renders
// through Route once, with the path and without it (a write of the store
// root itself, whose relative path is empty), as a delivered flag_only line
// with no model text, never render_refused.
func TestST1r_TheStoreRootLinesRender(t *testing.T) {
	for _, c := range []struct {
		name   string
		target func(root string) string
		want   string
	}{
		{"with the path", func(root string) string { return filepath.Join(root, spec.TrustFile) }, stRootLine},
		{"without it", func(root string) string { return root }, stRootUnnamedLine},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newST(t)
			f.call("toolu_w", "Write", map[string]any{"file_path": c.target(f.st.Root()), "content": "{}"}, 0).Steer(stEnv(0))
			lines := steerLines(t, f.st)
			if len(lines) != 1 {
				t.Fatalf("%d steer lines, want 1", len(lines))
			}
			l := lines[0]
			if l.Pattern != string(pattern.Integrity) || l.Kind != "store-touched" || l.Level != steer.LevelFlag.String() ||
				l.ModelText != "" || l.UserText != c.want || l.Delivery == nil || *l.Delivery != steer.DeliveryFlagOnly {
				t.Errorf("steer line %s/%s at %s, model %q, user %q, delivery %v, state %v; want integrity/store-touched, flag, %q, flag_only",
					l.Pattern, l.Kind, l.Level, l.ModelText, l.UserText, l.Delivery, l.State, c.want)
			}
		})
	}
}

// TestST1t_StoreNamedScansForTheStoreRoot (D173): the trigger also reads the
// raw input for the store root, its slashes plain or escaped; a sibling
// whose name only starts like the root's last element, or no root, does
// not trip it. An input that names the root's last element and escapes no
// slash costs no allocation: the step runs on every post.
func TestST1t_StoreNamedScansForTheStoreRoot(t *testing.T) {
	const root = "/home/u/.local/state/rashomon"
	for in, want := range map[string]bool{
		`{"file_path":"/home/u/.local/state/rashomon/spec-trust.json"}`:          true,
		`{"file_path":"\/home\/u\/.local\/state\/rashomon\/spec-trust.json"}`:    true,
		`{"command":"printf x > /home/u/.local/state/rashomon/spec-trust.json"}`: true,
		`{"file_path":"/home/u/.local/state/rashomo/x"}`:                         false,
		`{"command":"rashomon status"}`:                                          false,
	} {
		if got := storeNamed([]byte(in), root); got != want {
			t.Errorf("storeNamed(%s, %s) = %v, want %v", in, root, got, want)
		}
	}
	if storeNamed([]byte(`{"file_path":"/home/u/.local/state/rashomon/spec-trust.json"}`), "") {
		t.Error("storeNamed with no root tripped on the store's path")
	}
	near := []byte(`{"command":"go test ./... in /home/u/src/rashomon"}`)
	if n := testing.AllocsPerRun(20, func() { storeNamed(near, root) }); n != 0 {
		t.Errorf("storeNamed over an input naming %s alone: %v allocs, want 0", filepath.Base(root), n)
	}
}
