package spec

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestST1q_ABindingNeverTrustsTheManagedSpec (QA-2 ST9): a repository spec
// Parse refuses adds no source, so the last source is the managed spec. The
// binding check asks only about the repository spec it read, so even a
// bound that holds for anything leaves the managed spec Managed, never
// Trusted, and is never asked.
func TestST1q_ABindingNeverTrustsTheManagedSpec(t *testing.T) {
	m := newFS().managed(docA).repo("/r", broken)
	var asked []string
	c, err := DiscoverBound(m, Locations("/r", "/r"), managedAt, func(path, _ string) bool {
		asked = append(asked, path)
		return true
	})
	if err != nil || c == nil {
		t.Fatalf("DiscoverBound: %+v, %v", c, err)
	}
	if want := []string{managedAt}; !slices.Equal(paths(c), want) || c.Sources[0].Trust != Managed {
		t.Errorf("sources %+v, want the managed spec alone, managed", c.Sources)
	}
	if len(asked) != 0 {
		t.Errorf("bound was asked about %q; want no question with no repository source", asked)
	}
}

// trustKey is a key of TrustKeyBytes bytes, each b.
func trustKey(b byte) []byte { return bytes.Repeat([]byte{b}, TrustKeyBytes) }

// TestST1k_UnsignedBindingsBindNothing (D173): EncodeBindings' file reads
// back under its key; a file with no MAC, a MAC under another key or under
// an empty one, a key of another length or none, and a binding edited under
// a kept MAC are each ErrBindingsUnsigned, and no binding is read. A key of
// another length is refused on encode.
func TestST1k_UnsignedBindingsBindNothing(t *testing.T) {
	b := &Bindings{Bindings: []Binding{{ProjectRoot: "/p", SpecPath: "/p/.claude/rashomon/spec.json", SpecDigest: "sha256:ab", BoundAtMS: 1}}}
	good, err := EncodeBindings(b, trustKey(1))
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`[{"project_root":"/p","spec_path":"/p/.claude/rashomon/spec.json","spec_digest":"sha256:ab","bound_at_unix_ms":1}]`)
	macOf := func(key []byte) string {
		m := hmac.New(sha256.New, key)
		m.Write(raw)
		return hex.EncodeToString(m.Sum(nil))
	}
	if want := `{"bindings":` + string(raw) + `,"mac":"` + macOf(trustKey(1)) + `"}` + "\n"; string(good) != want {
		t.Fatalf("EncodeBindings = %s, want %s", good, want)
	}
	edited := bytes.Replace(good, []byte(`"/p"`), []byte(`"/q"`), 1)
	for _, c := range []struct {
		name string
		file string
		key  []byte // nil writes no key file
		ok   bool
	}{
		{"signed under the key", string(good), trustKey(1), true},
		{"no mac", `{"bindings":` + string(raw) + "}\n", trustKey(1), false},
		{"another key", string(good), trustKey(2), false},
		{"a key of another length", string(good), trustKey(1)[:TrustKeyBytes-1], false},
		{"no key", string(good), nil, false},
		{"a mac under an empty key, no key", `{"bindings":` + string(raw) + `,"mac":"` + macOf(nil) + `"}`, nil, false},
		{"a binding edited under the mac", string(edited), trustKey(1), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newFS()
			m.files[filepath.Join("/store", TrustFile)] = c.file
			if c.key != nil {
				m.files[filepath.Join("/store", TrustKeyFile)] = string(c.key)
			}
			got, err := ReadBindings(m, "/store")
			if c.ok {
				if err != nil || !got.Holds("/p", "/p/.claude/rashomon/spec.json", "sha256:ab") {
					t.Errorf("ReadBindings = %+v, %v; want the binding", got, err)
				}
				return
			}
			if !errors.Is(err, ErrBindingsUnsigned) || got != nil {
				t.Errorf("ReadBindings = %+v, %v; want nothing and ErrBindingsUnsigned", got, err)
			}
		})
	}
	if got, err := ReadBindings(newFS(), "/store"); err != nil || len(got.Bindings) != 0 {
		t.Errorf("ReadBindings with no file = %+v, %v; want none and no error", got, err)
	}
	if _, err := EncodeBindings(b, trustKey(1)[:8]); err == nil {
		t.Error("EncodeBindings under an 8-byte key: no error")
	}
}

// TestST1w_AFileTheVerbDidNotWriteIsUnsigned (D173b, QA-2's item 4): a
// bindings file that is not the file the verb writes is ErrBindingsUnsigned
// with no binding read, so the verb replaces it rather than stopping on it:
// a file that is not JSON, a file with a field it does not name, a file
// over its cap, a file beside an unreadable key, and a file whose bindings
// are not a list under a MAC that checks. A file that cannot be read at all
// is another error.
func TestST1w_AFileTheVerbDidNotWriteIsUnsigned(t *testing.T) {
	inner := []byte(`{"not":"a list"}`)
	m := hmac.New(sha256.New, trustKey(1))
	m.Write(inner)
	notAList := `{"bindings":` + string(inner) + `,"mac":"` + hex.EncodeToString(m.Sum(nil)) + `"}`
	for _, c := range []struct {
		name     string
		file     string
		fileErr  error
		keyErr   error
		unsigned bool
	}{
		{"not JSON", "x\n", nil, nil, true},
		{"a field it does not name", `{"bindings":[],"mac":"00","extra":1}`, nil, nil, true},
		{"over its cap", `{"bindings":[],"mac":"` + strings.Repeat("0", MaxTrustBytes) + `"}`, nil, nil, true},
		{"an unreadable key", `{"bindings":[],"mac":"00"}`, nil, fs.ErrPermission, true},
		{"bindings that are not a list", notAList, nil, nil, true},
		{"a file that cannot be read", "", fs.ErrPermission, nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			fsys := newFS()
			fsys.files[filepath.Join("/store", TrustFile)] = c.file
			fsys.files[filepath.Join("/store", TrustKeyFile)] = string(trustKey(1))
			if c.fileErr != nil {
				fsys.errs[filepath.Join("/store", TrustFile)] = c.fileErr
			}
			if c.keyErr != nil {
				fsys.errs[filepath.Join("/store", TrustKeyFile)] = c.keyErr
			}
			got, err := ReadBindings(fsys, "/store")
			if err == nil || got != nil || errors.Is(err, ErrBindingsUnsigned) != c.unsigned {
				t.Errorf("ReadBindings = %+v, %v; want nothing and unsigned %v", got, err, c.unsigned)
			}
		})
	}
}
